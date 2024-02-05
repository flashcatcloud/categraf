package heartbeat

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs/system"
	cpuUtil "github.com/shirou/gopsutil/v3/cpu"
)

const collinterval = 3

func Work() {
	conf := config.Config.Heartbeat

	if conf == nil || !conf.Enable {
		return
	}

	ps := system.NewSystemPS()

	interval := conf.Interval
	if interval <= 4 {
		interval = 4
	}

	client, err := newHTTPClient()
	if err != nil {
		log.Println("E! failed to create heartbeat client:", err)
		return
	}

	duration := time.Second * time.Duration(interval-collinterval)

	for {
		work(ps, client)
		time.Sleep(duration)
	}
}

func newHTTPClient() (*http.Client, error) {
	proxy, err := config.Config.Heartbeat.Proxy()
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(config.Config.Heartbeat.Timeout) * time.Millisecond

	trans := &http.Transport{
		Proxy: proxy,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(config.Config.Heartbeat.DialTimeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: timeout,
		MaxIdleConnsPerHost:   config.Config.Heartbeat.MaxIdleConnsPerHost,
	}

	if strings.HasPrefix(config.Config.Heartbeat.Url, "https:") {
		tlsCfg, err := config.Config.Heartbeat.TLSConfig()
		if err != nil {
			log.Println("E! failed to init tls:", err)
			return nil, err
		}

		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   timeout,
	}

	return client, nil
}

func version() string {
	components := strings.Split(config.Version, "-")
	switch len(components) {
	case 2:
		return components[0]
	case 3, 4:
		return components[0] + "-" + components[1]
	}
	return config.Version
}

func work(ps *system.SystemPS, client *http.Client) {
	cpuUsagePercent := cpuUsage(ps)
	hostname := config.Config.GetHostname()
	memUsagePercent := memUsage(ps)

	shortVersion := version()
	hostIP := config.Config.GetHostIP()
	data := map[string]interface{}{
		"agent_version": shortVersion,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"hostname":      hostname,
		"cpu_num":       runtime.NumCPU(),
		"cpu_util":      cpuUsagePercent,
		"mem_util":      memUsagePercent,
		"unixtime":      time.Now().UnixMilli(),
		"host_ip":       hostIP,
	}

	if ext, err := collectSystemInfo(); err == nil {
		data["extend_info"] = ext
		if cpuInfo, ok := ext.CPU.(map[string]string); ok {
			cpuNum := cpuInfo["cpu_cores"]
			if num, err := strconv.Atoi(cpuNum); err == nil {
				data["cpu_num"] = num
			}
		}
	} else {
		log.Println("E! failed to collect system info:", err)
	}

	bs, err := json.Marshal(data)
	if err != nil {
		log.Println("E! failed to marshal heartbeat request:", err)
		return
	}

	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(bs); err != nil {
		log.Println("E! failed to write gzip buffer:", err)
	}

	if err = g.Close(); err != nil {
		log.Println("E! failed to close gzip buffer:", err)
		return
	}
	if config.Config.DebugMode {
		log.Printf("D! heartbeat request: %s", string(bs))
	}

	req, err := http.NewRequest("POST", config.Config.Heartbeat.Url, &buf)
	if err != nil {
		log.Println("E! failed to new heartbeat request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("User-Agent", "categraf/"+hostIP)

	for i := 0; i < len(config.Config.Heartbeat.Headers); i += 2 {
		req.Header.Add(config.Config.Heartbeat.Headers[i], config.Config.Heartbeat.Headers[i+1])
		if config.Config.Heartbeat.Headers[i] == "Host" {
			req.Host = config.Config.Heartbeat.Headers[i+1]
		}
	}

	if config.Config.Heartbeat.BasicAuthPass != "" {
		req.SetBasicAuth(config.Config.Heartbeat.BasicAuthUser, config.Config.Heartbeat.BasicAuthPass)
	}

	res, err := client.Do(req)
	if err != nil {
		log.Println("E! failed to do heartbeat:", err)
		return
	}

	defer res.Body.Close()
	bs, err = ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println("E! failed to read heartbeat response body:", err, " status code:", res.StatusCode)
		return
	}

	if res.StatusCode/100 != 2 {
		log.Println("E! heartbeat status code:", res.StatusCode, " response:", string(bs))
		return
	}

	if config.Config.DebugMode {
		log.Println("D! heartbeat response:", string(bs), "status code:", res.StatusCode)
	}
}

func memUsage(ps *system.SystemPS) float64 {
	vm, err := ps.VMStat()
	if err != nil {
		log.Println("E! failed to get vmstat:", err)
		return 0
	}

	return vm.UsedPercent
}

func cpuUsage(ps *system.SystemPS) float64 {
	var (
		lastTotal  float64
		lastActive float64
		total      float64
		active     float64
	)

	// first
	times, err := ps.CPUTimes(false, true)
	if err != nil {
		log.Println("E! failed to collect cpu_util:", err)
		return 0
	}

	for _, cts := range times {
		lastTotal = totalCPUTime(cts)
		lastActive = activeCPUTime(cts)
		break
	}

	// sleep
	time.Sleep(time.Second * collinterval)

	// sencond
	times, err = ps.CPUTimes(false, true)
	if err != nil {
		log.Println("E! failed to collect cpu_util:", err)
		return 0
	}

	for _, cts := range times {
		total = totalCPUTime(cts)
		active = activeCPUTime(cts)
		break
	}

	// compute
	totalDelta := total - lastTotal
	if totalDelta < 0 {
		log.Println("W! current total CPU time is less than previous total CPU time")
		return 0
	}

	if totalDelta == 0 {
		return 0
	}

	return 100 * (active - lastActive) / totalDelta
}

func totalCPUTime(t cpuUtil.TimesStat) float64 {
	total := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Idle
	return total
}

func activeCPUTime(t cpuUtil.TimesStat) float64 {
	active := totalCPUTime(t) - t.Idle
	return active
}
