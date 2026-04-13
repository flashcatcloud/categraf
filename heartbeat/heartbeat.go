package heartbeat

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	keyset "flashcat.cloud/categraf/set/key"
	"io"
	"net"
	"net/http"
	"os"
	osExec "os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	cpuUtil "github.com/shirou/gopsutil/v3/cpu"
	"k8s.io/klog/v2"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs/system"
	"flashcat.cloud/categraf/pkg/cmdx"
)

const collinterval = 3

type (
	HeartbeatResponse struct {
		Data UpdateInfo `json:"dat"`
		Msg  string     `json:"err"`
	}
	UpdateInfo struct {
		NewVersion string `json:"new_version"`
		UpdateURL  string `json:"download_url"`
	}
)

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
		klog.ErrorS(err, "failed to create heartbeat client")
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
			klog.ErrorS(err, "failed to init tls")
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

func debugEnabled() bool {
	return klog.V(1).Enabled() && strings.Contains(config.Config.InputFilters, keyset.HeartbeatAgent)
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
		"global_labels": config.GlobalLabels(),
		"host_ip":       hostIP,
	}

	if ext, err := collectSystemInfo(); err == nil {
		data["extend_info"] = ext
		if cpuInfo, ok := ext.CPU.(map[string]string); ok {
			cpuNum := cpuInfo["cpu_logical_processors"]
			if num, err := strconv.Atoi(cpuNum); err == nil {
				data["cpu_num"] = num
			}
		}
	} else {
		klog.ErrorS(err, "failed to collect system info")
	}

	data["unixtime"] = time.Now().UnixMilli()
	bs, err := json.Marshal(data)
	if err != nil {
		klog.ErrorS(err, "failed to marshal heartbeat request")
		return
	}

	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(bs); err != nil {
		klog.ErrorS(err, "failed to write gzip buffer")
	}

	if err = g.Close(); err != nil {
		klog.ErrorS(err, "failed to close gzip buffer")
		return
	}
	if debugEnabled() {
		klog.V(1).InfoS("heartbeat request", "body", string(bs))
	}

	req, err := http.NewRequest("POST", config.Config.Heartbeat.Url, &buf)
	if err != nil {
		klog.ErrorS(err, "failed to create heartbeat request")
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
		klog.ErrorS(err, "failed to do heartbeat")
		return
	}

	defer res.Body.Close()
	bs, err = io.ReadAll(res.Body)
	if err != nil {
		klog.ErrorS(err, "failed to read heartbeat response body", "status_code", res.StatusCode)
		return
	}

	if res.StatusCode/100 != 2 {
		klog.Warningf("heartbeat status code: %d response: %s", res.StatusCode, string(bs))
		return
	}

	if debugEnabled() {
		klog.V(1).InfoS("heartbeat response", "body", string(bs), "status_code", res.StatusCode)
	}

	hr := HeartbeatResponse{}
	err = json.Unmarshal(bs, &hr)
	if err != nil {
		klog.Warningf("failed to unmarshal heartbeat response: %v", err)
		return
	}
	if len(hr.Data.NewVersion) != 0 && len(hr.Data.UpdateURL) != 0 && hr.Data.NewVersion != shortVersion && hr.Data.NewVersion != config.Version {
		var (
			out    bytes.Buffer
			stderr bytes.Buffer
		)
		exe, err := os.Executable()
		if err != nil {
			klog.ErrorS(err, "failed to get current executable")
			return
		}
		cmd := osExec.Command(exe, "-update", "-update_url", hr.Data.UpdateURL)
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err, timeout := cmdx.RunTimeout(cmd, time.Second*300)
		if timeout {
			klog.Warningf("exec %s timeout", cmd.String())
			return
		}
		if err != nil {
			klog.ErrorS(err, "failed to update categraf", "stderr", stderr.String(), "stdout", out.String(), "command", cmd.String())
			return
		}
		klog.Infof("update categraf(%s) from %s success, new version: %s", version(), hr.Data.UpdateURL, hr.Data.NewVersion)
	}
}

func memUsage(ps *system.SystemPS) float64 {
	vm, err := ps.VMStat()
	if err != nil {
		klog.ErrorS(err, "failed to get vmstat")
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
		klog.ErrorS(err, "failed to collect cpu_util")
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
		klog.ErrorS(err, "failed to collect cpu_util")
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
		klog.Warningf("current total CPU time is less than previous total CPU time")
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
