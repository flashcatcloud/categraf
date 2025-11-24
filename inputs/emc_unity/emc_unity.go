package emc_unity

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const (
	inputName = "emc_unity"
)

type EmcUnity struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &EmcUnity{}
	})
}

func (r *EmcUnity) Clone() inputs.Input {
	return &EmcUnity{}
}

func (r *EmcUnity) Name() string {
	return inputName
}

func (r *EmcUnity) GetInstances() []inputs.Instance {
	out := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		out[i] = r.Instances[i]
	}
	return out
}

type Instance struct {
	Addresses    []Address `toml:"addresses"`
	AgentHostTag string    `toml:"agent_host_tag"`

	config.InstanceConfig
}

type Address struct {
	config.HTTPCommonConfig

	URL      string        `toml:"url"`
	Username config.Secret `toml:"username"`
	Password config.Secret `toml:"password"`

	client  *http.Client
	baseURL *url.URL
}

func (i *Instance) Init() error {
	for idx, v := range i.Addresses {
		if v.URL == "" {
			return errors.New("did not provide IP")
		}

		base, err := url.Parse(v.URL)
		if err != nil {
			return fmt.Errorf("parse URL: %w", err)
		}

		jar, _ := cookiejar.New(nil)
		i.Addresses[idx].baseURL = base
		i.Addresses[idx].InitHTTPClientConfig()
		i.Addresses[idx].client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
			Timeout: time.Duration(v.Timeout),
			Jar:     jar,
		}
	}

	return nil
}

func (i *Instance) Gather(sList *types.SampleList) {
	for _, a := range i.Addresses {
		if err := i.collectLun(a, sList); err != nil {
			log.Println("E! error collectLun:", err)
			continue
		}

		if err := i.collectCpu(a, sList); err != nil {
			log.Println("E! error collectCpu:", err)
			continue
		}

		if err := i.collectFibreChannel(a, sList); err != nil {
			log.Println("E! error collectFibreChannel:", err)
			continue
		}
	}
}

type KpiResp struct {
	Base    string    `json:"@base"`
	Updated time.Time `json:"updated"`
	Links   []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	} `json:"links"`
	Entries []struct {
		Content struct {
			Path      string             `json:"path"`
			StartTime time.Time          `json:"startTime"`
			EndTime   time.Time          `json:"endTime"`
			Interval  int                `json:"interval"`
			Values    map[string]float64 `json:"values"`
		} `json:"content"`
	} `json:"entries"`
}

func (i *Instance) collectFibreChannel(a Address, sList *types.SampleList) error {
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	u.Path = "/api/types/kpiValue/instances"

	now := time.Now().UTC().Add(-3*time.Minute).Format("2006-01-02T15:04:05") + ".935Z"

	query := u.Query()
	query.Set("visibility", "Engineering")
	filter := fmt.Sprintf(`(%s) AND (startTime eq "%s") AND interval eq 60`,
		`(path eq "kpi.fibreChannel.*.rw.+.throughput") OR`+
			`(path eq "kpi.fibreChannel.*.rw.read.throughput") OR`+
			`(path eq "kpi.fibreChannel.*.rw.write.throughput") OR`+
			`(path eq "kpi.fibreChannel.*.rw.+.bandwidth") OR`+
			`(path eq "kpi.fibreChannel.*.rw.read.bandwidth") OR`+
			`(path eq "kpi.fibreChannel.+.rw.write.bandwidth")`,
		now)

	query.Set("filter", filter)
	query.Set("per_page", "100")
	query.Set("page", "1")
	query.Set("compact", "true")

	u.RawQuery = query.Encode()

	dst := KpiResp{}
	if err = a.getData(u, &dst); err != nil {
		return fmt.Errorf("getData: %w", err)
	}

	for _, entry := range dst.Entries {
		var latest time.Time
		var val any
		for k, v := range entry.Content.Values {
			t, err := time.Parse(time.RFC3339, k)
			if err != nil {
				return fmt.Errorf("parse time: %w", err)
			}

			if t.After(latest) {
				latest = t
				val = v
			}
		}

		switch {
		case strings.Contains(entry.Content.Path, "rw.+.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_total_throughput"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "read.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_read_throughput"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "write.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_write_throughput"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "+.bandwidth"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_total_bandwidth"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "read.bandwidth"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_read_bandwidth"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "write.bandwidth"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["fibre_channel_write_bandwidth"] = val
			tags := map[string]string{
				i.AgentHostTag:  a.baseURL.Host,
				"fibre_channel": spl[2],
			}

			sList.PushSamples(inputName, fields, tags)
		}
	}

	return nil
}

func (i *Instance) collectCpu(a Address, sList *types.SampleList) error {
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	u.Path = "/api/types/kpiValue/instances"

	now := time.Now().UTC().Add(-3*time.Minute).Format("2006-01-02T15:04:05") + ".935Z"

	query := u.Query()
	query.Set("visibility", "Engineering")
	filter := fmt.Sprintf(`(%s) AND (startTime eq "%s") AND interval eq 60`,
		`(path eq "kpi.sp.spa.utilization") OR`+
			`(path eq "kpi.sp.spb.utilization")`,
		now)

	query.Set("filter", filter)
	query.Set("per_page", "100")
	query.Set("page", "1")
	query.Set("compact", "true")

	u.RawQuery = query.Encode()

	dst := KpiResp{}
	if err = a.getData(u, &dst); err != nil {
		return fmt.Errorf("getData: %w", err)
	}

	for _, entry := range dst.Entries {
		var latest time.Time
		var val any
		for k, v := range entry.Content.Values {
			t, err := time.Parse(time.RFC3339, k)
			if err != nil {
				return fmt.Errorf("parse time: %w", err)
			}

			if t.After(latest) {
				latest = t
				val = v
			}
		}

		switch {
		case strings.Contains(entry.Content.Path, "spa.utilization"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["total_cpu_utilization"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"sp":           "spa",
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.Contains(entry.Content.Path, "spb.utilization"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["total_cpu_utilization"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"sp":           "spb",
			}

			sList.PushSamples(inputName, fields, tags)
		}
	}

	return nil
}

func (i *Instance) collectLun(a Address, sList *types.SampleList) error {
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	u.Path = "/api/types/kpiValue/instances"

	now := time.Now().UTC().Add(-3*time.Minute).Format("2006-01-02T15:04:05") + ".935Z"

	query := u.Query()
	query.Set("visibility", "Engineering")
	filter := fmt.Sprintf(`(%s) AND (startTime eq "%s") AND interval eq 60`,
		`(path eq "kpi.lun.*.sp.+.rw.+.throughput") OR `+
			`(path eq "kpi.lun.*.sp.+.rw.read.throughput") OR `+
			`(path eq "kpi.lun.*.sp.+.rw.write.throughput") OR `+
			`(path eq "kpi.lun.*.sp.+.rw.+.ioSize") OR `+
			`(path eq "kpi.lun.*.sp.+.rw.read.ioSize") OR `+
			`(path eq "kpi.lun.*.sp.+.rw.write.ioSize") OR `+
			`(path eq "kpi.lun.*.sp.+.responseTime") OR `+
			`(path eq "kpi.lun.+.sp.+.rw.+.lunThroughput")`,
		now)

	query.Set("filter", filter)
	query.Set("per_page", "100")
	query.Set("page", "1")
	query.Set("compact", "true")

	u.RawQuery = query.Encode()

	dst := KpiResp{}
	if err = a.getData(u, &dst); err != nil {
		return fmt.Errorf("getData: %w", err)
	}

	for _, entry := range dst.Entries {
		var latest time.Time
		var val any
		for k, v := range entry.Content.Values {
			t, err := time.Parse(time.RFC3339, k)
			if err != nil {
				return fmt.Errorf("parse time: %w", err)
			}

			if t.After(latest) {
				latest = t
				val = v
			}
		}

		switch {
		case strings.HasSuffix(entry.Content.Path, "rw.+.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_total_iops"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "rw.read.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_read_iops"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "rw.write.throughput"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_write_iops"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "rw.+.ioSize"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_total_io_size"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "rw.read.ioSize"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_read_io_size"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "rw.write.ioSize"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_write_io_size"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)

		case strings.HasSuffix(entry.Content.Path, "sp.+.responseTime"):
			spl := strings.Split(entry.Content.Path, ".")
			if len(spl) < 2 {
				log.Println("E! error parse path:", "unexpected path")
				continue
			}

			fields := make(map[string]interface{})
			fields["lun_response_time"] = val
			tags := map[string]string{
				i.AgentHostTag: a.baseURL.Host,
				"lun_name":     spl[2],
			}

			sList.PushSamples(inputName, fields, tags)
		}
	}

	return nil
}

func (i *Address) getData(u *url.URL, dst any) error {
	req, err := http.NewRequest(i.Method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	username, err := i.Username.Get()
	if err != nil {
		return fmt.Errorf("getting username failed: %w", err)
	}
	user := username.String()
	username.Destroy()

	password, err := i.Password.Get()
	if err != nil {
		return fmt.Errorf("getting password failed: %w", err)
	}
	pass := password.String()
	password.Destroy()

	req.SetBasicAuth(user, pass)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-EMC-REST-CLIENT", "true")

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("client do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("received status code %d (%s) for address %s, expected 200",
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			u.String())
	}

	return json.NewDecoder(resp.Body).Decode(&dst)
}
