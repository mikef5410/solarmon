package solarmon

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
	"encoding/json"
	"strconv"
)

type teslaEnergyGateway struct {
	Host      string
	sn        string
	user      string
}

//Batt <0 ... charging
//Solar >0 ... generating
//House >0 ... consuming
//Grid  <0 ... exporting

// Current performance data is stuffed in here
type EGPerfData struct {
	grid_status string //gridStatus
	grid_services_active bool
	
	uptime     uint64  //sitemaster
	running    bool
	connected_to_tesla bool

	batt_percentage float64 //SOE

	solar_energy_imported float64 //meters
	solar_energy_exported float64
        solar_instant_power float64
	solar_instant_apparent_power float64
	solar_instant_reactive_power float64
	solar_frequency float64
	solar_last_communication_time string
	solar_instant_average_voltage float64

	grid_energy_imported float64
	grid_energy_exported float64
        grid_instant_power float64
	grid_instant_apparent_power float64
	grid_instant_reactive_power float64
	grid_frequency float64
	grid_last_communication_time string
	grid_instant_average_voltage float64
	
	house_energy_imported float64
	house_energy_exported float64
        house_instant_power float64
	house_instant_apparent_power float64
	house_instant_reactive_power float64
	house_frequency float64
	house_last_communication_time string
	house_instant_average_voltage float64

	battery_energy_imported float64
	battery_energy_exported float64
        battery_instant_power float64
	battery_instant_apparent_power float64
	battery_instant_reactive_power float64
	battery_frequency float64
	battery_last_communication_time string
	battery_instant_average_voltage float64
	battery_instant_total_current float64
}

func (EG *teslaEnergyGateway) getSOE(EGPerfData *data) {
	type SOEdata struct {
		Percentage float64
	}
	var d SOEdata
        url:="https://"+EG.Host+"/api/system_status/soe"
	resp,err := resty.R().Get(url)
	if err != nil {
		fmt.Println(fmt.Errorf("getSOE failure: %s\n",err))
	}
	json.Unmarshal([]byte(resp),&d)
	data.batt_percentage=d.Percentage
	return
}

func (EG *teslaEnergyGateway) getSiteMaster(EGPerfData *data) {
	type SiteMasterData struct {
		Running bool
		Uptime string
		Connected_to_tesla bool
	}
	var d SiteMasterData
        url:="https://"+EG.Host+"/api/sitemaster"
	resp,err := resty.R().Get(url)
	if err != nil {
		fmt.Println(fmt.Errorf("getSiteMaster failure: %s\n",err))
	}
	json.Unmarshal([]byte(resp),&d)
	data.uptime=strconv.ParseUint(strings.TrimSuffix(d.Uptime,"s"),10,64)
	data.running=d.Running
	data.connected_to_tesla=d.Connected_to_tesla
	return
}

func (EG *teslaEnergyGateway) getGridStatus(EGPerfData *data) {
	type GridStatus struct {
		Grid_status string
		Grid_services_active bool
	}
	var d GridStatus
        url:="https://"+EG.Host+"/api/system_status/grid_status"
	resp,err := resty.R().Get(url)
	if err != nil {
		fmt.Println(fmt.Errorf("getGridStatus failure: %s\n",err))
	}
	json.Unmarshal([]byte(resp),&d)
	data.grid_status=d.Grid_status //SystemGridConnected, SystemIslandedActive, SystemTransitionToGrid
	data.grid_services_active=d.Grid_services_active
	return
}

func (EG *teslaEnergyGateway) getMeters(EGPerfData *data) {
	type meterData struct {
		Instant_total_current float64
		I_b_current float64
		Energy_imported float64
		Last_communication_time string
		Instant_average_voltage float64
		Instant_power float64
		Instant_reactive_power float64
		I_c_current float64
		I_a_current float64
		Energy_exported float64
		Frequency float64
		Timeout float64
		Instant_apparent_power float64
	}
	type meterAggregate struct {
		Site meterData
		Solar meterData
		Load meterData
		Battery meterData
	}
	
	var d meterAggregate
	url:="https://"+EG.Host+"/api/meters/aggregates"
	resp,err := resty.R().Get(url)
	if err != nil {
		fmt.Println(fmt.Errorf("getMeters failure: %s\n",err))
	}
	json.Unmarshal([]byte(resp),&d)
	data.solar_energy_imported=d.Solar.Energy_imported
	data.solar_energy_exported=d.Solar.Energy_exported
	data.solar_instant_power=d.Solar.Instant_power
	data.solar_instant_apparent_power=d.Solar.Instant_apparent_power
	data.solar_instant_reactive_power=d.Solar.Instant_reactive_power
	data.solar_frequency=d.Solar.Frequency
	data.solar_instant_average_voltage=d.Solar.Instant_average_voltage
	data.solar_last_communication_time=d.Solar.last_communication_time

	data.grid_energy_imported=d.Site.Energy_imported
	data.grid_energy_exported=d.Site.Energy_exported
	data.grid_instant_power=d.Site.Instant_power
	data.grid_instant_apparent_power=d.Site.Instant_apparent_power
	data.grid_instant_reactive_power=d.Site.Instant_reactive_power
	data.grid_frequency=d.Site.Frequency
	data.grid_instant_average_voltage=d.Site.Instant_average_voltage
	data.grid_last_communication_time=d.Site.last_communication_time

	data.battery_energy_imported=d.Battery.Energy_imported
	data.battery_energy_exported=d.Battery.Energy_exported
	data.battery_instant_power=d.Battery.Instant_power
	data.battery_instant_apparent_power=d.Battery.Instant_apparent_power
	data.battery_instant_reactive_power=d.Battery.Instant_reactive_power
	data.battery_frequency=d.Battery.Frequency
	data.battery_instant_average_voltage=d.Battery.Instant_average_voltage
	data.battery_last_communication_time=d.Battery.last_communication_time

	data.house_energy_imported=d.Load.Energy_imported
	data.house_energy_exported=d.Load.Energy_exported
	data.house_instant_power=d.Load.Instant_power
	data.house_instant_apparent_power=d.Load.Instant_apparent_power
	data.house_instant_reactive_power=d.Load.Instant_reactive_power
	data.house_frequency=d.Load.Frequency
	data.house_instant_average_voltage=d.Load.Instant_average_voltage
	data.house_last_communication_time=d.Load.last_communication_time
}


//Get a complete set of data, stuff it into a struct, push the struct onto the data channel
//and return.
func (EG *teslaEnergyGateway) PollData(EGChannel chan EGPerfData, stopChan chan int) {
	var data EGPerfData

	resty.SetTLSClientConfig(&tls.Config{ InsecureSkipVerify: true })
	for {
		select {
		default:
			EG.getSOE(&data)
			EG.getSiteMaster(&data)
			EG.getGridStatus(&data)
			EG.getMeters(&data)
			EGChannel <- data
			return

		case <-stopChan:
			return
		}
	}

}
