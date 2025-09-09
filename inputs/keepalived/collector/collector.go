package collector

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/prometheus/client_golang/prometheus"
)

type Collector interface {
	Refresh() error
	ScriptVrrps() ([]VRRPScript, error)
	DataVrrps() (map[string]*VRRPData, error)
	StatsVrrps() (map[string]*VRRPStats, error)
	JSONVrrps() ([]VRRP, error)
	HasVRRPScriptStateSupport() bool
	HasJSONSignalSupport() (bool, error)
}

// KeepalivedCollector implements prometheus.Collector interface and stores required info to collect data.
type KeepalivedCollector struct {
	sync.Mutex
	useJSON    bool
	scriptPath string
	metrics    map[string]*prometheus.Desc
	collector  Collector
}

// VRRPStats represents Keepalived stats about VRRP.
type VRRPStats struct {
	AdvertRcvd        int `json:"advert_rcvd"`
	AdvertSent        int `json:"advert_sent"`
	BecomeMaster      int `json:"become_master"`
	ReleaseMaster     int `json:"release_master"`
	PacketLenErr      int `json:"packet_len_err"`
	AdvertIntervalErr int `json:"advert_interval_err"`
	IPTTLErr          int `json:"ip_ttl_err"`
	InvalidTypeRcvd   int `json:"invalid_type_rcvd"`
	AddrListErr       int `json:"addr_list_err"`
	InvalidAuthType   int `json:"invalid_authtype"`
	AuthTypeMismatch  int `json:"authtype_mismatch"`
	AuthFailure       int `json:"auth_failure"`
	PRIZeroRcvd       int `json:"pri_zero_rcvd"`
	PRIZeroSent       int `json:"pri_zero_sent"`
}

// VRRPData represents Keepalived data about VRRP.
type VRRPData struct {
	IName        string   `json:"iname"`
	State        int      `json:"state"`
	WantState    int      `json:"wantstate"`
	Intf         string   `json:"ifp_ifname"`
	GArpDelay    int      `json:"garp_delay"`
	VRID         int      `json:"vrid"`
	VIPs         []string `json:"vips"`
	ExcludedVIPs []string `json:"evips"`
}

// VRRPScript represents Keepalived script about VRRP.
type VRRPScript struct {
	Name   string
	Status string
	State  string
}

// VRRP ties together VRRPData and VRRPStats.
type VRRP struct {
	Data  VRRPData  `json:"data"`
	Stats VRRPStats `json:"stats"`
}

// KeepalivedStats ties together VRRP and VRRPScript.
type KeepalivedStats struct {
	VRRPs   []VRRP
	Scripts []VRRPScript
}

// NewKeepalivedCollector is creating new instance of KeepalivedCollector.
func NewKeepalivedCollector(useJSON bool, scriptPath string, collector Collector) *KeepalivedCollector {
	kc := &KeepalivedCollector{
		useJSON:    useJSON,
		scriptPath: scriptPath,
		collector:  collector,
	}

	kc.fillMetrics()

	return kc
}

func (k *KeepalivedCollector) newConstMetric(
	ch chan<- prometheus.Metric,
	name string,
	valueType prometheus.ValueType,
	value float64,
	lableValues ...string,
) {
	pm, err := prometheus.NewConstMetric(
		k.metrics[name],
		valueType,
		value,
		lableValues...,
	)
	if err != nil {
		slog.Error("Failed to create new const metric",
			"name", name,
			"valueType", valueType,
			"value", value,
			"lableValues", lableValues,
			"error", err,
		)

		return
	}

	ch <- pm
}

// Collect get metrics and add to prometheus metric channel.
func (k *KeepalivedCollector) Collect(ch chan<- prometheus.Metric) {
	k.Lock()
	defer k.Unlock()

	keepalivedUp := float64(1)

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Millisecond
	b.MaxElapsedTime = 2 * time.Second
	b.Reset()

	var keepalivedStats *KeepalivedStats

	if err := backoff.Retry(func() error {
		var err error
		keepalivedStats, err = k.getKeepalivedStats()
		if err != nil {
			slog.Debug("Failed to get keepalived stats",
				"error", err,
				"retryAfter", b.NextBackOff().String(),
			)
		}

		return err
	}, b); err != nil {
		slog.Error("No data found to be exported", "error", err)

		keepalivedUp = 0
	}

	k.newConstMetric(ch, "keepalived_up", prometheus.GaugeValue, keepalivedUp)

	if keepalivedUp == 0 {
		return
	}

	for _, vrrp := range keepalivedStats.VRRPs {
		k.newConstMetric(
			ch,
			"keepalived_advertisements_received_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AdvertRcvd),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_advertisements_sent_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AdvertSent),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_become_master_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.BecomeMaster),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_release_master_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.ReleaseMaster),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_packet_length_errors_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.PacketLenErr),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_advertisements_interval_errors_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AdvertIntervalErr),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_ip_ttl_errors_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.IPTTLErr),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_invalid_type_received_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.InvalidTypeRcvd),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_address_list_errors_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AddrListErr),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_authentication_invalid_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.InvalidAuthType),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_authentication_mismatch_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AuthTypeMismatch),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_authentication_failure_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.AuthFailure),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_priority_zero_received_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.PRIZeroRcvd),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_priority_zero_sent_total",
			prometheus.CounterValue,
			float64(vrrp.Stats.PRIZeroSent),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)
		k.newConstMetric(
			ch,
			"keepalived_gratuitous_arp_delay_total",
			prometheus.CounterValue,
			float64(vrrp.Data.GArpDelay),
			vrrp.Data.IName,
			vrrp.Data.Intf,
			strconv.Itoa(vrrp.Data.VRID),
		)

		for _, ip := range vrrp.Data.VIPs {
			ipAddr, intf, ok := ParseVIP(ip)
			if !ok {
				continue
			}

			k.newConstMetric(
				ch,
				"keepalived_vrrp_state",
				prometheus.GaugeValue,
				float64(vrrp.Data.State),
				vrrp.Data.IName,
				intf,
				strconv.Itoa(vrrp.Data.VRID),
				ipAddr,
			)

			if k.scriptPath != "" {
				checkScript := float64(0)
				if ok := k.checkScript(ipAddr); ok {
					checkScript = 1
				}

				k.newConstMetric(
					ch,
					"keepalived_check_script_status",
					prometheus.GaugeValue,
					checkScript,
					vrrp.Data.IName,
					intf,
					strconv.Itoa(vrrp.Data.VRID),
					ipAddr,
				)
			}
		}

		// iter over excluded vips
		for _, ip := range vrrp.Data.ExcludedVIPs {
			ipAddr, intf, ok := ParseVIP(ip)
			if !ok {
				continue
			}

			k.newConstMetric(
				ch,
				"keepalived_vrrp_excluded_state",
				prometheus.GaugeValue,
				float64(vrrp.Data.State),
				vrrp.Data.IName,
				intf,
				strconv.Itoa(vrrp.Data.VRID),
				ipAddr,
			)
		}

		// record vrrp_state metric even when VIPs are not defined, to support old keepalived release
		if len(vrrp.Data.VIPs) == 0 {
			k.newConstMetric(
				ch,
				"keepalived_vrrp_state",
				prometheus.GaugeValue,
				float64(vrrp.Data.State),
				vrrp.Data.IName,
				vrrp.Data.Intf,
				strconv.Itoa(vrrp.Data.VRID),
				"",
			)
		}
	}

	for _, script := range keepalivedStats.Scripts {
		if scriptStatus, ok := script.getIntStatus(); !ok {
			slog.Warn("Unknown script status",
				"status", script.Status,
				"name", script.Name,
			)
		} else {
			k.newConstMetric(ch, "keepalived_script_status", prometheus.GaugeValue, float64(scriptStatus), script.Name)
		}

		if k.collector.HasVRRPScriptStateSupport() {
			if scriptState, ok := script.getIntState(); !ok {
				slog.Warn("Unknown script state",
					"state", script.State,
					"name", script.Name,
				)
			} else {
				k.newConstMetric(ch, "keepalived_script_state", prometheus.GaugeValue, float64(scriptState), script.Name)
			}
		}
	}
}

func (k *KeepalivedCollector) getKeepalivedStats() (*KeepalivedStats, error) {
	stats := &KeepalivedStats{
		VRRPs:   make([]VRRP, 0),
		Scripts: make([]VRRPScript, 0),
	}

	var err error

	if err := k.collector.Refresh(); err != nil {
		return nil, err
	}

	if k.useJSON {
		stats.VRRPs, err = k.collector.JSONVrrps()
		if err != nil {
			return nil, err
		}

		return stats, nil
	}

	stats.Scripts, err = k.collector.ScriptVrrps()
	if err != nil {
		return nil, err
	}

	vrrpStats, err := k.collector.StatsVrrps()
	if err != nil {
		return nil, err
	}

	vrrpData, err := k.collector.DataVrrps()
	if err != nil {
		return nil, err
	}

	for name, v := range vrrpData {
		if v.VRID == 0 {
			return nil, fmt.Errorf("incomplete data: instance %s has vrid=0", name)
		}
	}

	if len(vrrpData) != len(vrrpStats) {
		slog.Debug("keepalived.data and keepalived.stats datas are not synced",
			"dataCount", len(vrrpData),
			"statsCount", len(vrrpStats),
		)

		return nil, errors.New("keepalived.data and keepalived.stats datas are not synced")
	}

	for instance, vData := range vrrpData {
		if vStat, ok := vrrpStats[instance]; ok {
			stats.VRRPs = append(stats.VRRPs, VRRP{
				Data:  *vData,
				Stats: *vStat,
			})
		} else {
			slog.Error("keepalived.stats does not contain stats for instance",
				"instance", instance,
			)

			return nil, errors.New("there is no stats found for instance")
		}
	}

	return stats, nil
}

func (k *KeepalivedCollector) checkScript(vip string) bool {
	var stdout, stderr bytes.Buffer

	script := k.scriptPath + " " + vip
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Error("Check script failed",
			"VIP", vip,
			"stdout", stdout.String(),
			"stderr", stderr.String(),
			"error", err,
		)

		return false
	}

	return true
}

// Describe outputs metrics descriptions.
func (k *KeepalivedCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range k.metrics {
		ch <- m
	}
}

func (k *KeepalivedCollector) fillMetrics() {
	commonLabels := []string{"iname", "intf", "vrid"}
	k.metrics = map[string]*prometheus.Desc{
		"keepalived_up": prometheus.NewDesc("keepalived_up", "Status", nil, nil),
		"keepalived_vrrp_state": prometheus.NewDesc(
			"keepalived_vrrp_state",
			"State of vrrp",
			[]string{"iname", "intf", "vrid", "ip_address"},
			nil,
		),
		"keepalived_vrrp_excluded_state": prometheus.NewDesc(
			"keepalived_vrrp_excluded_state",
			"State of vrrp with excluded VIP",
			[]string{"iname", "intf", "vrid", "ip_address"},
			nil,
		),
		"keepalived_check_script_status": prometheus.NewDesc(
			"keepalived_check_script_status",
			"Check Script status for each VIP",
			[]string{"iname", "intf", "vrid", "ip_address"},
			nil,
		),
		"keepalived_gratuitous_arp_delay_total": prometheus.NewDesc(
			"keepalived_gratuitous_arp_delay_total",
			"Gratuitous ARP delay",
			commonLabels,
			nil,
		),
		"keepalived_advertisements_received_total": prometheus.NewDesc(
			"keepalived_advertisements_received_total",
			"Advertisements received",
			commonLabels,
			nil,
		),
		"keepalived_advertisements_sent_total": prometheus.NewDesc(
			"keepalived_advertisements_sent_total",
			"Advertisements sent",
			commonLabels,
			nil,
		),
		"keepalived_become_master_total": prometheus.NewDesc(
			"keepalived_become_master_total",
			"Became master",
			commonLabels,
			nil,
		),
		"keepalived_release_master_total": prometheus.NewDesc(
			"keepalived_release_master_total",
			"Released master",
			commonLabels,
			nil,
		),
		"keepalived_packet_length_errors_total": prometheus.NewDesc(
			"keepalived_packet_length_errors_total",
			"Packet length errors",
			commonLabels,
			nil,
		),
		"keepalived_advertisements_interval_errors_total": prometheus.NewDesc(
			"keepalived_advertisements_interval_errors_total",
			"Advertisement interval errors",
			commonLabels,
			nil,
		),
		"keepalived_ip_ttl_errors_total": prometheus.NewDesc(
			"keepalived_ip_ttl_errors_total",
			"TTL errors",
			commonLabels,
			nil,
		),
		"keepalived_invalid_type_received_total": prometheus.NewDesc(
			"keepalived_invalid_type_received_total",
			"Invalid type errors",
			commonLabels,
			nil,
		),
		"keepalived_address_list_errors_total": prometheus.NewDesc(
			"keepalived_address_list_errors_total",
			"Address list errors",
			commonLabels,
			nil,
		),
		"keepalived_authentication_invalid_total": prometheus.NewDesc(
			"keepalived_authentication_invalid_total",
			"Authentication invalid",
			commonLabels,
			nil,
		),
		"keepalived_authentication_mismatch_total": prometheus.NewDesc(
			"keepalived_authentication_mismatch_total",
			"Authentication mismatch",
			commonLabels,
			nil,
		),
		"keepalived_authentication_failure_total": prometheus.NewDesc(
			"keepalived_authentication_failure_total",
			"Authentication failure",
			commonLabels,
			nil,
		),
		"keepalived_priority_zero_received_total": prometheus.NewDesc(
			"keepalived_priority_zero_received_total",
			"Priority zero received",
			commonLabels,
			nil,
		),
		"keepalived_priority_zero_sent_total": prometheus.NewDesc(
			"keepalived_priority_zero_sent_total",
			"Priority zero sent",
			commonLabels,
			nil,
		),
		"keepalived_script_status": prometheus.NewDesc(
			"keepalived_script_status",
			"Tracker Script Status",
			[]string{"name"},
			nil,
		),
		"keepalived_script_state": prometheus.NewDesc(
			"keepalived_script_state",
			"Tracker Script State",
			[]string{"name"},
			nil,
		),
	}
}
