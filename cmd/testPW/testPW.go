package main

import (
        //"flag"
        "database/sql"
        "fmt"
        "github.com/mattn/go-sqlite3"
        "github.com/mikef5410/solarmon"
        "github.com/spf13/viper"
	"gopkg.in/resty.v1"
        "gopkg.in/yaml.v2"
        "io/ioutil"
        "time"
)

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
        egHost := configReader.GetString("powerwall.host")
	egsn := configReader.GetString("powerwall.sn")
	eguser := configReader.GetString("powerwall.user")

}

func poll() {

	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	cmd := fmt.Sprintf(``)

}
