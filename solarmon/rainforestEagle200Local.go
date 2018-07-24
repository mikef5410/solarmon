package solarmon

import (
	"fmt"
	"gopkg.in/resty.v1"
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

	resp, err:= resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v", resp)
}

func (self *RainforestEagle200Local) GetData() {

	cmd := fmt.Sprintf(`<Command>
<Name>device_query</Name>
<DeviceDetails>
<HardwareAddress>%s</HardwareAddress>
</DeviceDetails>
<Components>
<All>Y</All>
</Components>
</Command>`, self.MeterHardwareAddr)

	resp, err := resty.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v\n",resp)
}
