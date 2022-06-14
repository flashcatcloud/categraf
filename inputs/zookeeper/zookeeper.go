package zookeeper

import (
	crypto_tls "crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const (
	inputName                 = "zookeeper"
	commandNotAllowedTmpl     = "warning: %q command isn't allowed at %q, see '4lw.commands.whitelist' ZK config parameter"
	instanceNotServingMessage = "This ZooKeeper instance is not currently serving requests"
	cmdNotExecutedSffx        = "is not executed because it is not in the whitelist."
)

var (
	versionRE          = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+).*$`)
	metricNameReplacer = strings.NewReplacer("-", "_", ".", "_")
)

type Instance struct {
	Address string            `toml:"address"`
	Timeout int               `toml:"timeout"`
	Labels  map[string]string `toml:"labels"`
	tls.ClientConfig
}

func (i *Instance) ZkConnect() (net.Conn, error) {
	dialer := net.Dialer{Timeout: time.Duration(i.Timeout) * time.Second}
	tcpaddr, err := net.ResolveTCPAddr("tcp", i.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve zookeeper address: %s: %v", i.Address, err)
	}

	if !i.UseTLS {
		return dialer.Dial("tcp", tcpaddr.String())
	}
	tlsConfig, err := i.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to init tls config: %v", err)
	}
	return crypto_tls.DialWithDialer(&dialer, "tcp", tcpaddr.String(), tlsConfig)
}

type Zookeeper struct {
	config.Interval
	Instances []*Instance `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Zookeeper{}
	})
}

func (z *Zookeeper) Prefix() string {
	return ""
}

func (z *Zookeeper) Init() error {
	if len(z.Instances) == 0 {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (z *Zookeeper) Drop() {}

func (z *Zookeeper) Gather(slist *list.SafeList) {
	atomic.AddUint64(&z.Counter, 1)
	for i := range z.Instances {
		ins := z.Instances[i]
		z.wg.Add(1)
		go z.gatherOnce(slist, ins)
	}
	z.wg.Wait()
}

func (z *Zookeeper) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer z.wg.Done()

	// metrics labels
	tags := map[string]string{"address": ins.Address, "zk_host": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("zk_scrape_use_seconds", use, tags))
	}(begun)

	// zk_up
	conn, err := ins.ZkConnect()
	if err != nil {
		slist.PushFront(inputs.NewSample("zk_up", 0, tags))
		log.Println("E! failed connect to zookeeper:"+ins.Address, "err:", err)
		return
	}
	defer conn.Close()

	z.gatherMntrResult(conn, slist, ins, tags)
	z.gatherRuokResult(conn, slist, ins, tags)

}

func (z *Zookeeper) gatherMntrResult(conn net.Conn, slist *list.SafeList, ins *Instance, globalTags map[string]string) {
	res := sendZookeeperCmd(conn, ins.Address, "mntr")

	// get slice of strings from response, like 'zk_avg_latency 0'
	lines := strings.Split(res, "\n")

	// 'mntr' command isn't allowed in zk config, log as warning
	if strings.Contains(lines[0], cmdNotExecutedSffx) {
		slist.PushFront(inputs.NewSample("zk_up", 0, globalTags))
		log.Printf(commandNotAllowedTmpl, "mntr", ins.Address)
		return
	}

	slist.PushFront(inputs.NewSample("zk_up", 1, globalTags))

	// skip instance if it in a leader only state and doesnt serving client requests
	if lines[0] == instanceNotServingMessage {
		slist.PushFront(inputs.NewSample("zk_server_leader", 1, globalTags))
		return
	}

	// split each line into key-value pair
	for _, l := range lines {
		if l == "" {
			continue
		}

		kv := strings.Split(strings.Replace(l, "\t", " ", -1), " ")
		key := kv[0]
		value := kv[1]

		switch key {
		case "zk_server_state":
			if value == "leader" {
				slist.PushFront(inputs.NewSample("zk_server_leader", 1, globalTags))
			} else {
				slist.PushFront(inputs.NewSample("zk_server_leader", 0, globalTags))
			}

		case "zk_version":
			version := versionRE.ReplaceAllString(value, "$1")
			slist.PushFront(inputs.NewSample("zk_version", 1, globalTags, map[string]string{"version": version}))

		case "zk_peer_state":
			slist.PushFront(inputs.NewSample("zk_peer_state", 1, globalTags, map[string]string{"state": value}))

		default:
			var k string
			k = metricNameReplacer.Replace(key)
			if !isDigit(value) {
				log.Printf("warning: skipping metric %q which holds not-digit value: %q", key, value)
				continue
			}
			slist.PushFront(inputs.NewSample(k, value, globalTags))
		}
	}
}

func (z *Zookeeper) gatherRuokResult(conn net.Conn, slist *list.SafeList, ins *Instance, globalTags map[string]string) {
	res := sendZookeeperCmd(conn, ins.Address, "ruok")
	if res == "imok" {
		slist.PushFront(inputs.NewSample("zk_ruok", 1, globalTags))
	} else {
		if strings.Contains(res, cmdNotExecutedSffx) {
			log.Printf(commandNotAllowedTmpl, "ruok", ins.Address)
		}
		slist.PushFront(inputs.NewSample("zk_ruok", 0, globalTags))
	}
}

func sendZookeeperCmd(conn net.Conn, host, cmd string) string {
	_, err := conn.Write([]byte(cmd))
	if err != nil {
		log.Println("E! failed to exec Zookeeper command:", cmd)
	}

	res, err := ioutil.ReadAll(conn)
	if err != nil {
		log.Printf("E! failed read Zookeeper command: '%s' response from '%s': %s", cmd, host, err)
	}
	return string(res)
}

func isDigit(in string) bool {
	// check input is an int
	if _, err := strconv.Atoi(in); err != nil {
		// not int, try float
		if _, err := strconv.ParseFloat(in, 64); err != nil {
			return false
		}
	}
	return true
}
