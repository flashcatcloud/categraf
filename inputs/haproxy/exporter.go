// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package haproxy

import (
	"bufio"
	"crypto/tls"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "haproxy" // For Prometheus metrics.

	// HAProxy 1.4
	// # pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,
	// HAProxy 1.5
	// pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,
	// HAProxy 1.5.19
	// pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,
	// HAProxy 1.7
	// pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,agent_status,agent_code,agent_duration,check_desc,agent_desc,check_rise,check_fall,check_health,agent_rise,agent_fall,agent_health,addr,cookie,mode,algo,conn_rate,conn_rate_max,conn_tot,intercepted,dcon,dses
	minimumCsvFieldCount = 33

	pxnameField        = 0
	svnameField        = 1
	statusField        = 17
	typeField          = 32
	checkDurationField = 38
	qtimeMsField       = 58
	ctimeMsField       = 59
	rtimeMsField       = 60
	ttimeMsField       = 61

	excludedServerStates = ""
	showStatCmd          = "show stat\n"
	showInfoCmd          = "show info\n"
)

var (
	frontendLabelNames = []string{"frontend"}
	backendLabelNames  = []string{"backend"}
	serverLabelNames   = []string{"backend", "server"}
)

type metricInfo struct {
	Desc *prometheus.Desc
	Type prometheus.ValueType
}

func newFrontendMetric(metricName string, docString string, t prometheus.ValueType, constLabels prometheus.Labels) metricInfo {
	return metricInfo{
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "frontend", metricName),
			docString,
			frontendLabelNames,
			constLabels,
		),
		Type: t,
	}
}

func newBackendMetric(metricName string, docString string, t prometheus.ValueType, constLabels prometheus.Labels) metricInfo {
	return metricInfo{
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "backend", metricName),
			docString,
			backendLabelNames,
			constLabels,
		),
		Type: t,
	}
}

func newServerMetric(metricName string, docString string, t prometheus.ValueType, constLabels prometheus.Labels) metricInfo {
	return metricInfo{
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "server", metricName),
			docString,
			serverLabelNames,
			constLabels,
		),
		Type: t,
	}
}

type metrics map[int]metricInfo

func (m metrics) String() string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	s := make([]string, len(keys))
	for i, k := range keys {
		s[i] = strconv.Itoa(k)
	}
	return strings.Join(s, ",")
}

var (
	serverMetrics = metrics{
		2:  newServerMetric("current_queue", "Current number of queued requests assigned to this server.", prometheus.GaugeValue, nil),
		3:  newServerMetric("max_queue", "Maximum observed number of queued requests assigned to this server.", prometheus.GaugeValue, nil),
		4:  newServerMetric("current_sessions", "Current number of active sessions.", prometheus.GaugeValue, nil),
		5:  newServerMetric("max_sessions", "Maximum observed number of active sessions.", prometheus.GaugeValue, nil),
		6:  newServerMetric("limit_sessions", "Configured session limit.", prometheus.GaugeValue, nil),
		7:  newServerMetric("sessions_total", "Total number of sessions.", prometheus.CounterValue, nil),
		8:  newServerMetric("bytes_in_total", "Current total of incoming bytes.", prometheus.CounterValue, nil),
		9:  newServerMetric("bytes_out_total", "Current total of outgoing bytes.", prometheus.CounterValue, nil),
		13: newServerMetric("connection_errors_total", "Total of connection errors.", prometheus.CounterValue, nil),
		14: newServerMetric("response_errors_total", "Total of response errors.", prometheus.CounterValue, nil),
		15: newServerMetric("retry_warnings_total", "Total of retry warnings.", prometheus.CounterValue, nil),
		16: newServerMetric("redispatch_warnings_total", "Total of redispatch warnings.", prometheus.CounterValue, nil),
		17: newServerMetric("up", "Current health status of the server (1 = UP, 0 = DOWN).", prometheus.GaugeValue, nil),
		18: newServerMetric("weight", "Current weight of the server.", prometheus.GaugeValue, nil),
		21: newServerMetric("check_failures_total", "Total number of failed health checks.", prometheus.CounterValue, nil),
		24: newServerMetric("downtime_seconds_total", "Total downtime in seconds.", prometheus.CounterValue, nil),
		30: newServerMetric("server_selected_total", "Total number of times a server was selected, either for new sessions, or when re-dispatching.", prometheus.CounterValue, nil),
		33: newServerMetric("current_session_rate", "Current number of sessions per second over last elapsed second.", prometheus.GaugeValue, nil),
		35: newServerMetric("max_session_rate", "Maximum observed number of sessions per second.", prometheus.GaugeValue, nil),
		38: newServerMetric("check_duration_seconds", "Previously run health check duration, in seconds", prometheus.GaugeValue, nil),
		39: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "1xx"}),
		40: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "2xx"}),
		41: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "3xx"}),
		42: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "4xx"}),
		43: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "5xx"}),
		44: newServerMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "other"}),
		49: newServerMetric("client_aborts_total", "Total number of data transfers aborted by the client.", prometheus.CounterValue, nil),
		50: newServerMetric("server_aborts_total", "Total number of data transfers aborted by the server.", prometheus.CounterValue, nil),
		58: newServerMetric("http_queue_time_average_seconds", "Avg. HTTP queue time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		59: newServerMetric("http_connect_time_average_seconds", "Avg. HTTP connect time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		60: newServerMetric("http_response_time_average_seconds", "Avg. HTTP response time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		61: newServerMetric("http_total_time_average_seconds", "Avg. HTTP total time for last 1024 successful connections.", prometheus.GaugeValue, nil),
	}

	frontendMetrics = metrics{
		4:  newFrontendMetric("current_sessions", "Current number of active sessions.", prometheus.GaugeValue, nil),
		5:  newFrontendMetric("max_sessions", "Maximum observed number of active sessions.", prometheus.GaugeValue, nil),
		6:  newFrontendMetric("limit_sessions", "Configured session limit.", prometheus.GaugeValue, nil),
		7:  newFrontendMetric("sessions_total", "Total number of sessions.", prometheus.CounterValue, nil),
		8:  newFrontendMetric("bytes_in_total", "Current total of incoming bytes.", prometheus.CounterValue, nil),
		9:  newFrontendMetric("bytes_out_total", "Current total of outgoing bytes.", prometheus.CounterValue, nil),
		10: newFrontendMetric("requests_denied_total", "Total of requests denied for security.", prometheus.CounterValue, nil),
		12: newFrontendMetric("request_errors_total", "Total of request errors.", prometheus.CounterValue, nil),
		33: newFrontendMetric("current_session_rate", "Current number of sessions per second over last elapsed second.", prometheus.GaugeValue, nil),
		34: newFrontendMetric("limit_session_rate", "Configured limit on new sessions per second.", prometheus.GaugeValue, nil),
		35: newFrontendMetric("max_session_rate", "Maximum observed number of sessions per second.", prometheus.GaugeValue, nil),
		39: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "1xx"}),
		40: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "2xx"}),
		41: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "3xx"}),
		42: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "4xx"}),
		43: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "5xx"}),
		44: newFrontendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "other"}),
		48: newFrontendMetric("http_requests_total", "Total HTTP requests.", prometheus.CounterValue, nil),
		51: newFrontendMetric("compressor_bytes_in_total", "Number of HTTP response bytes fed to the compressor", prometheus.CounterValue, nil),
		52: newFrontendMetric("compressor_bytes_out_total", "Number of HTTP response bytes emitted by the compressor", prometheus.CounterValue, nil),
		53: newFrontendMetric("compressor_bytes_bypassed_total", "Number of bytes that bypassed the HTTP compressor", prometheus.CounterValue, nil),
		54: newFrontendMetric("http_responses_compressed_total", "Number of HTTP responses that were compressed", prometheus.CounterValue, nil),
		79: newFrontendMetric("connections_total", "Total number of connections", prometheus.CounterValue, nil),
	}
	backendMetrics = metrics{
		2:  newBackendMetric("current_queue", "Current number of queued requests not assigned to any server.", prometheus.GaugeValue, nil),
		3:  newBackendMetric("max_queue", "Maximum observed number of queued requests not assigned to any server.", prometheus.GaugeValue, nil),
		4:  newBackendMetric("current_sessions", "Current number of active sessions.", prometheus.GaugeValue, nil),
		5:  newBackendMetric("max_sessions", "Maximum observed number of active sessions.", prometheus.GaugeValue, nil),
		6:  newBackendMetric("limit_sessions", "Configured session limit.", prometheus.GaugeValue, nil),
		7:  newBackendMetric("sessions_total", "Total number of sessions.", prometheus.CounterValue, nil),
		8:  newBackendMetric("bytes_in_total", "Current total of incoming bytes.", prometheus.CounterValue, nil),
		9:  newBackendMetric("bytes_out_total", "Current total of outgoing bytes.", prometheus.CounterValue, nil),
		13: newBackendMetric("connection_errors_total", "Total of connection errors.", prometheus.CounterValue, nil),
		14: newBackendMetric("response_errors_total", "Total of response errors.", prometheus.CounterValue, nil),
		15: newBackendMetric("retry_warnings_total", "Total of retry warnings.", prometheus.CounterValue, nil),
		16: newBackendMetric("redispatch_warnings_total", "Total of redispatch warnings.", prometheus.CounterValue, nil),
		17: newBackendMetric("up", "Current health status of the backend (1 = UP, 0 = DOWN).", prometheus.GaugeValue, nil),
		18: newBackendMetric("weight", "Total weight of the servers in the backend.", prometheus.GaugeValue, nil),
		19: newBackendMetric("current_server", "Current number of active servers", prometheus.GaugeValue, nil),
		30: newBackendMetric("server_selected_total", "Total number of times a server was selected, either for new sessions, or when re-dispatching.", prometheus.CounterValue, nil),
		33: newBackendMetric("current_session_rate", "Current number of sessions per second over last elapsed second.", prometheus.GaugeValue, nil),
		35: newBackendMetric("max_session_rate", "Maximum number of sessions per second.", prometheus.GaugeValue, nil),
		39: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "1xx"}),
		40: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "2xx"}),
		41: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "3xx"}),
		42: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "4xx"}),
		43: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "5xx"}),
		44: newBackendMetric("http_responses_total", "Total of HTTP responses.", prometheus.CounterValue, prometheus.Labels{"code": "other"}),
		49: newBackendMetric("client_aborts_total", "Total number of data transfers aborted by the client.", prometheus.CounterValue, nil),
		50: newBackendMetric("server_aborts_total", "Total number of data transfers aborted by the server.", prometheus.CounterValue, nil),
		51: newBackendMetric("compressor_bytes_in_total", "Number of HTTP response bytes fed to the compressor", prometheus.CounterValue, nil),
		52: newBackendMetric("compressor_bytes_out_total", "Number of HTTP response bytes emitted by the compressor", prometheus.CounterValue, nil),
		53: newBackendMetric("compressor_bytes_bypassed_total", "Number of bytes that bypassed the HTTP compressor", prometheus.CounterValue, nil),
		54: newBackendMetric("http_responses_compressed_total", "Number of HTTP responses that were compressed", prometheus.CounterValue, nil),
		58: newBackendMetric("http_queue_time_average_seconds", "Avg. HTTP queue time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		59: newBackendMetric("http_connect_time_average_seconds", "Avg. HTTP connect time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		60: newBackendMetric("http_response_time_average_seconds", "Avg. HTTP response time for last 1024 successful connections.", prometheus.GaugeValue, nil),
		61: newBackendMetric("http_total_time_average_seconds", "Avg. HTTP total time for last 1024 successful connections.", prometheus.GaugeValue, nil),
	}

	haproxyInfo = prometheus.NewDesc(prometheus.BuildFQName(namespace, "version", "info"), "HAProxy version info.", []string{"release_date", "version"}, nil)
	haproxyUp   = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Was the last scrape of HAProxy successful.", nil, nil)
)

// Exporter collects HAProxy stats from the given URI and exports them using
// the prometheus metrics package.
type Exporter struct {
	URI       string
	mutex     sync.RWMutex
	fetchInfo func() (io.ReadCloser, error)
	fetchStat func() (io.ReadCloser, error)

	up                             prometheus.Gauge
	totalScrapes, csvParseFailures prometheus.Counter
	serverMetrics                  map[int]metricInfo
	excludedServerStates           map[string]struct{}
}

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, sslVerify, proxyFromEnv bool, selectedServerMetrics map[int]metricInfo, excludedServerStates string, timeout time.Duration) (*Exporter, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var fetchInfo func() (io.ReadCloser, error)
	var fetchStat func() (io.ReadCloser, error)
	switch u.Scheme {
	case "http", "https", "file":
		fetchStat = fetchHTTP(uri, sslVerify, proxyFromEnv, timeout)
	case "unix":
		fetchInfo = fetchUnix("unix", u.Path, showInfoCmd, timeout)
		fetchStat = fetchUnix("unix", u.Path, showStatCmd, timeout)
	case "tcp":
		fetchInfo = fetchUnix("tcp", u.Host, showInfoCmd, timeout)
		fetchStat = fetchUnix("tcp", u.Host, showStatCmd, timeout)
	default:
		return nil, fmt.Errorf("unsupported scheme: %q", u.Scheme)
	}

	excludedServerStatesMap := map[string]struct{}{}
	for _, f := range strings.Split(excludedServerStates, ",") {
		excludedServerStatesMap[f] = struct{}{}
	}

	return &Exporter{
		URI:       uri,
		fetchInfo: fetchInfo,
		fetchStat: fetchStat,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the last scrape of haproxy successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_scrapes_total",
			Help:      "Current total HAProxy scrapes.",
		}),
		csvParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_csv_parse_failures_total",
			Help:      "Number of errors while parsing CSV.",
		}),
		serverMetrics:        selectedServerMetrics,
		excludedServerStates: excludedServerStatesMap,
	}, nil
}

// Describe describes all the metrics ever exported by the HAProxy exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range frontendMetrics {
		ch <- m.Desc
	}
	for _, m := range backendMetrics {
		ch <- m.Desc
	}
	for _, m := range e.serverMetrics {
		ch <- m.Desc
	}
	ch <- haproxyInfo
	ch <- haproxyUp
	ch <- e.totalScrapes.Desc()
	ch <- e.csvParseFailures.Desc()
}

// Collect fetches the stats from configured HAProxy location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	up := e.scrape(ch)

	ch <- prometheus.MustNewConstMetric(haproxyUp, prometheus.GaugeValue, up)
	ch <- e.totalScrapes
	ch <- e.csvParseFailures
}

func fetchHTTP(uri string, sslVerify, proxyFromEnv bool, timeout time.Duration) func() (io.ReadCloser, error) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: !sslVerify}}
	if proxyFromEnv {
		tr.Proxy = http.ProxyFromEnvironment
	}
	client := http.Client{
		Timeout:   timeout,
		Transport: tr,
	}

	return func() (io.ReadCloser, error) {
		resp, err := client.Get(uri)
		if err != nil {
			return nil, err
		}
		if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func fetchUnix(scheme, address, cmd string, timeout time.Duration) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		f, err := net.DialTimeout(scheme, address, timeout)
		if err != nil {
			return nil, err
		}
		if err := f.SetDeadline(time.Now().Add(timeout)); err != nil {
			f.Close()
			return nil, err
		}
		n, err := io.WriteString(f, cmd)
		if err != nil {
			f.Close()
			return nil, err
		}
		if n != len(cmd) {
			f.Close()
			return nil, errors.New("write error")
		}
		return f, nil
	}
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) (up float64) {
	e.totalScrapes.Inc()
	var err error

	if e.fetchInfo != nil {
		infoReader, err := e.fetchInfo()
		if err != nil {
			log.Println("E! failed to fetch haproxy info:", err)
			return 0
		}
		defer infoReader.Close()

		info, err := e.parseInfo(infoReader)
		if err != nil {
			log.Println("E! failed to parse haproxy info:", err)
		} else {
			ch <- prometheus.MustNewConstMetric(haproxyInfo, prometheus.GaugeValue, 1, info.ReleaseDate, info.Version)
		}
	}

	body, err := e.fetchStat()
	if err != nil {
		log.Println("E! failed to fetch haproxy stat:", err)
		return 0
	}
	defer body.Close()

	reader := csv.NewReader(body)
	reader.TrailingComma = true
	reader.Comment = '#'

loop:
	for {
		row, err := reader.Read()
		switch err {
		case nil:
		case io.EOF:
			break loop
		default:
			if _, ok := err.(*csv.ParseError); ok {
				log.Println("E! failed to parse csv:", err)
				e.csvParseFailures.Inc()
				continue loop
			}
			log.Println("E! failed to read csv:", err)
			return 0
		}
		e.parseRow(row, ch)
	}
	return 1
}

type versionInfo struct {
	ReleaseDate string
	Version     string
}

func (e *Exporter) parseInfo(i io.Reader) (versionInfo, error) {
	var version, releaseDate string
	s := bufio.NewScanner(i)
	for s.Scan() {
		line := s.Text()
		if !strings.Contains(line, ":") {
			continue
		}

		field := strings.SplitN(line, ": ", 2)
		switch field[0] {
		case "Release_date":
			releaseDate = field[1]
		case "Version":
			version = field[1]
		}
	}
	return versionInfo{ReleaseDate: releaseDate, Version: version}, s.Err()
}

func (e *Exporter) parseRow(csvRow []string, ch chan<- prometheus.Metric) {
	if len(csvRow) < minimumCsvFieldCount {
		log.Println("E! Parser received unexpected number of CSV fields", "min", minimumCsvFieldCount, "received", len(csvRow))
		e.csvParseFailures.Inc()
		return
	}

	pxname, svname, status, typ := csvRow[pxnameField], csvRow[svnameField], csvRow[statusField], csvRow[typeField]

	const (
		frontend = "0"
		backend  = "1"
		server   = "2"
	)

	switch typ {
	case frontend:
		e.exportCsvFields(frontendMetrics, csvRow, ch, pxname)
	case backend:
		e.exportCsvFields(backendMetrics, csvRow, ch, pxname)
	case server:

		if _, ok := e.excludedServerStates[status]; !ok {
			e.exportCsvFields(e.serverMetrics, csvRow, ch, pxname, svname)
		}
	}
}

func parseStatusField(value string) int64 {
	switch value {
	case "UP", "UP 1/3", "UP 2/3", "OPEN", "no check", "DRAIN":
		return 1
	case "DOWN", "DOWN 1/2", "NOLB", "MAINT", "MAINT(via)", "MAINT(resolution)":
		return 0
	default:
		return 0
	}
}

func (e *Exporter) exportCsvFields(metrics map[int]metricInfo, csvRow []string, ch chan<- prometheus.Metric, labels ...string) {
	for fieldIdx, metric := range metrics {
		if fieldIdx > len(csvRow)-1 {
			// We can't break here because we are not looping over the fields in sorted order.
			continue
		}
		valueStr := csvRow[fieldIdx]
		if valueStr == "" {
			continue
		}

		var err error = nil
		var value float64
		var valueInt int64

		switch fieldIdx {
		case statusField:
			valueInt = parseStatusField(valueStr)
			value = float64(valueInt)
		case checkDurationField, qtimeMsField, ctimeMsField, rtimeMsField, ttimeMsField:
			value, err = strconv.ParseFloat(valueStr, 64)
			value /= 1000
		default:
			valueInt, err = strconv.ParseInt(valueStr, 10, 64)
			value = float64(valueInt)
		}
		if err != nil {
			log.Println("E! Can't parse CSV field value", "value", valueStr, "err", err)
			e.csvParseFailures.Inc()
			continue
		}
		ch <- prometheus.MustNewConstMetric(metric.Desc, metric.Type, value, labels...)
	}
}

// filterServerMetrics returns the set of server metrics specified by the comma
// separated filter.
func filterServerMetrics(filter string) (map[int]metricInfo, error) {
	metrics := map[int]metricInfo{}
	if len(filter) == 0 {
		return metrics, nil
	}

	for _, f := range strings.Split(filter, ",") {
		field, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("invalid server metric field number: %v", f)
		}
		if metric, ok := serverMetrics[field]; ok {
			metrics[field] = metric
		}
	}

	return metrics, nil
}
