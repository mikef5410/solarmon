package main

import (
	//"encoding/hex"
	"fmt"
	"github.com/spf13/viper"
	//"os"
	"github.com/mikef5410/solarmon"
	//"time"
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
	meter.GetData()
}
