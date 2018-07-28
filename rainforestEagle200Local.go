package solarmon

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"gopkg.in/resty.v1"
	"regexp"
	"strconv"
	"time"
)

type RainforestEagle200Local struct {
	Host              string
	User              string
	Pass              string
	MeterHardwareAddr string
}

type Device struct {
	XMLName          xml.Name `xml:"Device"`
	HardwareAddress  string
	Manufacturer     string
	ModelId          string
	Protocol         string
	LastContact      string
	ConnectionStatus string
	NetworkAddress   string
}

type DeviceList struct {
	XMLName xml.Name `xml:"DeviceList"`
	Devices []Device `xml:"Device"`
}

type DeviceDetailsDevice struct {
	XMLName    xml.Name      `xml:"Device"`
	Details    DeviceDetails `xml:"DeviceDetails"`
	Components []Component   `xml:"Components>Component"`
}

type Component struct {
	XMLName    xml.Name `xml:"Component"`
	HardwareId string
	FixedId    string
	Name       string
	Variables  []Variable `xml:"Variables>Variable"`
}

type DeviceDetails struct {
	XMLName          xml.Name `xml:"DeviceDetails"`
	Name             string
	HardwareAddress  string
	NetworkInterface string
	Protocol         string
	NetworkAddress   string
	Manufacturer     string
	ModelId          string
	LastContact      string
	ConnectionStatus string
}

type Variable struct {
	XMLName     xml.Name `xml:"Variable"`
	Name        string
	Value       string
	Units       string
	Description string
}

type AuthSuccess struct {
	ID, Message string
}
type AuthError struct {
	ID, Message string
}

type DataResponse struct {
	LastContact         time.Time
	InstantaneousDemand float64
	KWhFromGrid         float64
	KWhToGrid           float64
}

func (self *RainforestEagle200Local) Setup() {
	var deviceList DeviceList

	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	resp, err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	if err != nil {
		fmt.Printf("%s\n", fmt.Errorf("%s", err))
	}
	//fmt.Printf("%s", resp.Body())

	if err := xml.Unmarshal(resp.Body(), &deviceList); err != nil {
		fmt.Printf("Client unmarshal failed: " + err.Error())
	} else {
		//fmt.Printf("%v\n", deviceList)
		//fmt.Printf("%s\n", deviceList.Devices[0].HardwareAddress)
	}

	for ix, _ := range deviceList.Devices {
		if deviceList.Devices[ix].ModelId == "electric_meter" {
			self.MeterHardwareAddr = deviceList.Devices[ix].HardwareAddress
		}
	}
}

func (self *RainforestEagle200Local) GetData() DataResponse {
	var devDetails DeviceDetailsDevice

	retry := 10
	retryTime := time.Duration(10 * time.Second);
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	hardwareAddr := self.MeterHardwareAddr

	cmd := fmt.Sprintf(`<Command>
<Name>device_query</Name>
<DeviceDetails>
<HardwareAddress>%s</HardwareAddress>
</DeviceDetails>
<Components>
<All>Y</All>
</Components>
</Command>`, hardwareAddr)

	indexOfName := make(map[string]int)
	var LastContactStr string
	for retry > 0 {
		resp, err := resty.R().SetBasicAuth(self.User, self.Pass).
			SetBody(cmd).
			SetResult(AuthSuccess{}).
			Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

		if err != nil {
			fmt.Printf("%s\n", fmt.Errorf("%s", err))
			retry = retry - 1
			time.Sleep(retryTime)
			continue
		}                        

		re := regexp.MustCompile("&")
		fixedResp := re.ReplaceAllString(string(resp.Body()), " and ")
		//fmt.Printf("%s\n", fixedResp)

		if err := xml.Unmarshal([]byte(fixedResp), &devDetails); err != nil {
			fmt.Printf("Client unmarshal failed: " + err.Error())
			retry = retry - 1
			time.Sleep(retryTime)
			continue
		} 

                retry = 0 // We got here, so no errors
		LastContactStr = devDetails.Details.LastContact
		//fmt.Printf("Last Contact: %s\n", LastContactStr)

		//indexOfName = make(map[string]int)
		for ix, _ := range devDetails.Components[0].Variables {
			name := devDetails.Components[0].Variables[ix].Name
			indexOfName[name] = ix
		}
	}

	InstantaneousDemandStr := devDetails.Components[0].Variables[indexOfName["zigbee:InstantaneousDemand"]].Value
	KWhFromGridStr := devDetails.Components[0].Variables[indexOfName["zigbee:CurrentSummationDelivered"]].Value
	KWhToGridStr := devDetails.Components[0].Variables[indexOfName["zigbee:CurrentSummationReceived"]].Value

	var response DataResponse
	LastContactUnix, _ := strconv.ParseInt(LastContactStr, 0, 64)
	response.LastContact = time.Unix(LastContactUnix, 0)
	response.InstantaneousDemand, _ = strconv.ParseFloat(InstantaneousDemandStr, 64)
	response.KWhFromGrid, _ = strconv.ParseFloat(KWhFromGridStr, 64)
	response.KWhToGrid, _ = strconv.ParseFloat(KWhToGridStr, 64)

	return (response)

}

func (self *RainforestEagle200Local) PollData(gridChannel chan DataResponse) {
	gridChannel <- self.GetData()
}
