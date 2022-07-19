package haproxy

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "haproxy"

type Haproxy struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Haproxy{}
	})
}

func (r *Haproxy) Prefix() string {
	return inputName
}

func (r *Haproxy) Init() error {
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

func (r *Haproxy) Drop() {}

func (r *Haproxy) Gather(slist *list.SafeList) {
	atomic.AddUint64(&r.counter, 1)

	for i := range r.Instances {
		ins := r.Instances[i]

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
	Labels                map[string]string `toml:"labels"`
	IntervalTimes         int64             `toml:"interval_times"`
	Servers               []string          `toml:"servers"`
	Headers               []string          `toml:"headers"`
	Username              string            `toml:"username"`
	Password              string            `toml:"password"`
	IgnoreMetrics         []string          `toml:"ignore_metrics"`
	IgnoreLabelKeys       []string          `toml:"ignore_label_keys"`
	NamePrefix            string            `toml:"name_prefix"`
	Timeout               config.Duration   `toml:"timeout"`
	KeepFieldNames        bool              `toml:"keep_fieldnames"`
	ignoreMetricsFilter   filter.Filter
	ignoreLabelKeysFilter filter.Filter

	tls.ClientConfig
	client *http.Client
}

func (ins *Instance) Init() error {

	if ins.Timeout <= 0 {
		ins.Timeout = config.Duration(time.Second * 5)
	}
	if ins.client == nil {
		tlsCfg, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			log.Println("can't init TLSConfig :%s", err)
			return err
		}
		tr := &http.Transport{
			ResponseHeaderTimeout: time.Duration(ins.Timeout),
			TLSClientConfig:       tlsCfg,
		}
		client := &http.Client{
			Transport: tr,
			Timeout:   time.Duration(ins.Timeout),
		}
		ins.client = client
	}

	if len(ins.IgnoreMetrics) > 0 {
		ignoreMetricsFilter, err := filter.Compile(ins.IgnoreMetrics)
		if err != nil {
			return err
		}
		ins.ignoreMetricsFilter = ignoreMetricsFilter
	}

	if len(ins.IgnoreLabelKeys) > 0 {
		ignoreLabelKeysFilter, err := filter.Compile(ins.IgnoreLabelKeys)
		if err != nil {
			return err
		}
		ins.ignoreLabelKeysFilter = ignoreLabelKeysFilter
	}
	return nil
}

func (ins *Instance) gatherOnce(slist *list.SafeList) {
	if len(ins.Servers) == 0 {
		ins.gatherServer("http://127.0.0.1:1936/haproxy?stats", slist)
		return
	}

	endpoints := make([]string, 0, len(ins.Servers))

	for _, endpoint := range ins.Servers {
		if strings.HasPrefix(endpoint, "http") {
			endpoints = append(endpoints, endpoint)
			continue
		}

		socketPath := getSocketAddr(endpoint)

		matches, err := filepath.Glob(socketPath)

		if err != nil {
			log.Println("can't glob socket path %s", err)
			return
		}

		if len(matches) == 0 {
			endpoints = append(endpoints, socketPath)
		} else {
			endpoints = append(endpoints, matches...)
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(endpoints))
	for _, server := range endpoints {
		go func(serv string) {
			defer wg.Done()
			if err := ins.gatherServer(serv, slist); err != nil {
				//acc.AddError(err)
			}
		}(server)
	}

	wg.Wait()
}

func (ins *Instance) gatherServerSocket(addr string, slist *list.SafeList) error {
	socketPath := getSocketAddr(addr)

	c, err := net.Dial("unix", socketPath)

	if err != nil {
		return fmt.Errorf("could not connect to socket '%s': %s", addr, err)
	}

	_, errw := c.Write([]byte("show stat\n"))

	if errw != nil {
		return fmt.Errorf("could not write to socket '%s': %s", addr, errw)
	}

	return ins.importCsvResult(c, slist, socketPath)
}

func (ins *Instance) gatherServer(addr string, slist *list.SafeList) error {
	if !strings.HasPrefix(addr, "http") {
		return ins.gatherServerSocket(addr, slist)
	}

	if !strings.HasSuffix(addr, "metrics") {
		addr += "/metrics"
	}

	u, err := url.Parse(addr)
	if err != nil {
		log.Println("unable parse server address '%s': %s", addr, err)
		return err
	}
	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		log.Println("unable to create new request '%s': %s", addr, err)
		return err
	}
	if u.User != nil {
		p, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), p)
		u.User = &url.Userinfo{}
		addr = u.String()
	}

	if ins.Username != "" || ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	res, err := ins.client.Do(req)
	if err != nil {
		fmt.Errorf("unable to connect to haproxy server '%s': %s", addr, err)
		return err
	}
	defer res.Body.Close()

	labels := map[string]string{}

	for key, val := range ins.Labels {
		labels[key] = val
	}

	if res.StatusCode != 200 {
		slist.PushFront(types.NewSample("up", 0, labels))
		fmt.Println("unable to get valid stat result from '%s', http response code : %d", addr, res.StatusCode)
		return fmt.Errorf("unable to get valid stat result from '%s', http response code : %d", addr, res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		slist.PushFront(types.NewSample("up", 0, labels))
		log.Println("E! failed to read response body, error:", err)
		return err
	}

	slist.PushFront(types.NewSample("up", 1, labels))
	//ins.NamePrefix
	parser := prometheus.NewParser("", labels, res.Header, ins.ignoreMetricsFilter, ins.ignoreLabelKeysFilter)
	if err = parser.Parse(body, slist); err != nil {
		log.Println("E! failed to parse response body, url:", u.String(), "error:", err)
		return err
	}
	return nil
}

func getSocketAddr(sock string) string {
	socketAddr := strings.Split(sock, ":")

	if len(socketAddr) >= 2 {
		return socketAddr[1]
	}
	return socketAddr[0]
}

var typeNames = []string{"frontend", "backend", "server", "listener"}
var fieldRenames = map[string]string{
	"pxname":     "proxy",
	"svname":     "sv",
	"act":        "active_servers",
	"bck":        "backup_servers",
	"cli_abrt":   "cli_abort",
	"srv_abrt":   "srv_abort",
	"hrsp_1xx":   "http_response.1xx",
	"hrsp_2xx":   "http_response.2xx",
	"hrsp_3xx":   "http_response.3xx",
	"hrsp_4xx":   "http_response.4xx",
	"hrsp_5xx":   "http_response.5xx",
	"hrsp_other": "http_response.other",
}

//CSV format: https://cbonte.github.io/haproxy-dconv/1.5/configuration.html#9.1
func (ins *Instance) importCsvResult(r io.Reader, slist *list.SafeList, host string) error {
	csvr := csv.NewReader(r)
	//now := time.Now()

	headers, err := csvr.Read()
	if err != nil {
		return err
	}
	if len(headers[0]) <= 2 || headers[0][:2] != "# " {
		return fmt.Errorf("did not receive standard haproxy headers")
	}
	headers[0] = headers[0][2:]

	for {
		row, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		fields := make(map[string]interface{})
		tags := map[string]string{
			"server": host,
		}

		if len(row) != len(headers) {
			return fmt.Errorf("number of columns does not match number of headers. headers=%d columns=%d", len(headers), len(row))
		}
		for i, v := range row {
			if v == "" {
				continue
			}

			colName := headers[i]
			fieldName := colName
			if !ins.KeepFieldNames {
				if fieldRename, ok := fieldRenames[colName]; ok {
					fieldName = fieldRename
				}
			}

			switch colName {
			case "pxname", "svname":
				tags[fieldName] = v
			case "type":
				vi, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return fmt.Errorf("unable to parse type value '%s'", v)
				}
				if vi >= int64(len(typeNames)) {
					return fmt.Errorf("received unknown type value: %d", vi)
				}
				tags[fieldName] = typeNames[vi]
			case "check_desc", "agent_desc":
				// do nothing. These fields are just a more verbose description of the check_status & agent_status fields
			case "status", "check_status", "last_chk", "mode", "tracked", "agent_status", "last_agt", "addr", "cookie":
				// these are string fields
				fields[fieldName] = v
			case "lastsess":
				vi, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					//TODO log the error. And just once (per column) so we don't spam the log
					continue
				}
				fields[fieldName] = vi
			default:
				vi, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					//TODO log the error. And just once (per column) so we don't spam the log
					continue
				}
				fields[fieldName] = vi
			}
		}
		//acc.AddFields("haproxy", fields, tags, now)
		for key, val := range fields {
			slist.PushFront(types.NewSample(key, val, tags, ins.Labels))
		}
	}
	return err
}
