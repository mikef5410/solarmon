package main

import (
	//"encoding/hex"
	"fmt"
	"github.com/spf13/viper"
	//"os"
	"time"
)

func main() {
	var meter rainforestEagle200Local

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


	fmt.Printf("Connect to: %s\n", configReader.GetString("rainforest.host"), )

	meter.host = configReader.GetString("rainforest.host")
	meter.user = configReader.GetString("rainforest.cloudID"))
	meter.pass = configReader.GetString("rainforest.installCode"))


	fmt.Printf("%v\n",meter)
	
	meter.setup()
}
