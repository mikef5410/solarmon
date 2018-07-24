package solarmon

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"gopkg.in/resty.v1"
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
	Variables        []Variable
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

func (self *RainforestEagle200Local) Setup() {
	var deviceList DeviceList

	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	resp, err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n", fmt.Errorf("%s", err))
	fmt.Printf("%s", resp.Body())

	if err := xml.Unmarshal(resp.Body(), &deviceList); err != nil {
		fmt.Printf("Client unmarshal failed: " + err.Error())
	} else {
		fmt.Printf("%v\n", deviceList)
		fmt.Printf("%s\n", deviceList.Devices[0].HardwareAddress)
	}
}

func (self *RainforestEagle200Local) GetData() {
	var devDetails DeviceDetails

	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	hardwareAddr := "0x0013500100dad717"
	cmd := fmt.Sprintf(`<Command>
<Name>device_query</Name>
<DeviceDetails>
<HardwareAddress>%s</HardwareAddress>
</DeviceDetails>
<Components>
<All>Y</All>
</Components>
</Command>`, hardwareAddr)

	resp, err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n", fmt.Errorf("%s", err))
	fmt.Printf("%v\n", resp)

	if err := xml.Unmarshal(resp.Body(), &devDetails); err != nil {
		fmt.Printf("Client unmarshal failed: " + err.Error())
	} else {
		fmt.Printf("%v\n", devDetails)
	}

}
