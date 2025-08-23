package redfish

import (
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/tidwall/gjson"
)

const (
	inputName = "redfish"
)

type Redfish struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Redfish{}
	})
}

func (r *Redfish) Clone() inputs.Input {
	return &Redfish{}
}

func (r *Redfish) Name() string {
	return inputName
}

func (r *Redfish) GetInstances() []inputs.Instance {
	out := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		out[i] = r.Instances[i]
	}
	return out
}

type Instance struct {
	Addresses    []Address `toml:"addresses"`
	Sets         []Set     `toml:"sets"`
	Disks        Disk      `toml:"disks"`
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

type Set struct {
	URN     string   `toml:"urn"`
	Prefix  string   `toml:"prefix"`
	Metrics []Metric `toml:"metrics"`
}

type Disk struct {
	URN      string   `toml:"urn"`
	FromData bool     `toml:"from_data"`
	LinkPath string   `toml:"link_path"`
	DataPath string   `toml:"data_path"`
	DataName string   `toml:"data_name"`
	DataTags []string `toml:"data_tags"`
	Child    *Disk    `toml:"child"`
}

type Metric struct {
	Name   string `toml:"name"`
	Prefix string `toml:"prefix"`
	Path   string `toml:"path"`
	Tags   []Tag  `toml:"tags"`
}

type Tag struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

func (i *Instance) Init() error {
	for idx, v := range i.Addresses {
		if v.URL == "" {
			return types.ErrInstancesEmpty
		}

		base, err := url.Parse(v.URL)
		if err != nil {
			return fmt.Errorf("parse URL: %w", err)
		}

		i.Addresses[idx].baseURL = base
		i.Addresses[idx].InitHTTPClientConfig()
		i.Addresses[idx].client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
			Timeout: time.Duration(v.Timeout),
		}
	}

	if len(i.Sets) == 0 {
		return errors.New("metrics are not requested")
	}

	return nil
}

func join(in ...string) string {
	var parts []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}

	return strings.Join(parts, "_")
}

func (i *Instance) Gather(sList *types.SampleList) {
	for _, a := range i.Addresses {
		if err := i.gatherRedfishUp(a, sList); err != nil {
			log.Println("E! error gatherRedfishAccess", err)
			continue
		}

		for _, s := range i.Sets {
			setUrl := a.baseURL.ResolveReference(&url.URL{Path: s.URN})

			js, err := a.getData(setUrl.String())
			if err != nil {
				log.Println("E! error getData", err)
				continue
			}

			for _, m := range s.Metrics {
				value := gjson.Get(js, m.Path)

				if !value.IsArray() {
					fields := make(map[string]interface{})
					fields[join(m.Prefix, m.Name)] = prepareValue(value)
					tags := map[string]string{
						i.AgentHostTag: a.baseURL.Host,
					}

					for _, v := range m.Tags {
						tmp := gjson.Get(js, v.Path)
						if !tmp.Exists() || tmp.IsArray() {
							continue
						}

						tags[v.Name] = tmp.String()
					}

					sList.PushSamples(s.Prefix, fields, tags)

					continue
				}

				for idx, v := range value.Array() {
					fields := make(map[string]interface{})
					fields[join(m.Prefix, m.Name)] = prepareValue(v)
					tags := map[string]string{
						i.AgentHostTag: a.baseURL.Host,
					}

					for _, t := range m.Tags {
						tmp := gjson.Get(js, t.Path)
						if !tmp.Exists() {
							continue
						}

						if len(tmp.Array()) > idx {
							tags[t.Name] = tmp.Array()[idx].String()
						} else {
							tags[t.Name] = tmp.String()
						}
					}

					sList.PushSamples(s.Prefix, fields, tags)
				}
			}
		}

		// Disk
		if i.Disks.LinkPath == "" || i.Disks.DataPath == "" || i.Disks.URN == "" {
			continue
		}

		if err := i.gatherDisks(a, &i.Disks, sList, ""); err != nil {
			log.Println("E! get disks data error", err)
			continue
		}
	}
}

func (i *Instance) gatherRedfishUp(a Address, sList *types.SampleList) error {
	url_ := a.baseURL.ResolveReference(&url.URL{Path: "redfish/v1/"})

	req, err := http.NewRequest(http.MethodGet, url_.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("client do: %w", err)
	}

	ping := 0
	if resp.StatusCode == 200 {
		ping = 1
	}

	fields := make(map[string]interface{})
	fields["up"] = ping
	tags := map[string]string{
		i.AgentHostTag: a.baseURL.Host,
	}

	sList.PushSamples("redfish", fields, tags)

	return nil
}

func (i *Instance) gatherDisks(a Address, d *Disk, sList *types.SampleList, prev string) error {
	if d == nil {
		return nil
	}

	var u *url.URL
	if !d.FromData {
		u = a.baseURL.ResolveReference(&url.URL{Path: d.URN})
	} else {
		u = a.baseURL.ResolveReference(&url.URL{Path: gjson.Get(prev, d.URN).String()})
	}

	js, err := a.getData(u.String())
	if err != nil {
		return err
	}

	for _, v := range gjson.Get(js, d.LinkPath).Array() {
		tmpU := a.baseURL.ResolveReference(&url.URL{Path: v.String()})

		js, err := a.getData(tmpU.String())
		if err != nil {
			return err
		}

		tmp := gjson.Get(js, d.DataPath)

		tags := map[string]string{
			i.AgentHostTag: a.baseURL.Host,
		}
		fields := make(map[string]interface{})

		fields[d.DataName] = prepareValue(tmp)

		for _, dt := range d.DataTags {
			tmp := gjson.Get(js, dt)
			if tmp.String() != "" {
				tags[dt] = tmp.String()
			}
		}

		sList.PushSamples("redfish", fields, tags)

		if err = i.gatherDisks(a, d.Child, sList, js); err != nil {
			return err
		}
	}

	return nil
}

func prepareValue(val gjson.Result) any {
	switch val.String() {
	case "OK":
		return 1
	case "Warning":
		return 2
	case "Critical":
		return 3
	case "Enabled":
		return 1
	case "Disabled":
		return 2
	default:
		return val.Value()
	}
}

func (i *Address) getData(uri string) (string, error) {
	req, err := http.NewRequest(i.Method, uri, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	username, err := i.Username.Get()
	if err != nil {
		return "", fmt.Errorf("getting username failed: %w", err)
	}
	user := username.String()
	username.Destroy()

	password, err := i.Password.Get()
	if err != nil {
		return "", fmt.Errorf("getting password failed: %w", err)
	}
	pass := password.String()
	password.Destroy()

	req.SetBasicAuth(user, pass)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("client do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("received status code %d (%s) for address %s, expected 200",
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			uri)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read all: %w", err)
	}

	return string(body), nil
}
