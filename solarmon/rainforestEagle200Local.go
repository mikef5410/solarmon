package solarmon

import (
	"fmt"
	"gopkg.in/resty.v1"
	"crypto/tls"
)

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
	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	resp,err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v", resp)
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
