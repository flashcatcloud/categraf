package net

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const InputName = "net"

type NetIOStats struct {
	quit chan struct{}
	ps   system.PS

	PrintConfigs         bool     `toml:"print_configs"`
	IntervalSeconds      int64    `toml:"interval_seconds"`
	CollectProtocolStats bool     `toml:"collect_protocol_stats"`
	Interfaces           []string `toml:"interfaces"`

	interfaceFilters filter.Filter
}

func init() {
	ps := system.NewSystemPS()
	inputs.Add(InputName, func() inputs.Input {
		return &NetIOStats{
			quit: make(chan struct{}),
			ps:   ps,
		}
	})
}

func (s *NetIOStats) getInterval() time.Duration {
	if s.IntervalSeconds != 0 {
		return time.Duration(s.IntervalSeconds) * time.Second
	}
	return config.GetInterval()
}

// overwrite func
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

// overwrite func
func (s *NetIOStats) StopGoroutines() {
	s.quit <- struct{}{}
}

// overwrite func
func (s *NetIOStats) StartGoroutines(queue chan *types.Sample) {
	go s.LoopGather(queue)
}

func (s *NetIOStats) LoopGather(queue chan *types.Sample) {
	interval := s.getInterval()
	for {
		select {
		case <-s.quit:
			close(s.quit)
			return
		default:
			time.Sleep(interval)
			s.GatherOnce(queue)
		}
	}
}

func (s *NetIOStats) GatherOnce(queue chan *types.Sample) {
	defer func() {
		if r := recover(); r != nil {
			if strings.Contains(fmt.Sprint(r), "closed channel") {
				return
			} else {
				log.Println("E! gather metrics panic:", r)
			}
		}
	}()

	samples := s.Gather()

	if len(samples) == 0 {
		return
	}

	now := time.Now()
	for i := 0; i < len(samples); i++ {
		samples[i].Timestamp = now
		samples[i].Metric = InputName + "_" + samples[i].Metric
		queue <- samples[i]
	}
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
