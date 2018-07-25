package main

import (
	//"encoding/hex"
	"fmt"
	"github.com/spf13/viper"
	//"os"
	"github.com/mikef5410/solarmon"
	"time"
)

func main() {
	var meter solarmon.RainforestEagle200Local

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

	fmt.Printf("Connect to: %s\n", configReader.GetString("rainforest.host"))

	meter.Host = configReader.GetString("rainforest.host")
	meter.User = configReader.GetString("rainforest.cloudID")
	meter.Pass = configReader.GetString("rainforest.installCode")

	fmt.Printf("%v\n", meter)

	meter.Setup()
	j:=0
	pollms := time.Duration(configReader.GetInt("inverter.pollInterval")) * time.Millisecond
	
	for j<10 {
		resp := meter.GetData()
		fmt.Printf("Data age: %.6g s\n", time.Since(resp.LastContact).Seconds())
		fmt.Printf("Current Demand: %.8g W\n", resp.InstantaneousDemand*1000.0)
		fmt.Printf("KWh From Grid: %.8g kWh\n", resp.KWhFromGrid)
		fmt.Printf("KWh To Grid: %.8g kWh\n", resp.KWhToGrid)

		time.Sleep(pollms)
		j++
	}
}
