package main

import (
	//"flag"
	"fmt"
	"github.com/mikef5410/solarmon"
	"github.com/spf13/viper"
	"time"
)

func main() {
	var gridData solarmon.DataResponse
	gridChan := make(chan solarmon.DataResponse)

	var inverterData solarmon.PerfData
	inverterChan := make(chan solarmon.PerfData)

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

	//gridResult := meter.GetData()
	//inverterPwr := inv.GetReg("I_AC_Power")
	//inverterVoltage := inv.GetReg("I_AC_VoltageAB")
	//inverterCurrent := inv.GetReg("I_AC_Current")

	j:=0
	pollms := time.Duration(1000 * time.Millisecond)
	
	for j < 20 {
	
		go inv.PollData(inverterChan)
		go meter.PollData(gridChan)

		gridData = <-gridChan
		inverterData = <-inverterChan

		fmt.Printf("Grid Demand: %.6g W, Solar Output: %.6g W, House is using: %.6gW\n", gridData.InstantaneousDemand*1000, inverterData.AC_Power, inverterData.AC_Power+(gridData.InstantaneousDemand*1000.0))

		time.Sleep(pollms)
		j++
	}
}
