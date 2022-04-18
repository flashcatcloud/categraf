package net

import (
	"fmt"
	"log"
	"net"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const inputName = "net"

type NetIOStats struct {
	ps system.PS

	PrintConfigs         bool            `toml:"print_configs"`
	Interval             config.Duration `toml:"interval"`
	CollectProtocolStats bool            `toml:"collect_protocol_stats"`
	Interfaces           []string        `toml:"interfaces"`

	interfaceFilters filter.Filter
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(inputName, func() inputs.Input {
		return &NetIOStats{
			ps: ps,
		}
	})
}

func (s *NetIOStats) GetInputName() string {
	return inputName
}

func (s *NetIOStats) GetInterval() config.Duration {
	return s.Interval
}

func (s *NetIOStats) Drop() {}

func (s *NetIOStats) Init() error {
	var err error

	if len(s.Interfaces) > 0 {
		s.interfaceFilters, err = filter.Compile(s.Interfaces)
		if err != nil {
			return fmt.Errorf("error compiling interfaces filter: %s", err)
		}
	}

	return nil
}

func (s *NetIOStats) Gather() []*types.Sample {
	netio, err := s.ps.NetIO()
	if err != nil {
		log.Println("E! failed to get net io metrics:", err)
		return nil
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		log.Println("E! failed to list interfaces:", err)
		return nil
	}

	interfacesByName := map[string]net.Interface{}
	for _, iface := range interfaces {
		interfacesByName[iface.Name] = iface
	}

	var samples []*types.Sample

	for _, io := range netio {
		if len(s.Interfaces) > 0 {
			var found bool

			if s.interfaceFilters.Match(io.Name) {
				found = true
			}

			if !found {
				continue
			}
		}

		iface, ok := interfacesByName[io.Name]
		if !ok {
			continue
		}

		if iface.Flags&net.FlagLoopback == net.FlagLoopback {
			continue
		}

		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		tags := map[string]string{
			"interface": io.Name,
		}

		fields := map[string]interface{}{
			"bytes_sent":   io.BytesSent,
			"bytes_recv":   io.BytesRecv,
			"packets_sent": io.PacketsSent,
			"packets_recv": io.PacketsRecv,
			"err_in":       io.Errin,
			"err_out":      io.Errout,
			"drop_in":      io.Dropin,
			"drop_out":     io.Dropout,
		}

		samples = append(samples, inputs.NewSamples(fields, tags)...)
	}

	return samples
}
