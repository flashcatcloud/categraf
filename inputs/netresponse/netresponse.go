package netresponse

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const (
	inputName = "netresponse"

	Success          uint64 = 0
	Timeout          uint64 = 1
	ConnectionFailed uint64 = 2
	ReadFailed       uint64 = 3
	StringMismatch   uint64 = 4
)

type Instance struct {
	Targets       []string          `toml:"targets"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
	Protocol      string            `toml:"protocol"`
	Timeout       config.Duration   `toml:"timeout"`
	ReadTimeout   config.Duration   `toml:"read_timeout"`
	Send          string            `toml:"send"`
	Expect        string            `toml:"expect"`
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return errors.New("targets empty")
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
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`
	Counter   uint64
	wg        sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &NetResponse{}
	})
}

func (n *NetResponse) Prefix() string {
	return inputName
}

func (n *NetResponse) GetInterval() config.Duration {
	return n.Interval
}

func (n *NetResponse) Init() error {
	if len(n.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(n.Instances); i++ {
		if err := n.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (n *NetResponse) Drop() {}

func (n *NetResponse) Gather(slist *list.SafeList) {
	atomic.AddUint64(&n.Counter, 1)
	for i := range n.Instances {
		ins := n.Instances[i]
		n.wg.Add(1)
		go n.gatherOnce(slist, ins)
	}
	n.wg.Wait()
}

func (n *NetResponse) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer n.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&n.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	if config.Config.DebugMode {
		if len(ins.Targets) == 0 {
			log.Println("D! net_response targets empty")
		}
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

func (ins *Instance) gather(slist *list.SafeList, target string) {
	if config.Config.DebugMode {
		log.Println("D! net_response... target:", target)
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// should never happen
		log.Println("E! failed to split host port, target:", target, "error:", err)
		return
	}

	labels := map[string]string{"server": host, "port": port}
	for k, v := range ins.Labels {
		labels[k] = v
	}

	fields := map[string]interface{}{}

	defer func() {
		for field, value := range fields {
			slist.PushFront(inputs.NewSample(field, value, labels))
		}
	}()

	var returnTags map[string]string

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
