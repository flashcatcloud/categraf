package modbus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	mb "github.com/grid-x/modbus"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const (
	inputName         = "modbus"
	cDiscreteInputs   = "discrete_input"
	cCoils            = "coil"
	cHoldingRegisters = "holding_register"
	cInputRegisters   = "input_register"
)

var errAddressOverflow = errors.New("address overflow")

type Modbus struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Modbus{}
	})
}

func (m *Modbus) Clone() inputs.Input {
	return &Modbus{}
}

func (m *Modbus) Name() string {
	return inputName
}

func (m *Modbus) GetInstances() []inputs.Instance {
	res := make([]inputs.Instance, len(m.Instances))
	for i := 0; i < len(m.Instances); i++ {
		res[i] = m.Instances[i]
	}
	return res
}

type workarounds struct {
	AfterConnectPause          config.Duration `toml:"pause_after_connect"`
	PollPause                  config.Duration `toml:"pause_between_requests"`
	CloseAfterGather           bool            `toml:"close_connection_after_gather"`
	ReadCoilsStartingAtZero    bool            `toml:"read_coils_starting_at_zero"`
	StringRegisterLocation     string          `toml:"string_register_location"`
	OnRequestPerField          bool            `toml:"one_request_per_field"`
	MaxBitRegistersPerRequest  uint16          `toml:"max_bit_registers_per_request"`
	MaxWordRegistersPerRequest uint16          `toml:"max_word_registers_per_request"`
}

type rs485Config struct {
	DelayRtsBeforeSend config.Duration `toml:"delay_rts_before_send"`
	DelayRtsAfterSend  config.Duration `toml:"delay_rts_after_send"`
	RtsHighDuringSend  bool            `toml:"rts_high_during_send"`
	RtsHighAfterSend   bool            `toml:"rts_high_after_send"`
	RxDuringTx         bool            `toml:"rx_during_tx"`
}

type fieldConverterFunc func(bytes []byte) interface{}

type requestSet struct {
	coil     []request
	discrete []request
	holding  []request
	input    []request
}

func (r requestSet) empty() bool {
	l := len(r.coil)
	l += len(r.discrete)
	l += len(r.holding)
	l += len(r.input)
	return l == 0
}

type field struct {
	measurement string
	name        string
	address     uint16
	length      uint16
	omit        bool
	converter   fieldConverterFunc
	value       interface{}
	tags        map[string]string
}

type Instance struct {
	config.InstanceConfig

	Name                   string          `toml:"name"`
	Controller             string          `toml:"controller"`
	TransmissionMode       string          `toml:"transmission_mode"`
	BaudRate               int             `toml:"baud_rate"`
	DataBits               int             `toml:"data_bits"`
	Parity                 string          `toml:"parity"`
	StopBits               int             `toml:"stop_bits"`
	RS485                  *rs485Config    `toml:"rs485"`
	Timeout                config.Duration `toml:"timeout"`
	Retries                int             `toml:"busy_retries"`
	RetriesWaitTime        config.Duration `toml:"busy_retries_wait"`
	DebugConnection        bool            `toml:"debug_connection"`
	Workarounds            workarounds     `toml:"workarounds"`
	ConfigurationType      string          `toml:"configuration_type"`
	ExcludeRegisterTypeTag bool            `toml:"exclude_register_type_tag"`

	// configuration type specific settings
	configurationOriginal
	configurationPerRequest
	configurationPerMetric

	// Connection handling
	client      mb.Client
	handler     mb.ClientHandler
	isConnected bool
	// Request handling
	requests map[byte]requestSet
}

func (ins *Instance) Init() error {
	if ins.Name == "" {
		return errors.New("device name is empty")
	}

	if ins.Retries < 0 {
		return fmt.Errorf("retries cannot be negative in device %q", ins.Name)
	}

	if ins.Workarounds.MaxBitRegistersPerRequest > maxQuantityCoils {
		return fmt.Errorf("maximum number of bit-registers cannot exceed %d", maxQuantityCoils)
	}
	if ins.Workarounds.MaxWordRegistersPerRequest > maxQuantityHoldingRegisters {
		return fmt.Errorf("maximum number of word-registers cannot exceed %d", maxQuantityHoldingRegisters)
	}

	var cfg configuration
	switch ins.ConfigurationType {
	case "", "register":
		ins.configurationOriginal.workarounds = ins.Workarounds
		cfg = &ins.configurationOriginal
	case "request":
		ins.configurationPerRequest.workarounds = ins.Workarounds
		ins.configurationPerRequest.excludeRegisterType = ins.ExcludeRegisterTypeTag
		cfg = &ins.configurationPerRequest
	case "metric":
		ins.configurationPerMetric.workarounds = ins.Workarounds
		ins.configurationPerMetric.excludeRegisterType = ins.ExcludeRegisterTypeTag
		cfg = &ins.configurationPerMetric
	default:
		return fmt.Errorf("unknown configuration type %q in device %q", ins.ConfigurationType, ins.Name)
	}

	if err := cfg.check(); err != nil {
		return fmt.Errorf("configuration invalid for device %q: %w", ins.Name, err)
	}

	r, err := cfg.process()
	if err != nil {
		return fmt.Errorf("cannot process configuration for device %q: %w", ins.Name, err)
	}
	ins.requests = r

	if err := ins.initClient(); err != nil {
		return fmt.Errorf("initializing client failed for controller %q: %w", ins.Controller, err)
	}

	for slaveID, rqs := range ins.requests {
		var nHoldingRegs, nInputsRegs, nDiscreteRegs, nCoilRegs uint16
		var nHoldingFields, nInputsFields, nDiscreteFields, nCoilFields int

		for _, r := range rqs.holding {
			nHoldingRegs += r.length
			nHoldingFields += len(r.fields)
		}
		for _, r := range rqs.input {
			nInputsRegs += r.length
			nInputsFields += len(r.fields)
		}
		for _, r := range rqs.discrete {
			nDiscreteRegs += r.length
			nDiscreteFields += len(r.fields)
		}
		for _, r := range rqs.coil {
			nCoilRegs += r.length
			nCoilFields += len(r.fields)
		}
		slog.Info("modbus request info", "holding_reqs", len(rqs.holding), "holding_regs", nHoldingRegs, "holding_fields", nHoldingFields, "slave_id", slaveID, "device", ins.Name)
		slog.Info("modbus request info", "input_reqs", len(rqs.input), "input_regs", nInputsRegs, "input_fields", nInputsFields, "slave_id", slaveID, "device", ins.Name)
		slog.Info("modbus request info", "discrete_reqs", len(rqs.discrete), "discrete_regs", nDiscreteRegs, "discrete_fields", nDiscreteFields, "slave_id", slaveID, "device", ins.Name)
		slog.Info("modbus request info", "coil_reqs", len(rqs.coil), "coil_regs", nCoilRegs, "coil_fields", nCoilFields, "slave_id", slaveID, "device", ins.Name)
	}

	return nil
}

func (ins *Instance) initClient() error {
	u, err := url.Parse(ins.Controller)
	if err != nil {
		return err
	}

	var tracelog mb.Logger
	if ins.DebugConnection || ins.DebugMod {
		tracelog = ins
	}

	switch u.Scheme {
	case "tcp":
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			return err
		}
		switch ins.TransmissionMode {
		case "", "auto", "TCP":
			handler := mb.NewTCPClientHandler(host + ":" + port)
			handler.Timeout = time.Duration(ins.Timeout)
			handler.Logger = tracelog
			ins.handler = handler
		case "RTUoverTCP":
			handler := mb.NewRTUOverTCPClientHandler(host + ":" + port)
			handler.Timeout = time.Duration(ins.Timeout)
			handler.Logger = tracelog
			ins.handler = handler
		case "ASCIIoverTCP":
			handler := mb.NewASCIIOverTCPClientHandler(host + ":" + port)
			handler.Timeout = time.Duration(ins.Timeout)
			handler.Logger = tracelog
			ins.handler = handler
		default:
			return fmt.Errorf("invalid transmission mode %q for %q on device %q", ins.TransmissionMode, u.Scheme, ins.Name)
		}
	case "", "file":
		path := filepath.Join(u.Host, u.Path)
		if path == "" {
			return fmt.Errorf("invalid path for controller %q", ins.Controller)
		}
		switch ins.TransmissionMode {
		case "", "auto", "RTU":
			handler := mb.NewRTUClientHandler(path)
			handler.Timeout = time.Duration(ins.Timeout)
			handler.BaudRate = ins.BaudRate
			handler.DataBits = ins.DataBits
			handler.Parity = ins.Parity
			handler.StopBits = ins.StopBits
			handler.Logger = tracelog
			if ins.RS485 != nil {
				handler.RS485.Enabled = true
				handler.RS485.DelayRtsBeforeSend = time.Duration(ins.RS485.DelayRtsBeforeSend)
				handler.RS485.DelayRtsAfterSend = time.Duration(ins.RS485.DelayRtsAfterSend)
				handler.RS485.RtsHighDuringSend = ins.RS485.RtsHighDuringSend
				handler.RS485.RtsHighAfterSend = ins.RS485.RtsHighAfterSend
				handler.RS485.RxDuringTx = ins.RS485.RxDuringTx
			}
			ins.handler = handler
		case "ASCII":
			handler := mb.NewASCIIClientHandler(path)
			handler.Timeout = time.Duration(ins.Timeout)
			handler.BaudRate = ins.BaudRate
			handler.DataBits = ins.DataBits
			handler.Parity = ins.Parity
			handler.StopBits = ins.StopBits
			handler.Logger = tracelog
			if ins.RS485 != nil {
				handler.RS485.Enabled = true
				handler.RS485.DelayRtsBeforeSend = time.Duration(ins.RS485.DelayRtsBeforeSend)
				handler.RS485.DelayRtsAfterSend = time.Duration(ins.RS485.DelayRtsAfterSend)
				handler.RS485.RtsHighDuringSend = ins.RS485.RtsHighDuringSend
				handler.RS485.RtsHighAfterSend = ins.RS485.RtsHighAfterSend
				handler.RS485.RxDuringTx = ins.RS485.RxDuringTx
			}
			ins.handler = handler
		default:
			return fmt.Errorf("invalid transmission mode %q for %q on device %q", ins.TransmissionMode, u.Scheme, ins.Name)
		}
	default:
		return fmt.Errorf("invalid controller %q", ins.Controller)
	}

	ins.client = mb.NewClient(ins.handler)
	ins.isConnected = false

	return nil
}

func (ins *Instance) connect() error {
	err := ins.handler.Connect(context.Background())
	ins.isConnected = err == nil
	if ins.isConnected && ins.Workarounds.AfterConnectPause != 0 {
		nextRequest := time.Now().Add(time.Duration(ins.Workarounds.AfterConnectPause))
		time.Sleep(time.Until(nextRequest))
	}
	return err
}

func (ins *Instance) disconnect() error {
	err := ins.handler.Close()
	ins.isConnected = false
	return err
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if !ins.isConnected {
		if err := ins.connect(); err != nil {
			slog.Error("failed to connect", "error", err, "controller", ins.Controller)
			return
		}
	}

	for slaveID, requests := range ins.requests {
		if ins.DebugMod {
			slog.Debug("reading slave", "slave_id", slaveID, "controller", ins.Controller)
		}
		if err := ins.readSlaveData(slaveID, requests); err != nil {
			slog.Error("slave read failed", "slave_id", slaveID, "controller", ins.Controller, "error", err)
			var mbErr *mb.Error
			if !errors.As(err, &mbErr) || mbErr.ExceptionCode != mb.ExceptionCodeServerDeviceBusy {
				if ins.DebugMod {
					slog.Debug("reconnecting", "controller", ins.Controller)
				}
				if err := ins.disconnect(); err != nil {
					slog.Error("disconnecting failed", "controller", ins.Controller, "error", err)
					return
				}
				if err := ins.connect(); err != nil {
					slog.Error("reconnecting failed", "slave_id", slaveID, "controller", ins.Controller, "error", err)
					return
				}
			}
			continue
		}

		tags := map[string]string{
			"name":     ins.Name,
			"slave_id": strconv.Itoa(int(slaveID)),
		}

		if !ins.ExcludeRegisterTypeTag {
			tags["type"] = cCoils
		}
		ins.collectFields(slist, tags, requests.coil)

		if !ins.ExcludeRegisterTypeTag {
			tags["type"] = cDiscreteInputs
		}
		ins.collectFields(slist, tags, requests.discrete)

		if !ins.ExcludeRegisterTypeTag {
			tags["type"] = cHoldingRegisters
		}
		ins.collectFields(slist, tags, requests.holding)

		if !ins.ExcludeRegisterTypeTag {
			tags["type"] = cInputRegisters
		}
		ins.collectFields(slist, tags, requests.input)
	}

	if ins.Workarounds.CloseAfterGather {
		_ = ins.disconnect()
	}
}

func (ins *Instance) readSlaveData(slaveID byte, requests requestSet) error {
	ins.handler.SetSlave(slaveID)

	for retry := 0; retry < ins.Retries; retry++ {
		err := ins.gatherFields(requests)
		if err == nil {
			return nil
		}

		var mbErr *mb.Error
		if !errors.As(err, &mbErr) || mbErr.ExceptionCode != mb.ExceptionCodeServerDeviceBusy {
			return err
		}

		time.Sleep(time.Duration(ins.RetriesWaitTime))
	}
	return ins.gatherFields(requests)
}

func (ins *Instance) gatherFields(requests requestSet) error {
	if err := ins.gatherRequestsCoil(requests.coil); err != nil {
		return err
	}
	if err := ins.gatherRequestsDiscrete(requests.discrete); err != nil {
		return err
	}
	if err := ins.gatherRequestsHolding(requests.holding); err != nil {
		return err
	}
	return ins.gatherRequestsInput(requests.input)
}

func (ins *Instance) gatherRequestsCoil(requests []request) error {
	for _, request := range requests {
		bytes, err := ins.client.ReadCoils(context.Background(), request.address, request.length)
		if err != nil {
			return err
		}
		nextRequest := time.Now().Add(time.Duration(ins.Workarounds.PollPause))

		for i, field := range request.fields {
			offset := field.address - request.address
			idx := offset / 8
			bit := offset % 8

			v := (bytes[idx] >> bit) & 0x01
			request.fields[i].value = field.converter([]byte{v})
		}

		time.Sleep(time.Until(nextRequest))
	}
	return nil
}

func (ins *Instance) gatherRequestsDiscrete(requests []request) error {
	for _, request := range requests {
		bytes, err := ins.client.ReadDiscreteInputs(context.Background(), request.address, request.length)
		if err != nil {
			return err
		}
		nextRequest := time.Now().Add(time.Duration(ins.Workarounds.PollPause))

		for i, field := range request.fields {
			offset := field.address - request.address
			idx := offset / 8
			bit := offset % 8

			v := (bytes[idx] >> bit) & 0x01
			request.fields[i].value = field.converter([]byte{v})
		}

		time.Sleep(time.Until(nextRequest))
	}
	return nil
}

func (ins *Instance) gatherRequestsHolding(requests []request) error {
	for _, request := range requests {
		bytes, err := ins.client.ReadHoldingRegisters(context.Background(), request.address, request.length)
		if err != nil {
			return err
		}
		nextRequest := time.Now().Add(time.Duration(ins.Workarounds.PollPause))

		for i, field := range request.fields {
			offset := 2 * uint32(field.address-request.address)
			length := 2 * uint32(field.length)

			request.fields[i].value = field.converter(bytes[offset : offset+length])
		}

		time.Sleep(time.Until(nextRequest))
	}
	return nil
}

func (ins *Instance) gatherRequestsInput(requests []request) error {
	for _, request := range requests {
		bytes, err := ins.client.ReadInputRegisters(context.Background(), request.address, request.length)
		if err != nil {
			return err
		}
		nextRequest := time.Now().Add(time.Duration(ins.Workarounds.PollPause))

		for i, field := range request.fields {
			offset := 2 * uint32(field.address-request.address)
			length := 2 * uint32(field.length)

			request.fields[i].value = field.converter(bytes[offset : offset+length])
		}

		time.Sleep(time.Until(nextRequest))
	}
	return nil
}

func (ins *Instance) collectFields(slist *types.SampleList, baseTags map[string]string, requests []request) {
	for _, request := range requests {
		for _, field := range request.fields {
			finalTags := make(map[string]string, len(baseTags)+len(field.tags))
			for k, v := range baseTags {
				finalTags[k] = v
			}
			for k, v := range field.tags {
				finalTags[k] = v
			}

			measurement := "modbus"
			if field.measurement != "" {
				measurement = field.measurement
			}
			metricName := measurement + "_" + field.name

			slist.PushFront(types.NewSample("", metricName, field.value, finalTags))
		}
	}
}

func (ins *Instance) Printf(format string, v ...interface{}) {
	slog.Debug(fmt.Sprintf(format, v...))
}
