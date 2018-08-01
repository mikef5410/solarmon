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
	InverterEfficiency float64
	HousePowerUsage    float64
	TimeStamp          int64
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
			dataOut.TimeStamp = time.Now().Unix()

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
	statement, _ := db.Prepare(`INSERT INTO solarPerf (datetime(meter_lastContact,'unixepoch'), meter_demand, meter_KWHFromGrid, meter_KWHToGrid,
                                    inv_AC_Power, inv_AC_Current, inv_AC_Voltage, inv_AC_VA, inv_AC_VAR, 
                                    inv_AC_PF, inv_AC_Freq, inv_AC_Energy, inv_DC_Voltage, inv_DC_Current,
                                    inv_DC_Power, inv_SinkTemp, inv_Status, inv_Event1, inv_Efficiency,
                                    House_Demand, datetime(timestamp,'unixepoch') ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	for {
		currentData := <-dataChan
		statement.Exec(currentData.GridData.LastContact.Unix(),
			currentData.GridData.InstantaneousDemand, currentData.GridData.KWhFromGrid,
			currentData.GridData.KWhToGrid, currentData.InverterData.AC_Power, currentData.InverterData.AC_Current,
			currentData.InverterData.AC_Voltage, currentData.InverterData.AC_VA, currentData.InverterData.AC_VAR,
			currentData.InverterData.AC_PF, currentData.InverterData.AC_Freq, currentData.InverterData.AC_Energy,
			currentData.InverterData.DC_Voltage, currentData.InverterData.DC_Current,
			currentData.InverterData.DC_Power, currentData.InverterData.SinkTemp, int(currentData.InverterData.Status),
			int(currentData.InverterData.Event1), currentData.InverterEfficiency, currentData.HousePowerUsage,
			currentData.TimeStamp)
	}

}
