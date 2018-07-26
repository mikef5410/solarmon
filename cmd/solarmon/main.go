package main

import (
	//"flag"
	"fmt"
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

	var meter solarmon.RainforestEagle200Local
	var inv solarmon.SolarEdgeModbus

	meter.Host = configReader.GetString("rainforest.host")
	meter.User = configReader.GetString("rainforest.cloudID")
	meter.Pass = configReader.GetString("rainforest.installCode")
	meter.Setup()

	inv.Host = configReader.GetString("inverter.host")
	inv.Port = uint16(configReader.GetInt("inverter.port"))

	j := 0
	pollms := time.Duration(configReader.GetInt("solarmon.pollInterval")) * time.Millisecond
	liveFilename := configReader.GetString("solarmon.liveDataFile")

	var dataOut LiveData

	for j < 20 {

		go inv.PollData(inverterChan)
		go meter.PollData(gridChan)

		go FileWriter(liveFilename, FileWriterLiveDataChan)

		gridData = <-gridChan
		inverterData = <-inverterChan

		gridData.InstantaneousDemand = gridData.InstantaneousDemand * 1000 //Convert kW to W

		dataOut.InverterData = inverterData
		dataOut.GridData = gridData
		dataOut.InverterEfficiency = 100 * inverterData.AC_Power / inverterData.DC_Power
		dataOut.HousePowerUsage = inverterData.AC_Power + gridData.InstantaneousDemand
		dataOut.TimeStamp = time.Now().Unix()

		FileWriterLiveDataChan <- dataOut
		fmt.Printf("Grid Demand: %.6g W, Solar Generation: %.6g W, House Demand: %.6gW\n",
			gridData.InstantaneousDemand, inverterData.AC_Power, dataOut.HousePowerUsage)

		time.Sleep(pollms)
		j++
	}
}

func FileWriter(filename string, dataChan chan LiveData) {

	for {
		dataOut := <-dataChan
		serialized, err := yaml.Marshal(dataOut)
		if ( err != nil ) {
			fmt.Errorf("YAML Marshalling error: %s\n",err)
		} else {
			ioutil.WriteFile(filename, serialized, 0644)
		}
	}

}
