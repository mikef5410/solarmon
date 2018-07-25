package main

import (
	//"flag"
	"fmt"
	"github.com/mikef5410/solarmon"
	"github.com/spf13/viper"
	//"time"
)

func main() {

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


	gridResult := meter.GetData()
	inverterPwr := inv.GetReg("I_AC_Power")
	//inverterVoltage := inv.GetReg("I_AC_VoltageAB")
	//inverterCurrent := inv.GetReg("I_AC_Current")

	fmt.Printf("Grid Demand: %.6g W, Solar Output: %.6g W, House is using: %.6gW\n", gridResult.InstantaneousDemand*1000, inverterPwr.Value, inverterPwr.Value+(gridResult.InstantaneousDemand*1000.0))
}
