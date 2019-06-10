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

	eg.Host = configReader.GetString("powerwall.host")
	eg.Sn = configReader.GetString("powerwall.sn")
	eg.User = configReader.GetString("powerwall.user")
	fmt.Printf("SolarW,HouseW,BattW,GridW,BattCharge,SolarF,HouseF,BattF,GridF\n")
	stopPW := make(chan int, 1)
	go eg.PollData(EGChan, stopPW)
	for {
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
			//if (egData.Grid_up == true) {
			//	fmt.Printf("Grid is up\n")
			//} else {
			//	fmt.Printf("Grid is DOWN\n")
			//}
			fmt.Printf("%.5g,%.5g,%.5g,%.5g,%5.4g,%5.4g,%5.4g,%5.4g,%5.4g\n",
				egData.Solar_instant_power, egData.House_instant_power,
				egData.Battery_instant_power, egData.Grid_instant_power,
				egData.Batt_percentage,
				egData.Solar_frequency, egData.House_frequency, egData.Battery_frequency,
				egData.Grid_frequency)
			//fmt.Printf("Solar output: %g\n",egData.Solar_instant_power)
			//fmt.Printf("House power: %g\n",egData.House_instant_power)
			//fmt.Printf("Battery power: %g\n",egData.Battery_instant_power)
			//fmt.Printf("Battery charge: %5.4g%%\n",egData.Batt_percentage)
			//fmt.Printf("Grid power: %g\n",egData.Grid_instant_power)

		} else {
			stopPW <- 1
		}
		time.Sleep(500 * time.Millisecond)

	} //outer for

}
