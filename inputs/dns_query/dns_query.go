package dns_query

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/miekg/dns"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "dns_query"

type ResultType uint64

const (
	Success ResultType = iota
	Timeout
	Error
)

type DnsQuery struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &DnsQuery{}
	})
}

func (r *DnsQuery) Prefix() string {
	return inputName
}

func (r *DnsQuery) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (r *DnsQuery) Drop() {}

func (r *DnsQuery) Gather(slist *list.SafeList) {
	atomic.AddUint64(&r.counter, 1)

	for i := range r.Instances {
		ins := r.Instances[i]

		if len(ins.Servers) == 0 {
			continue
		}

		r.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer r.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&r.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	r.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	// Domains or subdomains to query
	Domains []string `toml:"domains"`

	// Network protocol name
	Network string `toml:"network"`

	// Server to query
	Servers []string `toml:"servers"`

	// Record type
	RecordType string `toml:"record_type"`

	// DNS server port number
	Port int `toml:"port"`

	// Dns query timeout in seconds. 0 means no timeout
	Timeout int `toml:"timeout"`
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 {
		resolvPath := "/etc/resolv.conf"
		if _, err := os.Stat(resolvPath); os.IsNotExist(err) {
			return nil
		}
		config, _ := dns.ClientConfigFromFile(resolvPath)
		Servers := []string{}
		for _, ipAddress := range config.Servers {
			Servers = append(Servers, ipAddress)
		}
		ins.Servers = Servers
		if len(ins.Servers) == 0 {
			return nil
		}
	}

	if ins.Network == "" {
		ins.Network = "udp"
	}

	if len(ins.RecordType) == 0 {
		ins.RecordType = "NS"
	}

	if len(ins.Domains) == 0 {
		ins.Domains = []string{"."}
		ins.RecordType = "NS"
	}

	if ins.Port == 0 {
		ins.Port = 53
	}

	if ins.Timeout == 0 {
		ins.Timeout = 2
	}

	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {
	var wg sync.WaitGroup

	for _, domain := range ins.Domains {
		for _, server := range ins.Servers {
			wg.Add(1)
			go func(domain, server string) {
				fields := make(map[string]interface{}, 2)
				tags := map[string]string{
					"server":      server,
					"domain":      domain,
					"record_type": ins.RecordType,
				}

				dnsQueryTime, rcode, err := ins.getDNSQueryTime(domain, server)
				if rcode >= 0 {
					fields["rcode_value"] = rcode
				}

				if err == nil {
					setResult(Success, fields)
					fields["query_time_ms"] = dnsQueryTime
				} else if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					setResult(Timeout, fields)
				} else if err != nil {
					setResult(Error, fields)
					log.Println("E!", err)
				}

				inputs.PushSamples(slist, fields, tags, ins.Labels)
				wg.Done()
			}(domain, server)
		}
	}

	wg.Wait()
}

func (ins *Instance) getDNSQueryTime(domain string, server string) (float64, int, error) {
	dnsQueryTime := float64(0)

	c := new(dns.Client)
	c.ReadTimeout = time.Duration(ins.Timeout) * time.Second
	c.Net = ins.Network

	m := new(dns.Msg)
	recordType, err := ins.parseRecordType()
	if err != nil {
		return dnsQueryTime, -1, err
	}
	m.SetQuestion(dns.Fqdn(domain), recordType)
	m.RecursionDesired = true

	r, rtt, err := c.Exchange(m, net.JoinHostPort(server, strconv.Itoa(ins.Port)))
	if err != nil {
		return dnsQueryTime, -1, err
	}
	if r.Rcode != dns.RcodeSuccess {
		return dnsQueryTime, r.Rcode, fmt.Errorf("invalid answer (%s) from %s after %s query for %s", dns.RcodeToString[r.Rcode], server, ins.RecordType, domain)
	}
	dnsQueryTime = float64(rtt.Nanoseconds()) / 1e6
	return dnsQueryTime, r.Rcode, nil
}

func (ins *Instance) parseRecordType() (uint16, error) {
	var recordType uint16
	var err error

	switch ins.RecordType {
	case "A":
		recordType = dns.TypeA
	case "AAAA":
		recordType = dns.TypeAAAA
	case "ANY":
		recordType = dns.TypeANY
	case "CNAME":
		recordType = dns.TypeCNAME
	case "MX":
		recordType = dns.TypeMX
	case "NS":
		recordType = dns.TypeNS
	case "PTR":
		recordType = dns.TypePTR
	case "SOA":
		recordType = dns.TypeSOA
	case "SPF":
		recordType = dns.TypeSPF
	case "SRV":
		recordType = dns.TypeSRV
	case "TXT":
		recordType = dns.TypeTXT
	default:
		err = fmt.Errorf("record type %s not recognized", ins.RecordType)
	}

	return recordType, err
}

func setResult(result ResultType, fields map[string]interface{}) {
	fields["result_code"] = uint64(result)
}
