package main

import (
	//"flag"
	//"database/sql"
	"fmt"
	//"github.com/mattn/go-sqlite3"
	"github.com/mikef5410/solarmon"
	"github.com/spf13/viper"
	//"gopkg.in/resty.v1"
	//"gopkg.in/yaml.v2"
	//"io/ioutil"
	"time"
)

func main() {
	var egData solarmon.EGPerfData
	EGChan := make(chan solarmon.EGPerfData)

	var eg solarmon.TeslaEnergyGateway

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
	eg.Host = configReader.GetString("powerwall.host")
	eg.Sn = configReader.GetString("powerwall.sn")
	eg.User = configReader.GetString("powerwall.user")

	for {
		stopPW := make(chan int, 1)
		go eg.PollData(EGChan, stopPW)
		gotEG := false
		timeout := false
		for (gotEG == false) && (timeout == false) {
			select {
			case egData = <-EGChan:
				gotEG = true
			case <-time.After(120 * time.Second):
				timeout = true
			}

		} // inner for
		if (timeout == false) && (gotEG == true) {
			//Process the data
		} else {
			stopPW <- 1
		}
		time.Sleep(500*time.Millisecond)

	} //outer for

}
