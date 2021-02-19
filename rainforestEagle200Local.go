package solarmon

/* Support for Rainforest Automation Eagle-200 Zigbee<->Smart Meter gateway
   The Eagle-200 implements a local REST API to get Smart Meter data. It's
   returned in XML format. We pull apart the XML, load a struct, and optionally
   send the result on a channel.
*/

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"golang.org/x/net/html/charset"
	"gopkg.in/resty.v1"
	"os"
	"regexp"
	"strconv"
	"strings"
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

//Read the device list from the gateway. Het the hardware address of the first electric_meter
func (self *RainforestEagle200Local) Setup() {
	var deviceList DeviceList

	cmd := fmt.Sprintf("<Command><Name>device_list</Name></Command>")
	client := resty.New()
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	resp, err := client.R().SetBasicAuth(self.User, self.Pass).
		SetBody(cmd).
		SetResult(AuthSuccess{}).
		Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

	if err != nil {
		fmt.Printf("%s\n", fmt.Errorf("%s", err))
	}

	//fmt.Printf("%s", resp.Body())

	//convert response to utf-8 string
	//response, err := ioutil.ReadAll(charmap.ISO8859_1.NewDecoder().Reader(bytes.NewReader(resp.Body())))
	decoder := xml.NewDecoder(bytes.NewReader(resp.Body()))
	decoder.CharsetReader = charset.NewReaderLabel
	if err = decoder.Decode(&deviceList); err != nil {
		//if err = xml.Unmarshal(response, &deviceList); err != nil {
		fmt.Printf("rainforest Setup Client unmarshal failed: " + err.Error())
	} else {
		//	//fmt.Printf("%v\n", deviceList)
		//	//fmt.Printf("%s\n", deviceList.Devices[0].HardwareAddress)
	}

	for ix, _ := range deviceList.Devices {
		if deviceList.Devices[ix].ModelId == "electric_meter" {
			self.MeterHardwareAddr = deviceList.Devices[ix].HardwareAddress
		}
	}
}

// Read available data from the meter. Return a struct loaded up with current data
func (self *RainforestEagle200Local) GetData() DataResponse {
	var devDetails DeviceDetailsDevice

	client := resty.New()
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	retry := 10
	ok := false
	retryTime := time.Duration(10 * time.Second)
	//resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
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
		resp, err := client.R().SetBasicAuth(self.User, self.Pass).
			SetBody(cmd).
			SetResult(AuthSuccess{}).
			Post(fmt.Sprintf("https://%s/cgi-bin/post_manager", self.Host))

		if err != nil {
			fmt.Printf("%s\n", fmt.Errorf("%s", err))
			retry = retry - 1
			time.Sleep(retryTime)
			continue
		}
		//convert response to utf-8 string
		//response, err := ioutil.ReadAll(charmap.ISO8859_1.NewDecoder().Reader(bytes.NewReader(resp.Body())))

		//fmt.Printf("%s\n", string(resp.Body()))
		//fmt.Printf("----------------------------\n")

		re := regexp.MustCompile("&")
		fixedResp := re.ReplaceAllString(string(resp.Body()), " and ")

		//fmt.Printf("%s\n", fixedResp)
		//fmt.Printf("----------------------------\n")

		decoder := xml.NewDecoder(strings.NewReader(fixedResp))
		decoder.CharsetReader = charset.NewReaderLabel
		if err := decoder.Decode(&devDetails); err != nil {
			//if err := xml.Unmarshal([]byte(fixedResp), &devDetails); err != nil {
			fmt.Printf("rainforest getData Client unmarshal failed: " + err.Error())
			retry = retry - 1
			time.Sleep(retryTime)
			continue
		}

		retry = 0 // We got here, so no errors
		ok = true
		LastContactStr = devDetails.Details.LastContact
		//fmt.Printf("Last Contact: %s\n", LastContactStr)

		//indexOfName = make(map[string]int)
		for ix, _ := range devDetails.Components[0].Variables {
			name := devDetails.Components[0].Variables[ix].Name
			indexOfName[name] = ix
		}
	}

	if !ok {
		fmt.Printf("Too many Rainforest eagle errors. Exiting.\n")
		os.Exit(1)
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

// Goroutine to read one set of data, push it on a channel, and return
func (self *RainforestEagle200Local) PollData(interval_ms int, gridChannel chan DataResponse, stopChan chan int) {
	for {
		select {
		default:
			gridChannel <- self.GetData()
			//return
		case <-stopChan:
			return
		}
		time.Sleep(time.Duration(interval_ms) * time.Millisecond)
	}
}
