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
	"os"
	"time"
	"encoding/json"
	//"github.com/davecgh/go-spew/spew"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"sync/atomic"
	//"errors"
)

type LiveData struct {
	EGData             solarmon.EGPerfData
	InverterData       solarmon.PerfData
	GridData           solarmon.DataResponse
	DailyEnergy        EnergyCounters
	InverterEfficiency float64
	HousePowerUsage    float64
	TimeStamp          time.Time
	BattState          string
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

type MQTTServer struct {
	Url          string
	ClientID     string
	User         string
	Pass         string
	Prefix       string
	ClientHandle mqtt.Client
}

var LastGridState bool
var LastGridChange time.Time
var updateCounter uint64

func main() {
	//Find config file and read it
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
	var mqtts MQTTServer

	meter.Host = configReader.GetString("rainforest.host")
	meter.User = configReader.GetString("rainforest.cloudID")
	meter.Pass = configReader.GetString("rainforest.installCode")
	meter.Setup()

	inv.Host = configReader.GetString("inverter.host")
	inv.Port = uint16(configReader.GetInt("inverter.port"))

	eg.Host = configReader.GetString("powerwall.host")
	eg.Sn = configReader.GetString("powerwall.sn")
	eg.User = configReader.GetString("powerwall.user")

	mqtts.Url = configReader.GetString("mqtt.url")
	mqtts.ClientID = configReader.GetString("mqtt.clientiD")
	mqtts.User = configReader.GetString("mqtt.user")
	mqtts.Pass = configReader.GetString("mqtt.pass")
	mqtts.Prefix = configReader.GetString("mqtt.prefix")
	mqtts.ClientHandle = nil

	//How long to sleep between polls?
	pollms := configReader.GetInt("solarmon.pollInterval")

	//Where does the live data file live?
	liveFilename := configReader.GetString("solarmon.liveDataFile")

	//Get our Database connection
	database := openDB(dbFile)

	//Get our start-of-day kWh counters
	startOfDayEnergy := initializeSOD(database)

	dayNum := time.Now().Local().Day() //Use Day-Of-Month to detect when we roll past midnight

	//Make our inter-thread comm channels
	var gridData solarmon.DataResponse
	gridChan := make(chan solarmon.DataResponse, 10)

	var inverterData solarmon.PerfData
	inverterChan := make(chan solarmon.PerfData, 10)

	var egData solarmon.EGPerfData
	egChan := make(chan solarmon.EGPerfData, 10)

	FileWriterLiveDataChan := make(chan LiveData, 20)

	DBWriterChan := make(chan LiveData, 20)

	MQTTChan := make(chan LiveData, 20)

	var dataOut LiveData

	updateCounter = 0
	go Watchdog()

	//Launch our live data file writer, and database logger threads
	go FileWriter(liveFilename, FileWriterLiveDataChan)
	go DBWriter(database, DBWriterChan)
	go mqtts.MqttPublisher(MQTTChan)

	//Loop, polling for data and writing to our filerwriter and dbwriter channels
	stopInv := make(chan int, 1)
	stopMeter := make(chan int, 1)
	stopEG := make(chan int, 1)
	retryCount := 5

RETRY:
	go inv.PollData(pollms, inverterChan, stopInv)
	go meter.PollData(pollms, gridChan, stopMeter)
	go eg.PollData(pollms, egChan, stopEG)

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

			battPower := dataOut.EGData.Battery_instant_power
			dataOut.BattState = "STANDBY"
			if battPower < -20 {
				dataOut.BattState = "CHARGE"
			}
			if battPower > 20 {
				dataOut.BattState = "DISCHARGE"
				if dataOut.EGData.Grid_up {
					dataOut.BattState = "VOLUNTARY_DISCHARGE"
				}
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
				startOfDayEnergy.HouseUsage = dataOut.EGData.House_energy_imported / 1000.0
			}

			dataOut.DailyEnergy.SolarKWh = inverterData.AC_Energy/1000.0 - startOfDayEnergy.SolarKWh
			dataOut.DailyEnergy.KWhToGrid = dataOut.GridData.KWhToGrid - startOfDayEnergy.KWhToGrid
			dataOut.DailyEnergy.KWhFromGrid = dataOut.GridData.KWhFromGrid - startOfDayEnergy.KWhFromGrid
			dataOut.DailyEnergy.GridNet = dataOut.DailyEnergy.KWhFromGrid - dataOut.DailyEnergy.KWhToGrid
			dataOut.DailyEnergy.HouseUsage = dataOut.EGData.House_energy_imported/1000.0 - startOfDayEnergy.HouseUsage

			FileWriterLiveDataChan <- dataOut
			DBWriterChan <- dataOut
			MQTTChan <- dataOut
			//fmt.Printf("Grid Demand: %.6g W, Solar Generation: %.6g W, House Demand: %.6gW\n",
			//	gridData.InstantaneousDemand, inverterData.AC_Power, dataOut.HousePowerUsage)
		} else {
			//timed out. kill our goroutines
			stopInv <- 1
			stopMeter <- 1
			stopEG <- 1
			break
		}
	}
	time.Sleep(10 * time.Second)
	retryCount = retryCount - 1
	fmt.Printf("Reading failure. Retry count: %d\n", retryCount)
	if retryCount > 0 {
		goto RETRY
	} else {
		os.Exit(1)
	}
}

func Watchdog() {
	var lastCount uint64
	for {
		lastCount = updateCounter
		time.Sleep(120 * time.Second)
		if lastCount == updateCounter {
			fmt.Printf("Watchdog timeout\n")
			os.Exit(1)
		}
	}
}

//Write the LiveData file for web use
func FileWriter(filename string, dataChan chan LiveData) {

	for {
		dataOut := <-dataChan
		serialized, err := yaml.Marshal(dataOut)
		if err != nil {
			_ = fmt.Errorf("YAML Marshalling error: %s\n", err)
		} else {

			ioutil.WriteFile(filename, serialized, 0644)
		}
		atomic.AddUint64(&updateCounter, 1)
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
		_ = fmt.Errorf("schema get failed: %s\n", err)
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
	if err != nil {
		_ = fmt.Errorf("Prepare error: %s", err)
	}

	for {
		currentData := <-dataChan

		timeLastContact, _ := currentData.GridData.LastContact.MarshalText()
		timeStamp, _ := currentData.TimeStamp.MarshalText()
		gridLastChange, _ := currentData.EGData.Grid_last_change.MarshalText()
		err := db.Ping()
		if err != nil {
			_ = fmt.Errorf("Database ping response: %s\n", err)
		}

		_, err = statement.Exec(timeLastContact,
			currentData.GridData.InstantaneousDemand, currentData.GridData.KWhFromGrid,
			currentData.GridData.KWhToGrid, currentData.InverterData.AC_Power, currentData.InverterData.AC_Current,
			currentData.InverterData.AC_Voltage, currentData.InverterData.AC_VA, currentData.InverterData.AC_VAR,
			currentData.InverterData.AC_PF, currentData.InverterData.AC_Freq, currentData.InverterData.AC_Energy,
			currentData.InverterData.DC_Voltage, currentData.InverterData.DC_Current,
			currentData.InverterData.DC_Power, currentData.InverterData.SinkTemp, int(currentData.InverterData.Status),
			int(currentData.InverterData.Event1), currentData.InverterEfficiency, currentData.HousePowerUsage,
			timeStamp, currentData.EGData.Grid_status, currentData.EGData.Grid_up, gridLastChange,

			currentData.EGData.Grid_services_active, currentData.EGData.Uptime, currentData.EGData.Running,
			currentData.EGData.Connected_to_tesla, currentData.EGData.Batt_percentage,
			currentData.EGData.Solar_energy_exported,
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

		if err != nil {
			_ = fmt.Errorf("Database write error: %s\n", err)
		}
	}

}

//Initialize Energy counters with start-of-day numbers
func initializeSOD(db *sql.DB) EnergyCounters {
	var results EnergyCounters

	results.SolarKWh = 0
	results.KWhToGrid = 0
	results.KWhFromGrid = 0
	results.HouseUsage = 0
	var lastChange string

	//First try to get the first entry for today
	res := db.QueryRow(`SELECT meter_KWHFromGrid,meter_KWHToGrid,inv_AC_Energy,eg_house_energy_imported FROM solarPerf 
                            WHERE datetime(timestamp,'localtime') >= date('now','localtime')
                            ORDER BY timestamp LIMIT 1;`)

	err := res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh, &results.HouseUsage)
	if err == nil {
		//fmt.Printf("First restore\n")
		results.SolarKWh = results.SolarKWh / 1000.0
		results.HouseUsage = results.HouseUsage / 1000.0
	} else {
		_ = fmt.Errorf("First restore error: %s\n", err)
		//No entry yet for today, so take the last available
		res = db.QueryRow(`SELECT meter_KWHFromGrid,meter_KWHToGrid,inv_AC_Energy ,eg_house_energy_imported FROM solarPerf 
                           ORDER BY timestamp DESC LIMIT 1`)
		err = res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh, &results.HouseUsage)
		if err == nil {
			//fmt.Printf("Second restore\n")
			results.SolarKWh = results.SolarKWh / 1000.0
			results.HouseUsage = results.HouseUsage / 1000.0
		}
	}

	//Find last grid state
	res = db.QueryRow(`SELECT grid_last_change,grid_up FROM solarPerf 
                           ORDER BY timestamp DESC LIMIT 1;`)
	err = res.Scan(&lastChange, &LastGridState)
	if err == nil {
		LastGridChange.UnmarshalText([]byte(lastChange))
		return (results)
	} else {
		_ = fmt.Errorf("Force last grid change, %s\n", err)
		LastGridChange = time.Now()
		LastGridState = true
	}
        fmt.Printf("DB SOD done.\n")
	return (results)

}

// Beginning of day in sqlite3:
//SELECT * FROM statistics WHERE date(date) BETWEEN date(datetime('now', 'start of day')) AND date(datetime('now')) ORDER BY date LIMIT 1;

func (server *MQTTServer) connect() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(server.Url)
	opts.SetUsername(server.User)
	opts.SetPassword(server.Pass)
	opts.SetClientID(server.ClientID)
	opts.SetKeepAlive(10)
	opts.SetMaxReconnectInterval(30)
	opts.SetWill(server.Prefix+"/"+"solarmon", "OFFLINE", 0, true)
	opts.SetAutoReconnect(true)

	server.ClientHandle = mqtt.NewClient(opts)
	ConnToken := server.ClientHandle.Connect()
	for !ConnToken.WaitTimeout(5 * time.Second) {
	}
	if err := ConnToken.Error(); err != nil {
		_ = fmt.Errorf("MQTT Connection problem: %s\n", err)
		//log.Fatal(err)
	}
}

func (server *MQTTServer) publish(topic string, value string, retain bool, synchronous bool) error {
	fullTopic := server.Prefix + "/" + topic
	token := server.ClientHandle.Publish(fullTopic, 0, retain, value)
	if err := token.Error(); err != nil {
		_ = fmt.Errorf("Publish error: %s\n", err)
		return (err)
	}

	if synchronous {
		if token.WaitTimeout(5 * time.Second) {
			//OK
			return (nil)
		} else {
			return (fmt.Errorf("Publish timed out"))
		}
	}
	return (nil)
}

func (server *MQTTServer) MqttPublisher(dataChan chan LiveData) {
	server.connect()
	for {
		currentData := <-dataChan

		//timeLastContact, _ := currentData.GridData.LastContact.MarshalText()
		timeStamp, _ := currentData.TimeStamp.MarshalText()
		gridLastChange, _ := currentData.EGData.Grid_last_change.MarshalText()

		retain := true
		async := false
		sync := true

		_ = server.publish("solarmon", "ONLINE", retain, async)
		_ = server.publish("grid/up", fmt.Sprintf("%t", currentData.EGData.Grid_up), retain, async)
		_ = server.publish("battery/status", currentData.BattState, retain, async)
		_ = server.publish("battery/soe", fmt.Sprintf("%5.4g", currentData.EGData.Batt_percentage), retain, async)
		_ = server.publish("grid/status", currentData.EGData.Grid_status, retain, async)
		_ = server.publish("eg/uptime", fmt.Sprintf("%d", currentData.EGData.Uptime), retain, async)
		_ = server.publish("eg/running", fmt.Sprintf("%t", currentData.EGData.Running), retain, async)
		_ = server.publish("eg/timestamp", string(timeStamp), retain, async)
		_ = server.publish("grid/gridLastChange", string(gridLastChange), retain, async)
		_ = server.publish("grid/power", fmt.Sprintf("%.6g", currentData.GridData.InstantaneousDemand), retain, async)
		_ = server.publish("grid/energyToGridK", fmt.Sprintf("%g", currentData.GridData.KWhToGrid), retain, async)
		_ = server.publish("grid/energyFromGridK", fmt.Sprintf("%g", currentData.GridData.KWhFromGrid), retain, async)
		_ = server.publish("solar/power", fmt.Sprintf("%g", currentData.InverterData.AC_Power), retain, async)
		_ = server.publish("solar/frequency", fmt.Sprintf("%g", currentData.EGData.Solar_frequency), retain, async)
		_ = server.publish("solar/temp", fmt.Sprintf("%g", currentData.InverterData.SinkTemp), retain, async)
		_ = server.publish("solar/efficiency", fmt.Sprintf("%g", currentData.InverterEfficiency), retain, async)

		serialized, err := yaml.Marshal(currentData)
		if err != nil {
			_ = fmt.Errorf("YAML Marshalling error: %s\n", err)
		} else {
			_ = server.publish("solarmonData", string(serialized), retain, sync)
		}

		serialized, err = json.Marshal(currentData)
		if err != nil {
			_ = fmt.Errorf("Json Marshalling error: %s\n", err)
		} else {
			_ = server.publish("solarmonDatajson", string(serialized), retain, sync)
		}
	}
}
