package main

import (
	//"encoding/hex"
	"fmt"
	"github.com/spf13/viper"
	//"os"
	"time"
	"github.com/mikef5410/solarmon"
)

func main() {
	var inv solarmon.SolarEdgeModbus

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

	fmt.Printf("Connect to: %s, Port %d\n", configReader.GetString("inverter.host"), configReader.GetInt("inverter.port"))

	inv.Host = configReader.GetString("inverter.host")
	inv.Port = uint16(configReader.GetInt("inverter.port"))

	inv.AllRegDump()
	pollms := time.Duration(configReader.GetInt("inverter.pollInterval")) * time.Millisecond
	j := 0
	for j < 10 {
		//x := inv.getReg("C_SerialNumber")
		//fmt.Printf("S/N: %s\n", x.strval)

		power := inv.GetReg("I_AC_Power")
		fmt.Printf("Power out = %8.5g %s\n", power.Value, power.Units)

		v := inv.GetReg("I_AC_VoltageAB")
		fmt.Printf("Voltage = %8.5g %s\n", v.Value, v.Units)

		i := inv.GetReg("I_AC_Current")
		fmt.Printf("Current = %8.5g %s\n", i.Value, i.Units)

		time.Sleep(pollms)
		j++
	}
}
