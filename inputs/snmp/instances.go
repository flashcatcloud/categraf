package snmp

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/freedomkk-qfeng/go-fastping"
	"github.com/gosnmp/gosnmp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

type Instance struct {
	config.InstanceConfig
	// The SNMP agent to query. Format is [SCHEME://]ADDR[:PORT] (e.g.
	// udp://1.2.3.4:161).  If the scheme is not specified then "udp" is used.
	Agents []string `toml:"agents"`

	// The tag used to name the agent host
	AgentHostTag string `toml:"agent_host_tag"`

	ClientConfig

	Tables []Table `toml:"table"`

	// Name & Fields are the elements of a Table.
	// Categraf chokes if we try to embed a Table. So instead we have to embed the
	// fields of a Table, and construct a Table during runtime.
	Name   string  `toml:"name"`
	Fields []Field `toml:"field"`

	connectionCache []snmpConnection

	Translator string `toml:"translator"`

	translator Translator

	Mappings map[string]map[string]string `toml:"mappings"`
}

func (ins *Instance) Init() error {

	if len(ins.Agents) == 0 {
		return types.ErrInstancesEmpty
	}

	var err error
	switch ins.Translator {
	case "gosmi":
		ins.translator, err = NewGosmiTranslator(ins.Path)
		if err != nil {
			return err
		}
		ins.translator.SetDebugMode(ins.DebugMod)
	case "", "netsnmp":
		ins.translator = NewNetsnmpTranslator()
		ins.translator.SetDebugMode(ins.DebugMod)
	default:
		return fmt.Errorf("invalid translator value")
	}

	ins.connectionCache = make([]snmpConnection, len(ins.Agents))

	for i := range ins.Tables {
		if err := ins.Tables[i].Init(ins.translator); err != nil {
			return fmt.Errorf("initializing table %s ins: %s", ins.Tables[i].Name, err)
		}
	}

	for i := range ins.Fields {
		if err := ins.Fields[i].init(ins.translator); err != nil {
			return fmt.Errorf("initializing field %s ins: %w", ins.Fields[i].Name, err)
		}
	}

	if len(ins.AgentHostTag) == 0 {
		ins.AgentHostTag = "agent_host"
	}

	return nil
}

func (ins *Instance) up(slist *types.SampleList, i int) {
	host := ins.Agents[i]
	u, err := url.Parse(ins.Agents[i])
	if err == nil {
		host = u.Hostname()
	}

	etags := map[string]string{}
	for k, v := range ins.GetLabels() {
		etags[k] = v
	}
	if m, ok := ins.Mappings[host]; ok {
		for k, v := range m {
			etags[k] = v
		}
	}
	etags[ins.AgentHostTag] = host

	// icmp probe
	up := Ping(host, 300)
	slist.PushSample(inputName, "icmp_up", up, etags)

	// snmp probe
	oid := ".1.3.6.1.2.1.1.1.0"
	gs, err := ins.getConnection(i)
	if err != nil {
		slist.PushSample(inputName, "up", 0, etags)
		return
	}
	_, err = gs.Get([]string{oid})
	if err != nil {
		if strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "reset") {
			slist.PushSample(inputName, "up", 0, etags)
			return
		}
	}
	slist.PushSample(inputName, "up", 1, etags)
}

// Gather retrieves all the configured fields and tables.
// Any error encountered does not halt the process. The errors are accumulated
// and returned at the end.
func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup
	for i, agent := range ins.Agents {
		wg.Add(1)
		go func(i int, agent string) {
			defer wg.Done()
			// First is the top-level fields. We treat the fields as table prefixes with an empty index.
			t := Table{
				Name:   ins.Name,
				Fields: ins.Fields,

				DebugMode: ins.DebugMod,
			}
			for idx, f := range t.Fields {
				t.Fields[idx].Oid = strings.TrimSpace(f.Oid)
			}
			topTags := map[string]string{}
			for k, v := range ins.GetLabels() {
				topTags[k] = v
			}
			extraTags := map[string]string{}
			if m, ok := ins.Mappings[agent]; ok {
				extraTags = m
			}
			ins.up(slist, i)

			gs, err := ins.getConnection(i)
			if err != nil {
				log.Printf("agent %s ins: %s", agent, err)
				return
			}
			if err := ins.gatherTable(slist, gs, t, topTags, extraTags, false); err != nil {
				log.Printf("agent %s ins: %s", agent, err)
			}

			// Now is the real tables.
			for _, t := range ins.Tables {
				if err := ins.gatherTable(slist, gs, t, topTags, extraTags, true); err != nil {
					log.Printf("agent %s ins: gathering table %s error: %s", agent, t.Name, err)
				}
			}
		}(i, agent)
	}
	wg.Wait()
}

func (ins *Instance) gatherTable(slist *types.SampleList, gs snmpConnection, t Table, topTags, extraTags map[string]string, walk bool) error {
	rt, err := t.Build(gs, walk, ins.translator)
	if err != nil {
		return err
	}

	prefix := inputName
	if len(rt.Name) != 0 {
		prefix = inputName + "_" + rt.Name
	}
	for _, tr := range rt.Rows {
		if !walk {
			// top-level table. Add tags to topTags.
			for k, v := range tr.Tags {
				topTags[k] = v
			}
		} else {
			// real table. Inherit any specified tags.
			for _, k := range t.InheritTags {
				if v, ok := topTags[k]; ok {
					tr.Tags[k] = v
				}
			}
		}
		if _, ok := tr.Tags[ins.AgentHostTag]; !ok {
			tr.Tags[ins.AgentHostTag] = gs.Host()
		}
		for k, v := range extraTags {
			tr.Tags[k] = v
		}
		slist.PushSamples(prefix, tr.Fields, tr.Tags)
	}

	return nil
}

// snmpConnection is an interface which wraps a *gosnmp.GoSNMP object.
// We interact through an interface, so we can mock it out in tests.
type snmpConnection interface {
	Host() string

	// BulkWalkAll(string) ([]gosnmp.SnmpPDU, error)

	Walk(string, gosnmp.WalkFunc) error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
}

// getConnection creates a snmpConnection (*gosnmp.GoSNMP) object and caches the
// result using `agentIndex` as the cache key.  This is done to allow multiple
// connections to a single address.  It is an error to use a connection in
// more than one goroutine.
func (ins *Instance) getConnection(idx int) (snmpConnection, error) {
	if gs := ins.connectionCache[idx]; gs != nil {
		return gs, nil
	}

	agent := ins.Agents[idx]

	var err error
	var gs GosnmpWrapper
	gs, err = NewWrapper(ins.ClientConfig)
	if err != nil {
		return nil, err
	}

	err = gs.SetAgent(agent)
	if err != nil {
		return nil, err
	}

	ins.connectionCache[idx] = gs

	if err := gs.Connect(); err != nil {
		return nil, fmt.Errorf("setting up connection: %w", err)
	}

	return gs, nil
}

func Ping(ip string, timeout int) float64 {
	rtt, err := fastPingRtt(ip, timeout)
	if err != nil {
		log.Printf("W! snmp ping %s error:%s", ip, err)
	}
	if rtt == -1 {
		return 0
	}
	return 1
}

func fastPingRtt(ip string, timeout int) (float64, error) {
	var rt float64
	rt = -1
	p := fastping.NewPinger()
	ra, err := net.ResolveIPAddr("ip4:icmp", ip)
	if err != nil {
		return -1, err
	}
	p.AddIPAddr(ra)
	p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		rt = float64(rtt.Nanoseconds()) / 1000000.0
	}
	p.OnIdle = func() {
	}
	p.MaxRTT = time.Millisecond * time.Duration(timeout)
	err = p.Run()
	if err != nil {
		return -1, err
	}

	return rt, err
}
