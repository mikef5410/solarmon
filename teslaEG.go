package solarmon

import (
        //"flag"
        "fmt"
	"gopkg.in/resty.v1"
	"encoding/json"
	"strconv"
	"strings"
	"crypto/tls"
	"time"
)

type TeslaEnergyGateway struct {
	Host      string
	Sn        string
	User      string
}

//Batt <0 ... charging
//Solar >0 ... generating
//House >0 ... consuming
//Grid  <0 ... exporting

// Current performance data is stuffed in here
type EGPerfData struct {
	Grid_status string //GridStatus "SystemIsIslandedActive" or "SystemGridConnected"
	Grid_up bool
	Grid_last_change time.Time
	Grid_services_active bool
	
	Uptime     uint64  //sitemaster
	Running    bool
	Connected_to_tesla bool

	Batt_percentage float64 //SOE

	Solar_energy_imported float64 //meters
	Solar_energy_exported float64
        Solar_instant_power float64
	Solar_instant_apparent_power float64
	Solar_instant_reactive_power float64
	Solar_frequency float64
	Solar_last_communication_time string
	Solar_instant_average_voltage float64

	Grid_energy_imported float64
	Grid_energy_exported float64
        Grid_instant_power float64
	Grid_instant_apparent_power float64
	Grid_instant_reactive_power float64
	Grid_frequency float64
	Grid_last_communication_time string
	Grid_instant_average_voltage float64
	
	House_energy_imported float64
	House_energy_exported float64
        House_instant_power float64
	House_instant_apparent_power float64
	House_instant_reactive_power float64
	House_frequency float64
	House_last_communication_time string
	House_instant_average_voltage float64

	Battery_energy_imported float64
	Battery_energy_exported float64
        Battery_instant_power float64
	Battery_instant_apparent_power float64
	Battery_instant_reactive_power float64
	Battery_frequency float64
	Battery_last_communication_time string
	Battery_instant_average_voltage float64
	Battery_instant_total_current float64
}

func (EG *TeslaEnergyGateway) getSOE(data *EGPerfData) {
	type SOEdata struct {
		Percentage float64
	}
	var d SOEdata
        url:="https://"+EG.Host+"/api/system_status/soe"
	resp,err := resty.R().Get(url)
	if err != nil {
		fmt.Println(fmt.Errorf("getSOE failure: %s\n",err))
	}
	json.Unmarshal([]byte(resp.Body()),&d)
	data.Batt_percentage=d.Percentage
	return
}

func (EG *TeslaEnergyGateway) getSiteMaster(data *EGPerfData) {
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
	json.Unmarshal([]byte(resp.Body()),&d)
	data.Uptime,err=strconv.ParseUint(strings.TrimSuffix(d.Uptime,"s"),10,64)
	data.Running=d.Running
	data.Connected_to_tesla=d.Connected_to_tesla
	return
}

func (EG *TeslaEnergyGateway) getGridStatus(data *EGPerfData) {
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
	json.Unmarshal([]byte(resp.Body()),&d)
	data.Grid_status=d.Grid_status //SystemGridConnected, SystemIslandedActive, SystemTransitionToGrid
	data.Grid_services_active=d.Grid_services_active
	up := (0 == strings.Compare("SystemGridConnected",d.Grid_status))
	data.Grid_up=up
	return
}

func (EG *TeslaEnergyGateway) getMeters(data *EGPerfData) {
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
	json.Unmarshal([]byte(resp.Body()),&d)
	data.Solar_energy_imported=d.Solar.Energy_imported
	data.Solar_energy_exported=d.Solar.Energy_exported
	data.Solar_instant_power=d.Solar.Instant_power
	data.Solar_instant_apparent_power=d.Solar.Instant_apparent_power
	data.Solar_instant_reactive_power=d.Solar.Instant_reactive_power
	data.Solar_frequency=d.Solar.Frequency
	data.Solar_instant_average_voltage=d.Solar.Instant_average_voltage
	data.Solar_last_communication_time=d.Solar.Last_communication_time

	data.Grid_energy_imported=d.Site.Energy_imported
	data.Grid_energy_exported=d.Site.Energy_exported
	data.Grid_instant_power=d.Site.Instant_power
	data.Grid_instant_apparent_power=d.Site.Instant_apparent_power
	data.Grid_instant_reactive_power=d.Site.Instant_reactive_power
	data.Grid_frequency=d.Site.Frequency
	data.Grid_instant_average_voltage=d.Site.Instant_average_voltage
	data.Grid_last_communication_time=d.Site.Last_communication_time

	data.Battery_energy_imported=d.Battery.Energy_imported
	data.Battery_energy_exported=d.Battery.Energy_exported
	data.Battery_instant_power=d.Battery.Instant_power
	data.Battery_instant_apparent_power=d.Battery.Instant_apparent_power
	data.Battery_instant_reactive_power=d.Battery.Instant_reactive_power
	data.Battery_frequency=d.Battery.Frequency
	data.Battery_instant_average_voltage=d.Battery.Instant_average_voltage
	data.Battery_last_communication_time=d.Battery.Last_communication_time

	data.House_energy_imported=d.Load.Energy_imported
	data.House_energy_exported=d.Load.Energy_exported
	data.House_instant_power=d.Load.Instant_power
	data.House_instant_apparent_power=d.Load.Instant_apparent_power
	data.House_instant_reactive_power=d.Load.Instant_reactive_power
	data.House_frequency=d.Load.Frequency
	data.House_instant_average_voltage=d.Load.Instant_average_voltage
	data.House_last_communication_time=d.Load.Last_communication_time
}


//Get a complete set of data, stuff it into a struct, push the struct onto the data channel
//and return.
func (EG *TeslaEnergyGateway) PollData(EGChannel chan EGPerfData, stopChan chan int) {
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

		case <-stopChan:
			return
		}
	}

}

	
