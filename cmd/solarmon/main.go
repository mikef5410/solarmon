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
)

type LiveData struct {
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
}

func main() {
	var gridData solarmon.DataResponse
	gridChan := make(chan solarmon.DataResponse)

	var inverterData solarmon.PerfData
	inverterChan := make(chan solarmon.PerfData)

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

	meter.Host = configReader.GetString("rainforest.host")
	meter.User = configReader.GetString("rainforest.cloudID")
	meter.Pass = configReader.GetString("rainforest.installCode")
	meter.Setup()

	inv.Host = configReader.GetString("inverter.host")
	inv.Port = uint16(configReader.GetInt("inverter.port"))

	//How long to sleep between polls?
	pollms := time.Duration(configReader.GetInt("solarmon.pollInterval")) * time.Millisecond

	//Where does the live data file live?
	liveFilename := configReader.GetString("solarmon.liveDataFile")

	var dataOut LiveData

	database := openDB(dbFile)
	startOfDayEnergy := initializeSOD(database) 
	dayNum := time.Now().Day() //Use Day-Of-Month to detect when we roll past midnight

	//Get our start-of-day kWh counters

	//Launch our live data file writer, and database logger threads
	go FileWriter(liveFilename, FileWriterLiveDataChan)
	go DBWriter(database, DBWriterChan)

	//Loop, polling for data and writing to our filerwriter and dbwriter channels
	for {
		stopInv := make(chan int, 1)
		stopMeter := make(chan int, 1)

		go inv.PollData(inverterChan, stopInv)
		go meter.PollData(gridChan, stopMeter)

		//gridData = <-gridChan
		//inverterData = <-inverterChan
		gotGrid := false
		gotInv := false
		timeout := false
		for ((gotGrid && gotInv) == false) && (timeout == false) {
			select {
			case gridData = <-gridChan:
				gotGrid = true
			case inverterData = <-inverterChan:
				gotInv = true
			case <-time.After(120 * time.Second):
				timeout = true
			}
		}

		if (timeout == false) && ((gotGrid && gotInv) == true) {
			gridData.InstantaneousDemand = gridData.InstantaneousDemand * 1000 //Convert kW to W

			dataOut.InverterData = inverterData
			dataOut.GridData = gridData
			dataOut.InverterEfficiency = 100 * inverterData.AC_Power / inverterData.DC_Power
			dataOut.HousePowerUsage = inverterData.AC_Power + gridData.InstantaneousDemand
			dataOut.TimeStamp = time.Now()

			if time.Now().Day() != dayNum { //We just rolled past midnight
				dayNum = time.Now().Day()
				// update start-of-day numbers
				startOfDayEnergy.SolarKWh = inverterData.AC_Energy/1000.0
				startOfDayEnergy.KWhToGrid = dataOut.GridData.KWhToGrid
				startOfDayEnergy.KWhFromGrid = dataOut.GridData.KWhFromGrid
			}

			dataOut.DailyEnergy.SolarKWh = inverterData.AC_Energy/1000.0 - startOfDayEnergy.SolarKWh
			dataOut.DailyEnergy.KWhToGrid = dataOut.GridData.KWhToGrid - startOfDayEnergy.KWhToGrid
			dataOut.DailyEnergy.KWhFromGrid = dataOut.GridData.KWhFromGrid - startOfDayEnergy.KWhFromGrid
			dataOut.DailyEnergy.GridNet = dataOut.DailyEnergy.KWhFromGrid - dataOut.DailyEnergy.KWhToGrid
			dataOut.DailyEnergy.HouseUsage =  dataOut.DailyEnergy.GridNet + dataOut.DailyEnergy.SolarKWh

			FileWriterLiveDataChan <- dataOut
			DBWriterChan <- dataOut
			//fmt.Printf("Grid Demand: %.6g W, Solar Generation: %.6g W, House Demand: %.6gW\n",
			//	gridData.InstantaneousDemand, inverterData.AC_Power, dataOut.HousePowerUsage)
		} else {
			//timed out. kill our goroutines
			stopInv <- 1
			stopMeter <- 1
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
	database, _ := sql.Open("sqlite3", filename)
	statement, _ := database.Prepare(`CREATE TABLE IF NOT EXISTS solarPerf (id INTEGER PRIMARY KEY,
                        meter_lastContact datetime, meter_demand REAL, meter_KWHFromGrid REAL, meter_KWHToGrid REAL,
                        inv_AC_Power REAL, inv_AC_Current REAL, inv_AC_Voltage REAL, inv_AC_VA REAL, inv_AC_VAR REAL,
                        inv_AC_PF REAL, inv_AC_Freq REAL, inv_AC_Energy REAL, inv_DC_Voltage REAL, inv_DC_Current REAL,
                        inv_DC_Power REAL, inv_SinkTemp REAL, inv_Status int, inv_Event1 int, inv_Efficiency REAL,
                        House_Demand REAL, timestamp datetime )`)
	statement.Exec()
	return (database)
}

func DBWriter(db *sql.DB, dataChan chan LiveData) {
	statement, _ := db.Prepare(`INSERT INTO solarPerf (meter_lastContact, meter_demand, meter_KWHFromGrid, meter_KWHToGrid,
                                    inv_AC_Power, inv_AC_Current, inv_AC_Voltage, inv_AC_VA, inv_AC_VAR, 
                                    inv_AC_PF, inv_AC_Freq, inv_AC_Energy, inv_DC_Voltage, inv_DC_Current,
                                    inv_DC_Power, inv_SinkTemp, inv_Status, inv_Event1, inv_Efficiency,
                                    House_Demand, timestamp ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	for {
		currentData := <-dataChan

		timeLastContact, _ := currentData.GridData.LastContact.MarshalText()
		timeStamp, _ := currentData.TimeStamp.MarshalText()
		statement.Exec(timeLastContact,
			currentData.GridData.InstantaneousDemand, currentData.GridData.KWhFromGrid,
			currentData.GridData.KWhToGrid, currentData.InverterData.AC_Power, currentData.InverterData.AC_Current,
			currentData.InverterData.AC_Voltage, currentData.InverterData.AC_VA, currentData.InverterData.AC_VAR,
			currentData.InverterData.AC_PF, currentData.InverterData.AC_Freq, currentData.InverterData.AC_Energy,
			currentData.InverterData.DC_Voltage, currentData.InverterData.DC_Current,
			currentData.InverterData.DC_Power, currentData.InverterData.SinkTemp, int(currentData.InverterData.Status),
			int(currentData.InverterData.Event1), currentData.InverterEfficiency, currentData.HousePowerUsage,
			timeStamp)
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
                            WHERE timeStamp BETWEEN datetime('now','start of day') AND datetime('now','localtime') 
                            ORDER BY timeStamp LIMIT 1`)

	err := res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh)
	if err == nil {
		return (results)
	}

	//No entry yet for today, so take the last available
	res = db.QueryRow(`SELECT meter_KWHFromGrid,meter_KWHToGrid,inv_AC_Energy FROM solarPerf 
                           WHERE timeStamp <= datetime('now','start of day')  
                           ORDER BY timeStamp DESC LIMIT 1`)
	err = res.Scan(&results.KWhFromGrid, &results.KWhToGrid, &results.SolarKWh)
	if err == nil {
		return (results)
	}

	return (results)
}

// Beginning of day in sqlite3:
//SELECT * FROM statistics WHERE date BETWEEN datetime('now', 'start of day') AND datetime('now', 'localtime') ORDER BY date LIMIT 1;
