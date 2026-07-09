package dns_query

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/miekg/dns"
)

const inputName = "dns_query"

type ResultType uint64

const (
	Success ResultType = iota
	Timeout
	Error
)

type DnsQuery struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &DnsQuery{}
	})
}

func (dq *DnsQuery) Clone() inputs.Input {
	return &DnsQuery{}
}

func (c *DnsQuery) Name() string {
	return inputName
}

func (dq *DnsQuery) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(dq.Instances))
	for i := 0; i < len(dq.Instances); i++ {
		ret[i] = dq.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	EnableAutoDetectDnsServer bool `toml:"auto_detect_local_dns_server"`
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

	ExpectQueryIps map[string][]string `toml:"expect_query_ips"`
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 && ins.EnableAutoDetectDnsServer {
		resolvPath := "/etc/resolv.conf"
		if _, err := os.Stat(resolvPath); os.IsNotExist(err) {
			return types.ErrInstancesEmpty
		}

		config, err := dns.ClientConfigFromFile(resolvPath)
		if err != nil {
			log.Println("E! failed to detect local dns server:", err)
			return types.ErrInstancesEmpty
		}

		ins.Servers = config.Servers
	}

	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
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

// set(B) - set(A)
func diff(ips1, ips2 []string) (diff []string, match []string) {
	ips1Set := make(map[string]bool)
	for _, ip := range ips1 {
		ips1Set[ip] = true
	}

	for _, ip := range ips2 {
		if !ips1Set[ip] {
			diff = append(diff, ip)
		} else {
			match = append(match, ip)
		}
	}

	return diff, match
}

func (ins *Instance) Gather(slist *types.SampleList) {
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

				dnsQueryTime, rcode, ips, err := ins.getDNSQueryTime(domain, server)
				if rcode >= 0 {
					fields["rcode_value"] = rcode
				}

				if v, ok := ins.ExpectQueryIps[domain]; ok && v != nil {
					sort.Slice(ips, func(i, j int) bool {
						ip1 := net.ParseIP(ips[i])
						ip2 := net.ParseIP(ips[j])
						return bytes.Compare(ip1, ip2) < 0
					})
					minus, _ := diff(v, ips)
					status := 0
					if len(minus) > 0 {
						status = 1

						diffTags := make(map[string]string)
						for k, v := range tags {
							diffTags[k] = v
						}
						diffTags["diff"] = strings.Join(minus, ",")
						diffTags["ips"] = strings.Join(ips, ",")
						slist.PushSample(inputName, "status_change_detail", status, diffTags)
					}
					slist.PushSample(inputName, "status_change", status, tags)
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
				if len(ips) > 0 {
					tags["ips"] = strings.Join(ips, ",")
				}

				slist.PushSamples("dns_query", fields, tags)
				wg.Done()
			}(domain, server)
		}
	}

	wg.Wait()
}

func (ins *Instance) getDNSQueryTime(domain string, server string) (float64, int, []string, error) {
	dnsQueryTime := float64(0)

	c := new(dns.Client)
	c.ReadTimeout = time.Duration(ins.Timeout) * time.Second
	c.Net = ins.Network

	m := new(dns.Msg)
	recordType, err := ins.parseRecordType()
	if err != nil {
		return dnsQueryTime, -1, nil, err
	}
	m.SetQuestion(dns.Fqdn(domain), recordType)
	m.RecursionDesired = true

	r, rtt, err := c.Exchange(m, net.JoinHostPort(server, strconv.Itoa(ins.Port)))
	if err != nil {
		return dnsQueryTime, -1, nil, err
	}
	if r.Rcode != dns.RcodeSuccess {
		return dnsQueryTime, r.Rcode, nil, fmt.Errorf("invalid answer (%s) from %s after %s query for %s", dns.RcodeToString[r.Rcode], server, ins.RecordType, domain)
	}
	dnsQueryTime = float64(rtt.Nanoseconds()) / 1e6
	ips := make([]string, 0)
	for _, record := range r.Answer {
		if ip, found := extractIP(record); found {
			ips = append(ips, ip)
		}
	}
	return dnsQueryTime, r.Rcode, ips, nil
}

func extractIP(record dns.RR) (string, bool) {
	if r, ok := record.(*dns.A); ok {
		return r.A.String(), true
	}
	if r, ok := record.(*dns.AAAA); ok {
		return r.AAAA.String(), true
	}
	return "", false
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