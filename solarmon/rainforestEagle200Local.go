package solarmon

import (
	"fmt"
	"gopkg.in/resty.v1"
)

type rainforestEagle200Local struct {
	host              string
	user              string
	pass              string
	meterHardwareAddr string
}

func (self *rainforestEagle200Local) setup() {
	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")

	resp, err:= resty.R().SetBasicAuth(self.user, self.pass).
		SetBody(cmd).
		//SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v", resp)
}

func (self *rainforestEagle200Local) getData() {

	cmd := fmt.Sprintf(`<Command>
<Name>device_query</Name>
<DeviceDetails>
<HardwareAddress>%s</HardwareAddress>
</DeviceDetails>
<Components>
<All>Y</All>
</Components>
</Command>`, self.meterHardwareAddr)

	resp, err := resty.R().SetBasicAuth(self.user, self.pass).
		SetBody(cmd).
		//SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.host))

	fmt.Printf("%s\n",fmt.Errorf("%s",err))
	fmt.Printf("%v\n",resp)
}
