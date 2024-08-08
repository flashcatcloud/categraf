package zookeeper

import (
	crypto_tls "crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const (
	inputName                 = "zookeeper"
	commandNotAllowedTmpl     = "E!: %q command isn't allowed at %q, see '4lw.commands.whitelist' ZK config parameter\n"
	instanceNotServingMessage = "This ZooKeeper instance is not currently serving requests"
	cmdNotExecutedSffx        = "is not executed because it is not in the whitelist."
)

var (
	versionRE          = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+).*$`)
	metricNameReplacer = strings.NewReplacer("-", "_", ".", "_")
)

type Instance struct {
	config.InstanceConfig

	Addresses   string `toml:"addresses"`
	Timeout     int    `toml:"timeout"`
	ClusterName string `toml:"cluster_name"`
	tls.ClientConfig
}

func (ins *Instance) ZkHosts() []string {
	return strings.Fields(ins.Addresses)
}

func (ins *Instance) ZkConnect(host string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: time.Duration(ins.Timeout) * time.Second}
	tcpaddr, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve zookeeper(cluster: %s) address: %s: %v", ins.ClusterName, host, err)
	}

	if !ins.UseTLS {
		return dialer.Dial("tcp", tcpaddr.String())
	}
	tlsConfig, err := ins.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to init tls config: %v", err)
	}
	return crypto_tls.DialWithDialer(&dialer, "tcp", tcpaddr.String(), tlsConfig)
}

type Zookeeper struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Zookeeper{}
	})
}

func (z *Zookeeper) Clone() inputs.Input {
	return &Zookeeper{}
}

func (z *Zookeeper) Name() string {
	return inputName
}

func (z *Zookeeper) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(z.Instances))
	for i := 0; i < len(z.Instances); i++ {
		ret[i] = z.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if len(ins.ZkHosts()) == 0 {
		return types.ErrInstancesEmpty
	}
	// set default timeout
	if ins.Timeout == 0 {
		ins.Timeout = 10
	}
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	hosts := ins.ZkHosts()
	if len(hosts) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < len(hosts); i++ {
		wg.Add(1)
		go ins.gatherOneHost(wg, slist, hosts[i])
	}
	wg.Wait()
}

func (ins *Instance) gatherOneHost(wg *sync.WaitGroup, slist *types.SampleList, zkHost string) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Println("E! Recovered in zookeeper gatherOneHost ", zkHost, r)
		}
	}()

	tags := map[string]string{"zk_host": zkHost, "zk_cluster": ins.ClusterName}
	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample("", "zk_scrape_use_seconds", use, tags))
	}(begun)

	// zk_up
	mntrConn, err := ins.ZkConnect(zkHost)
	if err != nil {
		slist.PushFront(types.NewSample("", "zk_up", 0, tags))
		log.Println("E! failed to connect zookeeper:", zkHost, "error:", err)
		return
	}

	defer mntrConn.Close()
	// prevent blocking
	mntrConn.SetDeadline(time.Now().Add(time.Duration(ins.Timeout) * time.Second))

	ins.gatherMntrResult(mntrConn, slist, tags)

	// zk_ruok
	ruokConn, err := ins.ZkConnect(zkHost)
	if err != nil {
		slist.PushFront(types.NewSample("", "zk_ruok", 0, tags))
		log.Println("E! failed to connect zookeeper:", zkHost, "error:", err)
		return
	}

	defer ruokConn.Close()
	// prevent blocking
	ruokConn.SetDeadline(time.Now().Add(time.Duration(ins.Timeout) * time.Second))

	ins.gatherRuokResult(ruokConn, slist, tags)

}

func (ins *Instance) gatherMntrResult(conn net.Conn, slist *types.SampleList, globalTags map[string]string) {
	res := sendZookeeperCmd(conn, "mntr")

	// get slice of strings from response, like 'zk_avg_latency 0'
	lines := strings.Split(res, "\n")

	// 'mntr' command isn't allowed in zk config, log as warning
	if strings.Contains(lines[0], cmdNotExecutedSffx) {
		slist.PushFront(types.NewSample("", "zk_up", 0, globalTags))
		log.Printf(commandNotAllowedTmpl, "mntr", conn.RemoteAddr().String())
		return
	}

	slist.PushFront(types.NewSample("", "zk_up", 1, globalTags))

	// skip instance if it in a leader only state and doesnt serving client requests
	if lines[0] == instanceNotServingMessage {
		slist.PushFront(types.NewSample("", "zk_server_leader", 1, globalTags))
		return
	}

	// split each line into key-value pair
	for _, l := range lines {
		if l == "" {
			continue
		}

		kv := strings.Fields(l)
		if len(kv) < 2 {
			continue
		}

		key := kv[0]
		value := kv[1]

		switch key {
		case "zk_server_state":
			if value == "leader" {
				slist.PushFront(types.NewSample("", "zk_server_leader", 1, globalTags))
			} else {
				slist.PushFront(types.NewSample("", "zk_server_leader", 0, globalTags))
			}

		case "zk_version":
			version := versionRE.ReplaceAllString(value, "$1")
			slist.PushFront(types.NewSample("", "zk_version", 1, globalTags, map[string]string{"version": version}))

		case "zk_peer_state":
			slist.PushFront(types.NewSample("", "zk_peer_state", 1, globalTags, map[string]string{"state": value}))

		default:
			var k string

			if !isDigit(value) {
				log.Printf("warning: skipping metric %q which holds not-digit value: %q", key, value)
				continue
			}
			k = metricNameReplacer.Replace(key)
			if strings.Contains(k, "{") {
				labels := parseLabels(k)
				slist.PushFront(types.NewSample("", k, value, globalTags, labels))
			} else {
				slist.PushFront(types.NewSample("", k, value, globalTags))
			}
		}
	}
}

func (ins *Instance) gatherRuokResult(conn net.Conn, slist *types.SampleList, globalTags map[string]string) {
	res := sendZookeeperCmd(conn, "ruok")
	if res == "imok" {
		slist.PushFront(types.NewSample("", "zk_ruok", 1, globalTags))
	} else {
		if strings.Contains(res, cmdNotExecutedSffx) {
			log.Printf(commandNotAllowedTmpl, "ruok", conn.RemoteAddr().String())
		}
		slist.PushFront(types.NewSample("", "zk_ruok", 0, globalTags))
	}
}

func sendZookeeperCmd(conn net.Conn, cmd string) string {
	_, err := conn.Write([]byte(cmd))
	if err != nil {
		log.Printf("E! failed to exec Zookeeper command: %s response from '%s': %s", cmd, conn.RemoteAddr().String(), err)
		return ""
	}

	res, err := io.ReadAll(conn)
	if err != nil {
		log.Printf("E! failed read Zookeeper command: '%s' response from '%s': %s", cmd, conn.RemoteAddr().String(), err)
		return ""
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

func parseLabels(in string) map[string]string {
	labels := map[string]string{}

	labelsRE := regexp.MustCompile(`{(.*)}`)
	labelRE := regexp.MustCompile(`(.*)\=(\".*\")`)
	matchLables := labelsRE.FindStringSubmatch(in)
	if len(matchLables) > 1 {
		labelsStr := matchLables[1]
		for _, labelStr := range strings.Split(labelsStr, ",") {
			m := labelRE.FindStringSubmatch(labelStr)
			if len(m) == 3 {
				key := m[1]
				value := m[2]
				labels[key] = value
			}
		}
	}
	return labels
}
