package main

import (
	//"flag"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mikef5410/solarmon"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"time"
	//"github.com/davecgh/go-spew/spew"
)

type LiveData struct {
	EGData             solarmon.EGPerfData
	InverterData       solarmon.PerfData
	GridData           solarmon.DataResponse
	DailyEnergy        EnergyCounters
	InverterEfficiency float64
	HousePowerUsage    float64
	TimeStamp          time.Time
}

type EnergyCounters struct {
	SolarKWh    float64
	KWhToGrid   float64
	KWhFromGrid float64
	GridNet     float64
	HouseUsage  float64
	KWhToBatt   float64
	KWhFromBatt float64
}

var LastGridState bool
var LastGridChange time.Time

func main() {
	var gridData solarmon.DataResponse
	gridChan := make(chan solarmon.DataResponse)

	var inverterData solarmon.PerfData
	inverterChan := make(chan solarmon.PerfData)

	var egData solarmon.EGPerfData
	egChan := make(chan solarmon.EGPerfData)

	FileWriterLiveDataChan := make(chan LiveData, 50)
	DBWriterChan := make(chan LiveData, 50)

	configReader := viper.New()
	configReader.SetConfigName("solarmon")
	configReader.AddConfigPath("/etc")
	configReader.AddConfigPath("$HOME/.config")
	configReader.AddConfigPath(".")
	configReader.SetConfigType("yaml")

	err := configReader.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error in config file: %s \n", err))
	}

	dbFile := configReader.GetString("solarmon.dbFile")

	var meter solarmon.RainforestEagle200Local
	var inv solarmon.SolarEdgeModbus
	var eg solarmon.TeslaEnergyGateway

	meter.Host = configReader.GetString("rainforest.host")
	meter.User = configReader.GetString("rainforest.cloudID")
	meter.Pass = configReader.GetString("rainforest.installCode")
	meter.Setup()

	inv.Host = configReader.GetString("inverter.host")
	inv.Port = uint16(configReader.GetInt("inverter.port"))

	eg.Host = configReader.GetString("powerwall.host")
	eg.Sn = configReader.GetString("powerwall.sn")
	eg.User = configReader.GetString("powerwall.user")

	//How long to sleep between polls?
	pollms := time.Duration(configReader.GetInt("solarmon.pollInterval")) * time.Millisecond

	//Where does the live data file live?
	liveFilename := configReader.GetString("solarmon.liveDataFile")

	var dataOut LiveData

	database := openDB(dbFile)
	startOfDayEnergy := initializeSOD(database)
	dayNum := time.Now().Local().Day() //Use Day-Of-Month to detect when we roll past midnight

	//Get our start-of-day kWh counters

	//Launch our live data file writer, and database logger threads
	go FileWriter(liveFilename, FileWriterLiveDataChan)
	go DBWriter(database, DBWriterChan)

	//Loop, polling for data and writing to our filerwriter and dbwriter channels
	stopInv := make(chan int, 1)
	stopMeter := make(chan int, 1)
	stopEG := make(chan int, 1)

	go inv.PollData(inverterChan, stopInv)
	go meter.PollData(gridChan, stopMeter)
	go eg.PollData(egChan, stopEG)

	for {
		gotGrid := false
		gotInv := false
		gotEG := false
		timeout := false
		for ((gotGrid && gotInv && gotEG) == false) && (timeout == false) {
			select {
			case gridData = <-gridChan:
				gotGrid = true
			case inverterData = <-inverterChan:
				gotInv = true
			case egData = <-egChan:
				gotEG = true
			case <-time.After(120 * time.Second):
				timeout = true
			}
		}

		if (timeout == false) && ((gotGrid && gotInv && gotEG) == true) {
			if egData.Grid_up != LastGridState {
				LastGridState = egData.Grid_up
				LastGridChange = time.Now()
			}
			egData.Grid_last_change = LastGridChange
			gridData.InstantaneousDemand = gridData.InstantaneousDemand * 1000 //Convert kW to W
			dataOut.EGData = egData
			dataOut.InverterData = inverterData
			dataOut.GridData = gridData
			dataOut.InverterEfficiency = 100 * inverterData.AC_Power / inverterData.DC_Power
			dataOut.HousePowerUsage = egData.House_instant_power
			dataOut.TimeStamp = time.Now()

			if time.Now().Local().Day() != dayNum { //We just rolled past midnight
				dayNum = time.Now().Local().Day()
				// update start-of-day numbers
				startOfDayEnergy.SolarKWh = inverterData.AC_Energy / 1000.0
				startOfDayEnergy.KWhToGrid = dataOut.GridData.KWhToGrid
				startOfDayEnergy.KWhFromGrid = dataOut.GridData.KWhFromGrid
			}

			dataOut.DailyEnergy.SolarKWh = inverterData.AC_Energy/1000.0 - startOfDayEnergy.SolarKWh
			dataOut.DailyEnergy.KWhToGrid = dataOut.GridData.KWhToGrid - startOfDayEnergy.KWhToGrid
			dataOut.DailyEnergy.KWhFromGrid = dataOut.GridData.KWhFromGrid - startOfDayEnergy.KWhFromGrid
			dataOut.DailyEnergy.GridNet = dataOut.DailyEnergy.KWhFromGrid - dataOut.DailyEnergy.KWhToGrid
			dataOut.DailyEnergy.HouseUsage = dataOut.DailyEnergy.GridNet + dataOut.DailyEnergy.SolarKWh

			FileWriterLiveDataChan <- dataOut
			DBWriterChan <- dataOut
			//fmt.Printf("Grid Demand: %.6g W, Solar Generation: %.6g W, House Demand: %.6gW\n",
			//	gridData.InstantaneousDemand, inverterData.AC_Power, dataOut.HousePowerUsage)
		} else {
			//timed out. kill our goroutines
			stopInv <- 1
			stopMeter <- 1
			stopEG <- 1
		}
		time.Sleep(pollms)
	}
}

//Write the LiveData file for web use
func FileWriter(filename string, dataChan chan LiveData) {

	for {
		dataOut := <-dataChan
		serialized, err := yaml.Marshal(dataOut)
		if err != nil {
			fmt.Errorf("YAML Marshalling error: %s\n", err)
		} else {
			ioutil.WriteFile(filename, serialized, 0644)
		}
	}

}

func openDB(filename string) *sql.DB {
	vers := -1
	database, _ := sql.Open("sqlite3", filename)
	statement, _ := database.Prepare(`CREATE TABLE IF NOT EXISTS solarPerf (id INTEGER PRIMARY KEY,
                        meter_lastContact datetime, meter_demand REAL, meter_KWHFromGrid REAL, meter_KWHToGrid REAL,
                        inv_AC_Power REAL, inv_AC_Current REAL, inv_AC_Voltage REAL, inv_AC_VA REAL, inv_AC_VAR REAL,
                        inv_AC_PF REAL, inv_AC_Freq REAL, inv_AC_Energy REAL, inv_DC_Voltage REAL, inv_DC_Current REAL,
                        inv_DC_Power REAL, inv_SinkTemp REAL, inv_Status int, inv_Event1 int, inv_Efficiency REAL,
                        House_Demand REAL, timestamp datetime )`)
	statement.Exec()

	res := database.QueryRow("PRAGMA user_version;")
	err := res.Scan(&vers)
	if err != nil {
		fmt.Printf("schema get failed\n")
	}
	if vers == 0 { // Augment table, increase schema version to 1
		fmt.Printf("Upgrade database to version 1\n")
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN grid_status TEXT;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN grid_up BOOLEAN;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN grid_last_change datetime;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN grid_services_active BOOLEAN;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_uptime INTEGER;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_running BOOLEAN;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_connected_to_tesla BOOLEAN;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN batt_percentage REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_solar_energy_exported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_solar_instant_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_solar_instant_apparent_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_solar_instant_reactive_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_solar_frequency REAL;")
		statement.Exec()

		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_energy_imported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_energy_exported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_instant_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_instant_apparent_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_instant_reactive_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_grid_frequency REAL;")
		statement.Exec()

		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_house_energy_imported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_house_instant_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_house_instant_apparent_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_house_instant_reactive_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_house_frequency REAL;")
		statement.Exec()

		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_energy_imported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_energy_exported REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_instant_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_instant_apparent_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_instant_reactive_power REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_frequency REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_instant_average_voltage REAL;")
		statement.Exec()
		statement, _ = database.Prepare("ALTER TABLE solarPerf ADD COLUMN eg_battery_instant_total_current REAL;")
		statement.Exec()
		statement, _ = database.Prepare("PRAGMA user_version=1;")
		statement.Exec()
	} //If schema upgrade
	return (database)
}

func DBWriter(db *sql.DB, dataChan chan LiveData) {
	statement, err := db.Prepare(`INSERT INTO solarPerf (meter_lastContact, meter_demand, meter_KWHFromGrid, meter_KWHToGrid,
                                    inv_AC_Power, inv_AC_Current, inv_AC_Voltage, inv_AC_VA, inv_AC_VAR, 
                                    inv_AC_PF, inv_AC_Freq, inv_AC_Energy, inv_DC_Voltage, inv_DC_Current,
                                    inv_DC_Power, inv_SinkTemp, inv_Status, inv_Event1, inv_Efficiency,
                                    House_Demand, timestamp, grid_status, grid_up, grid_last_change,
                                    grid_services_active, eg_uptime, eg_running, eg_connected_to_tesla,
                                    batt_percentage, 
                                    eg_solar_energy_exported, eg_solar_instant_power, eg_solar_instant_apparent_power,
                                    eg_solar_instant_reactive_power, eg_solar_frequency,                                    
                                    eg_grid_energy_exported, eg_grid_energy_imported, eg_grid_instant_power,
                                    eg_grid_instant_apparent_power,
                                    eg_grid_instant_reactive_power, eg_grid_frequency,                                    
                                    eg_house_energy_imported, eg_house_instant_power, eg_house_instant_apparent_power,
                                    eg_house_instant_reactive_power, eg_house_frequency,                                    
                                    eg_battery_energy_exported, eg_battery_energy_imported, eg_battery_instant_power,
                                    eg_battery_instant_apparent_power,
                                    eg_battery_instant_reactive_power, eg_battery_frequency,  eg_battery_instant_average_voltage, 
                                    eg_battery_instant_total_current )
                                    values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
                                               ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
                                               ?, ?, ?, ?, ? )`)
	if (err != nil) {
		fmt.Printf("Prepare error: %s", err);
	}

	for {
		currentData := <-dataChan

		timeLastContact, _ := currentData.GridData.LastContact.MarshalText()
		timeStamp, _ := currentData.TimeStamp.MarshalText()
		gridLastChange, _ := currentData.EGData.Grid_last_change.MarshalText()

		statement.Exec(timeLastContact,
			currentData.GridData.InstantaneousDemand, currentData.GridData.KWhFromGrid,
			currentData.GridData.KWhToGrid, currentData.InverterData.AC_Power, currentData.InverterData.AC_Current,
			currentData.InverterData.AC_Voltage, currentData.InverterData.AC_VA, currentData.InverterData.AC_VAR,
			currentData.InverterData.AC_PF, currentData.InverterData.AC_Freq, currentData.InverterData.AC_Energy,
			currentData.InverterData.DC_Voltage, currentData.InverterData.DC_Current,
			currentData.InverterData.DC_Power, currentData.InverterData.SinkTemp, int(currentData.InverterData.Status),
			int(currentData.InverterData.Event1), currentData.InverterEfficiency, currentData.HousePowerUsage,
			timeStamp, currentData.EGData.Grid_status, currentData.EGData.Grid_up, gridLastChange,

			currentData.EGData.Grid_services_active, currentData.EGData.Uptime, currentData.EGData.Running,
			currentData.EGData.Connected_to_tesla, currentData.EGData.Batt_percentage, currentData.EGData.Solar_energy_exported,
			currentData.EGData.Solar_instant_power, currentData.EGData.Solar_instant_apparent_power,
			currentData.EGData.Solar_instant_reactive_power, currentData.EGData.Solar_frequency,

			currentData.EGData.Grid_energy_exported, currentData.EGData.Grid_energy_imported,
			currentData.EGData.Grid_instant_power, currentData.EGData.Grid_instant_apparent_power,
			currentData.EGData.Grid_instant_reactive_power, currentData.EGData.Grid_frequency,

			currentData.EGData.House_energy_imported, currentData.EGData.House_instant_power,
			currentData.EGData.House_instant_apparent_power, currentData.EGData.House_instant_reactive_power,
			currentData.EGData.House_frequency,

			currentData.EGData.Battery_energy_exported, currentData.EGData.Battery_energy_imported,
			currentData.EGData.Battery_instant_power, currentData.EGData.Battery_instant_apparent_power,
			currentData.EGData.Battery_instant_reactive_power, currentData.EGData.Battery_frequency,
			currentData.EGData.Battery_instant_average_voltage, currentData.EGData.Battery_instant_total_current)
	}

}

//Initialize Energy counters with start-of-day numbers
func initializeSOD(db *sql.DB) EnergyCounters {
	var results EnergyCounters

	results.SolarKWh = 0
	results.KWhToGrid = 0
	results.KWhFromGrid = 0

	//First try to get the first entry for today
	res := db.QueryRow(`SELECT meter_KWHFromGrid,meter_KWHToGrid,inv_AC_Energy FROM solarPerf 
                            WHERE datetime(timestamp) >= datetime('now','start of day','localtime')
                            ORDER BY timestamp LIMIT 1;`)

	err := res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh)
	if err == nil {
		fmt.Printf("First restore\n")
		results.SolarKWh = results.SolarKWh / 1000.0
	} else {
		fmt.Printf("First restore error: %s\n",err)
		//No entry yet for today, so take the last available
		res = db.QueryRow(`SELECT meter_KWHFromGrid,meter_KWHToGrid,inv_AC_Energy FROM solarPerf 
                           ORDER BY timestamp DESC LIMIT 1`)
		err = res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh)
		if err == nil {
		fmt.Printf("Second restore\n")
			results.SolarKWh = results.SolarKWh / 1000.0
		}
	}

	//Find last grid state
	res = db.QueryRow(`SELECT grid_last_change,grid_up FROM solarPerf 
                           ORDER BY timestamp DESC LIMIT 1;`)
	err = res.Scan(&LastGridChange, &LastGridState)
	if err == nil {
		return (results)
	} else {
		fmt.Printf("Force last grid change, %s\n",err)
		LastGridChange=time.Now()
		LastGridState=true
	}

	return (results)
	
}

// Beginning of day in sqlite3:
//SELECT * FROM statistics WHERE date(date) BETWEEN date(datetime('now', 'start of day')) AND date(datetime('now')) ORDER BY date LIMIT 1;
