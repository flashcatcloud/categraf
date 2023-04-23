package net_response

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "net_response"

	Success          uint64 = 0
	Timeout          uint64 = 1
	ConnectionFailed uint64 = 2
	ReadFailed       uint64 = 3
	StringMismatch   uint64 = 4
)

type Instance struct {
	config.InstanceConfig

	Targets     []string        `toml:"targets"`
	Protocol    string          `toml:"protocol"`
	Timeout     config.Duration `toml:"timeout"`
	ReadTimeout config.Duration `toml:"read_timeout"`
	Send        string          `toml:"send"`
	Expect      string          `toml:"expect"`
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.Protocol == "" {
		ins.Protocol = "tcp"
	}

	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(time.Second)
	}

	if ins.ReadTimeout == 0 {
		ins.ReadTimeout = config.Duration(time.Second)
	}

	if ins.Protocol == "udp" && ins.Send == "" {
		return errors.New("send string cannot be empty when protocol is udp")
	}

	if ins.Protocol == "udp" && ins.Expect == "" {
		return errors.New("expected string cannot be empty when protocol is udp")
	}

	for i := 0; i < len(ins.Targets); i++ {
		target := ins.Targets[i]

		host, port, err := net.SplitHostPort(target)
		if err != nil {
			return fmt.Errorf("failed to split host port, target: %s, error: %v", target, err)
		}

		if host == "" {
			ins.Targets[i] = "localhost:" + port
		}

		if port == "" {
			return errors.New("bad port, target: " + target)
		}
	}

	return nil
}

type NetResponse struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NetResponse{}
	})
}

func (n *NetResponse) Clone() inputs.Input {
	return &NetResponse{}
}

func (n *NetResponse) Name() string {
	return inputName
}

func (n *NetResponse) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(n.Instances))
	for i := 0; i < len(n.Instances); i++ {
		ret[i] = n.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Targets) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	for _, target := range ins.Targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ins.gather(slist, target)
		}(target)
	}
	wg.Wait()
}

func (ins *Instance) gather(slist *types.SampleList, target string) {
	if config.Config.DebugMode {
		log.Println("D! net_response... target:", target)
	}

	labels := map[string]string{"target": target}
	fields := map[string]interface{}{}

	defer func() {
		for field, value := range fields {
			slist.PushFront(types.NewSample(inputName, field, value, labels))
		}
	}()

	var returnTags map[string]string
	var err error

	switch ins.Protocol {
	case "tcp":
		returnTags, fields, err = ins.TCPGather(target)
		if err != nil {
			log.Println("E! failed to gather:", target, "error:", err)
			return
		}
		labels["protocol"] = "tcp"
	case "udp":
		returnTags, fields, err = ins.UDPGather(target)
		if err != nil {
			log.Println("E! failed to gather:", target, "error:", err)
			return
		}
		labels["protocol"] = "udp"
	default:
		log.Println("E! bad protocol, target:", target)
	}

	for k, v := range returnTags {
		labels[k] = v
	}
}

func (ins *Instance) TCPGather(address string) (map[string]string, map[string]interface{}, error) {
	// Prepare returns
	tags := make(map[string]string)
	fields := make(map[string]interface{})

	// Start Timer
	start := time.Now()
	// Connecting
	conn, err := net.DialTimeout("tcp", address, time.Duration(ins.Timeout))
	// Stop timer
	responseTime := time.Since(start).Seconds()
	// Handle error
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			fields["result_code"] = Timeout
		} else {
			fields["result_code"] = ConnectionFailed
		}
		return tags, fields, nil
	}
	defer conn.Close()

	// Send string if needed
	if ins.Send != "" {
		msg := []byte(ins.Send)
		if _, gerr := conn.Write(msg); gerr != nil {
			return nil, nil, gerr
		}
		// Stop timer
		responseTime = time.Since(start).Seconds()
	}
	// Read string if needed
	if ins.Expect != "" {
		// Set read timeout
		if gerr := conn.SetReadDeadline(time.Now().Add(time.Duration(ins.ReadTimeout))); gerr != nil {
			return nil, nil, gerr
		}
		// Prepare reader
		reader := bufio.NewReader(conn)
		tp := textproto.NewReader(reader)
		// Read
		data, err := tp.ReadLine()
		// Stop timer
		responseTime = time.Since(start).Seconds()
		// Handle error
		if err != nil {
			log.Printf("E! read tcp failed, address: %s, error: %s", address, err)
			fields["result_code"] = ReadFailed
		} else {
			if strings.Contains(data, ins.Expect) {
				fields["result_code"] = Success
			} else {
				fields["result_code"] = StringMismatch
			}
		}
	} else {
		fields["result_code"] = Success
	}
	fields["response_time"] = responseTime
	return tags, fields, nil
}

// UDPGather will execute if there are UDP tests defined in the configuration.
// It will return a map[string]interface{} for fields and a map[string]string for tags
func (ins *Instance) UDPGather(address string) (map[string]string, map[string]interface{}, error) {
	// Prepare returns
	tags := make(map[string]string)
	fields := make(map[string]interface{})

	// Start Timer
	start := time.Now()
	// Resolving
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	// Handle error
	if err != nil {
		fields["result_code"] = ConnectionFailed
		// Error encoded in result
		//nolint:nilerr
		return tags, fields, nil
	}
	// Connecting
	conn, err := net.DialUDP("udp", nil, udpAddr)
	// Handle error
	if err != nil {
		fields["result_code"] = ConnectionFailed
		// Error encoded in result
		//nolint:nilerr
		return tags, fields, nil
	}
	defer conn.Close()
	// Send string
	msg := []byte(ins.Send)
	if _, gerr := conn.Write(msg); gerr != nil {
		return nil, nil, gerr
	}
	// Read string
	// Set read timeout
	if gerr := conn.SetReadDeadline(time.Now().Add(time.Duration(ins.ReadTimeout))); gerr != nil {
		return nil, nil, gerr
	}
	// Read
	buf := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buf)
	// Stop timer
	responseTime := time.Since(start).Seconds()
	// Handle error
	if err != nil {
		log.Printf("E! read udp failed, address: %s, error: %s", address, err)
		fields["result_code"] = ReadFailed
		// Error encoded in result
		//nolint:nilerr
		return tags, fields, nil
	}

	if strings.Contains(string(buf), ins.Expect) {
		fields["result_code"] = Success
	} else {
		fields["result_code"] = StringMismatch
	}

	fields["response_time"] = responseTime

	return tags, fields, nil
}
