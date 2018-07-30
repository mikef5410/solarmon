package solarmon

import (
	"encoding/binary"
	//	"encoding/hex"
	"fmt"
	"github.com/goburrow/modbus"
	//	"log"
	"math"
	//	"os"
	"sort"
	"time"
)

type readBuf struct {
	baseAddr  uint32
	length    uint32
	timeStamp time.Time
	buffer    []byte
}

type SolarEdgeModbus struct {
	readBuf
	Host      string
	Port      uint16
	handler   *modbus.TCPClientHandler
	client    modbus.Client
	connected bool
}

type regInfo struct {
	addr      uint32
	datatype  uint16
	scaleAddr uint32
	strlen    uint32
	units     string
}

type RegResult struct {
	Value    float64
	Units    string
	Datatype uint16
	Strval   string
}

const StaleAge = 1 * time.Second
const IOTimeout = 3 * time.Second
const InverterSlaveID = 1

const (
	INT16   = 1
	UINT16  = 2
	UINT32  = 3
	FLOAT64 = 4
	STRING  = 5
)

var regAddr = map[string]*regInfo{
	"C_Manufacturer":  {40005, STRING, 0, 32, ""},
	"C_Model":         {40021, STRING, 0, 32, ""},
	"C_Version":       {40045, STRING, 0, 16, ""},
	"C_SerialNumber":  {40053, STRING, 0, 32, ""},
	"C_DeviceAddress": {40069, UINT16, 0, 0, ""},

	"I_AC_Current":  {40072, UINT16, 40076, 0, "A"},
	"I_AC_CurrentA": {40073, UINT16, 40076, 0, "A"},
	"I_AC_CurrentB": {40074, UINT16, 40076, 0, "A"},
	"I_AC_CurrentC": {40075, UINT16, 40076, 0, "A"},

	"I_AC_VoltageAB": {40077, UINT16, 40083, 0, "V"},
	"I_AC_VoltageBC": {40078, UINT16, 40083, 0, "V"},
	"I_AC_VoltageCA": {40079, UINT16, 40083, 0, "V"},

	"I_AC_VoltageAN": {40080, UINT16, 40083, 0, "V"},
	"I_AC_VoltageBN": {40081, UINT16, 40083, 0, "V"},
	"I_AC_VoltageCN": {40082, UINT16, 40083, 0, "V"},

	"I_AC_Power": {40084, INT16, 40085, 0, "W"},

	"I_AC_Frequency": {40086, UINT16, 40087, 0, "Hz"},

	"I_AC_VA":  {40088, INT16, 40089, 0, "VA"},
	"I_AC_VAR": {40090, INT16, 40091, 0, "VAR"},

	"I_AC_PF": {40092, INT16, 40093, 0, "%"},

	"I_AC_Energy": {40094, UINT32, 40096, 0, "Wh"},

	"I_DC_Current": {40097, INT16, 40098, 0, "A"},
	"I_DC_Voltage": {40099, UINT16, 40100, 0, "V"},
	"I_DC_Power":   {40101, INT16, 40102, 0, "W"},

	"I_Temp_Sink": {40104, INT16, 40107, 0, "℃"},

	"I_Status":         {40108, UINT16, 0, 0, ""},
	"I_Status_Vendor":  {40109, UINT16, 0, 0, ""},
	"I_Event_1_Vendor": {40114, UINT32, 0, 0, ""},
	"I_Event_4_Vendor": {40120, UINT32, 0, 0, ""},
}

type PerfData struct {
	AC_Power   float64
	AC_Current float64
	AC_Voltage float64
	AC_VA      float64
	AC_VAR     float64
	AC_PF      float64
	AC_Freq    float64
	AC_Energy  float64
	DC_Voltage float64
	DC_Current float64
	DC_Power   float64
	SinkTemp   float64
	Status     float64
	Event1     float64
}

func (inverter *SolarEdgeModbus) checkStale(addr uint32) bool {
	retry := 2
	var err error

	length := 138 //byte length
	if addr <= 40069 {
		addr = 40001
	} else {
		addr = 40070
		length = 104
	}

	for retry > 0 {
		if time.Since(inverter.timeStamp) >= StaleAge {
			err = inverter.get(addr, int(length))
		}
		if inverter.baseAddr != addr {
			err = inverter.get(addr, int(length))
		}
		if err != nil {
			retry--
			time.Sleep(2 * time.Second)
			if retry == 0 {
				inverter.handler.Close()
				inverter.connected = false
				return (false)
			}
			//fmt.Printf("retry\n")
		} else {
			retry = 0
		}
	}
	return (true)
}

func (inverter *SolarEdgeModbus) GetReg(name string) RegResult {
	result := RegResult{Value: 0, Units: "", Datatype: FLOAT64, Strval: ""}
	value := int32(0)
	if attribs, ok := regAddr[name]; ok {
		//Lookup was OK
		for inverter.checkStale(attribs.addr) == false {
			time.Sleep(1 * time.Second)
		}

		startAddr := 2 * (attribs.addr - inverter.baseAddr)

		switch attribs.datatype {
		case INT16:
			value = int32(int16(binary.BigEndian.Uint16(inverter.buffer[startAddr:])))
		case UINT16:
			value = int32(binary.BigEndian.Uint16(inverter.buffer[startAddr:]))
		case UINT32:
			value = int32(binary.BigEndian.Uint32(inverter.buffer[startAddr:]))
		case STRING:
			result.Strval = string(inverter.buffer[startAddr : startAddr+attribs.strlen-1])
			result.Datatype = STRING
		default:
			return (RegResult{0.0, "", 0, ""})

		}

		//Numeric result needs to be scaled.
		if result.Datatype != STRING {
			if attribs.scaleAddr > 0 {
				startAddr = 2 * (attribs.scaleAddr - inverter.baseAddr)
				scaleFact := int(int16(binary.BigEndian.Uint16(inverter.buffer[startAddr:])))
				result.Value = float64(value) * math.Pow10(scaleFact)
				result.Units = attribs.units
			} else { //Flag value
				result.Value = float64(value)
				result.Units = ""
			}
		}
	}
	return (result)
}

func (inverter *SolarEdgeModbus) checkConnection() (ok bool) {
	if inverter.connected {
		return (true)
	}
	inverter.handler = modbus.NewTCPClientHandler(fmt.Sprintf("%s:%d", inverter.Host, inverter.Port))
	inverter.handler.Timeout = IOTimeout
	inverter.handler.SlaveId = InverterSlaveID
	//inverter.handler.Logger = log.New(os.Stdout, "modbusio: ", log.LstdFlags)
	err := inverter.handler.Connect()
	if err == nil {
		inverter.connected = true
		ok = true
		inverter.client = modbus.NewClient(inverter.handler)
	} else {
		ok = false
	}
	return (ok)
}

func (inverter *SolarEdgeModbus) get(addr uint32, length int) (err error) {
	if inverter.checkConnection() {
		//fmt.Printf("Read %x for %d\n", addr, uint16(length/2))
		inverter.buffer, err = inverter.client.ReadHoldingRegisters(uint16(addr-1), uint16(length/2))
		if err == nil {
			inverter.timeStamp = time.Now()
			inverter.length = uint32(length)
			inverter.baseAddr = addr
			//fmt.Printf("%s",hex.Dump(inverter.buffer))
		}
	} else {
		err = fmt.Errorf("SolarEdgeModbus: read registers failed")
	}
	return (err)
}

func (inverter *SolarEdgeModbus) AllRegDump() {
	//Make an array of keys to sort
	keys := make([]string, 0, len(regAddr))
	for key := range regAddr {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	//Iterate over sorted keys
	for _, key := range keys {
		result := inverter.GetReg(key)

		switch result.Datatype {
		case INT16, UINT16, UINT32, FLOAT64:
			fmt.Printf("%s = %.8g %s\n", key, result.Value, result.Units)
		case STRING:
			fmt.Printf("%s = %s\n", key, result.Strval)
		default:

		}
	}
}

func (inverter *SolarEdgeModbus) PollData(inverterChannel chan PerfData, stopChan chan int) {
	var data PerfData

	for {
		select {
		default:
			data.AC_Power = inverter.GetReg("I_AC_Power").Value
			data.AC_Current = inverter.GetReg("I_AC_Current").Value
			data.AC_Voltage = inverter.GetReg("I_AC_VoltageAB").Value
			data.AC_VA = inverter.GetReg("I_AC_VA").Value
			data.AC_VAR = inverter.GetReg("I_AC_VAR").Value
			data.AC_PF = inverter.GetReg("I_AC_PF").Value
			data.AC_Freq = inverter.GetReg("I_AC_Frequency").Value
			data.AC_Energy = inverter.GetReg("I_AC_Energy").Value
			data.DC_Voltage = inverter.GetReg("I_DC_Voltage").Value
			data.DC_Current = inverter.GetReg("I_DC_Current").Value
			data.DC_Power = inverter.GetReg("I_DC_Power").Value
			data.SinkTemp = inverter.GetReg("I_Temp_Sink").Value
			data.Status = inverter.GetReg("I_Status_Vendor").Value
			data.Event1 = inverter.GetReg("I_Event_1_Vendor").Value

			inverterChannel <- data
			return

		case <-stopChan:
			return
		}
	}

}
