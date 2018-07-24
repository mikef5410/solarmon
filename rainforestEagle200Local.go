package solarmon

import (
	"fmt"
	"gopkg.in/resty.v1"
	"crypto/tls"
	"encoding/xml"
)

type Device struct {
        XMLName xml.Name `xml:"Device"`
	HardwareAddress string
	Manufacturer string
	ModelId string
	Protocol string
	LastContact string
	ConnectionStatus string
	NetworkAddress string
}

type DeviceList struct {
     XMLName xml.Name `xml:"DeviceList"`
     Devices []Device `xml:"Device"`
     }       

type RainforestEagle200Local struct {
	Host              string
	User              string
	Pass              string
	MeterHardwareAddr string
}

type AuthSuccess struct {
	ID, Message string
}
type AuthError struct {
	ID, Message string
}
func (self *RainforestEagle200Local) Setup() {
/*	deviceList := DeviceList {
                   Devices: []Device {
                            Device {  },
                   },
        }
*/
        var deviceList DeviceList
        
	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	resp,err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%s", resp.Body())

	if err:= xml.Unmarshal(resp.Body(),&deviceList); err != nil {
		fmt.Printf("Client unmarhal failed: " + err.Error())
	} else {
		fmt.Printf("%q\n",deviceList)
                fmt.Printf("%s\n",deviceList.Devices[0].HardwareAddress)
               	}
}

func (self *RainforestEagle200Local) GetData() {
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	hardwareAddr:="0x0013500100dad717"
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

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v\n",resp)
}
