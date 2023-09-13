package nsq

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/httpx"
	"flashcat.cloud/categraf/types"
)

const inputName = "nsq"

const (
	requestPattern = `%s/stats?format=json`
)

type Nsq struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Nsq{}
	})
}

func (nsq *Nsq) Clone() inputs.Input {
	return &Nsq{}
}

func (nsq *Nsq) Name() string {
	return inputName
}

func (pt *Nsq) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	Targets []string `toml:"targets"`
	URL     string   `toml:"url"`

	config.HTTPCommonConfig

	client *http.Client
}

func (ins *Instance) Init() error {
	if len(ins.URL) != 0 {
		log.Println("W! url is deprecated, please use targets")
	}
	if len(ins.Targets) == 0 && len(ins.URL) == 0 {
		return types.ErrInstancesEmpty
	}

	ins.InitHTTPClientConfig()

	client, err := ins.createHTTPClient()
	if err != nil {
		return err
	}
	ins.client = client
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Targets) != 0 {
		var wg sync.WaitGroup
		for _, e := range ins.Targets {
			wg.Add(1)
			go func(e string) {
				defer wg.Done()
				ins.gatherEndpoint(e, slist)
			}(e)
		}

		wg.Wait()

	}
	// 兼容了旧的方法
	if len(ins.URL) != 0 {
		topics, err := ins.GetTopicInfo()
		if err != nil {
			log.Println("E! Failed to obtain the topic list error:", err)
		} else {
			for _, topic := range topics {
				v, err := ins.getQueuesInfo(topic)
				if err != nil {
					v = 0
					log.Println("E! Failed to obtain topic depth value error:", err)
				}
				fields := map[string]interface{}{
					"depth": v,
				}
				tags := map[string]string{
					"topic_name": topic,
				}

				slist.PushSamples(inputName, fields, tags)
			}
		}

	}

}

func (ins *Instance) gatherEndpoint(e string, slist *types.SampleList) {
	u, err := buildURL(e)
	if err != nil {
		log.Println("E! error buildURL", err)
		return
	}
	r, err := ins.client.Get(u.String())
	if err != nil {
		log.Println("E! error while polling", u.String(), err)
		return
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		log.Println(u.String(), "E! error while polling", r.Status)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println("E! error reading body", err)
		return
	}

	data := &NSQStatsData{}
	err = json.Unmarshal(body, data)
	if err != nil {
		log.Println("E! error parsing response", err)
		return

	}
	// Data was not parsed correctly attempt to use old format.
	if len(data.Version) < 1 {
		wrapper := &NSQStats{}
		err = json.Unmarshal(body, wrapper)
		if err != nil {
			log.Println("E! error parsing response", err)
			return

		}
		data = &wrapper.Data
	}

	tags := map[string]string{
		`server_host`:    u.Host,
		`server_version`: data.Version,
	}

	fields := make(map[string]interface{})
	if data.Health == `OK` {
		fields["server_count"] = int64(1)
	} else {
		fields["server_count"] = int64(0)
	}
	fields["topic_count"] = int64(len(data.Topics))

	slist.PushSamples("nsq_server", fields, tags)
	for _, t := range data.Topics {
		topicStats(t, slist, u.Host, data.Version)
	}
}

func buildURL(e string) (*url.URL, error) {
	u := fmt.Sprintf(requestPattern, e)
	addr, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("unable to parse address %q: %w", u, err)
	}
	return addr, nil
}

func topicStats(t TopicStats, slist *types.SampleList, host, version string) {
	// per topic overall (tag: name, paused, channel count)
	tags := map[string]string{
		"server_host":    host,
		"server_version": version,
		"topic":          t.Name,
	}

	fields := map[string]interface{}{
		"depth":         t.Depth,
		"backend_depth": t.BackendDepth,
		"message_count": t.MessageCount,
		"channel_count": int64(len(t.Channels)),
	}
	slist.PushSamples("nsq_topic", fields, tags)

	for _, c := range t.Channels {
		channelStats(c, slist, host, version, t.Name)
	}
}

func channelStats(c ChannelStats, slist *types.SampleList, host, version, topic string) {
	tags := map[string]string{
		"server_host":    host,
		"server_version": version,
		"topic":          topic,
		"channel":        c.Name,
	}

	fields := map[string]interface{}{
		"depth":          c.Depth,
		"backend_depth":  c.BackendDepth,
		"inflight_count": c.InFlightCount,
		"deferred_count": c.DeferredCount,
		"message_count":  c.MessageCount,
		"requeue_count":  c.RequeueCount,
		"timeout_count":  c.TimeoutCount,
		"client_count":   int64(len(c.Clients)),
	}

	slist.PushSamples("nsq_channel", fields, tags)
	for _, cl := range c.Clients {
		clientStats(cl, slist, host, version, topic, c.Name)
	}
}

func clientStats(c ClientStats, slist *types.SampleList, host, version, topic, channel string) {
	tags := map[string]string{
		"server_host":       host,
		"server_version":    version,
		"topic":             topic,
		"channel":           channel,
		"client_id":         c.ID,
		"client_hostname":   c.Hostname,
		"client_version":    c.Version,
		"client_address":    c.RemoteAddress,
		"client_user_agent": c.UserAgent,
		"client_tls":        strconv.FormatBool(c.TLS),
		"client_snappy":     strconv.FormatBool(c.Snappy),
		"client_deflate":    strconv.FormatBool(c.Deflate),
	}
	if len(c.Name) > 0 {
		tags["client_name"] = c.Name
	}

	fields := map[string]interface{}{
		"ready_count":    c.ReadyCount,
		"inflight_count": c.InFlightCount,
		"message_count":  c.MessageCount,
		"finish_count":   c.FinishCount,
		"requeue_count":  c.RequeueCount,
	}
	slist.PushSamples("nsq_client", fields, tags)
}

type NSQStats struct {
	Code int64        `json:"status_code"`
	Txt  string       `json:"status_txt"`
	Data NSQStatsData `json:"data"`
}

type NSQStatsData struct {
	Version   string       `json:"version"`
	Health    string       `json:"health"`
	StartTime int64        `json:"start_time"`
	Topics    []TopicStats `json:"topics"`
}

// e2e_processing_latency is not modeled
type TopicStats struct {
	Name         string         `json:"topic_name"`
	Depth        int64          `json:"depth"`
	BackendDepth int64          `json:"backend_depth"`
	MessageCount int64          `json:"message_count"`
	Paused       bool           `json:"paused"`
	Channels     []ChannelStats `json:"channels"`
}

// e2e_processing_latency is not modeled
type ChannelStats struct {
	Name          string        `json:"channel_name"`
	Depth         int64         `json:"depth"`
	BackendDepth  int64         `json:"backend_depth"`
	InFlightCount int64         `json:"in_flight_count"`
	DeferredCount int64         `json:"deferred_count"`
	MessageCount  int64         `json:"message_count"`
	RequeueCount  int64         `json:"requeue_count"`
	TimeoutCount  int64         `json:"timeout_count"`
	Paused        bool          `json:"paused"`
	Clients       []ClientStats `json:"clients"`
}

type ClientStats struct {
	Name                          string `json:"name"` // DEPRECATED 1.x+, still here as the structs are currently being shared for parsing v3.x and 1.x
	ID                            string `json:"client_id"`
	Hostname                      string `json:"hostname"`
	Version                       string `json:"version"`
	RemoteAddress                 string `json:"remote_address"`
	State                         int64  `json:"state"`
	ReadyCount                    int64  `json:"ready_count"`
	InFlightCount                 int64  `json:"in_flight_count"`
	MessageCount                  int64  `json:"message_count"`
	FinishCount                   int64  `json:"finish_count"`
	RequeueCount                  int64  `json:"requeue_count"`
	ConnectTime                   int64  `json:"connect_ts"`
	SampleRate                    int64  `json:"sample_rate"`
	Deflate                       bool   `json:"deflate"`
	Snappy                        bool   `json:"snappy"`
	UserAgent                     string `json:"user_agent"`
	TLS                           bool   `json:"tls"`
	TLSCipherSuite                string `json:"tls_cipher_suite"`
	TLSVersion                    string `json:"tls_version"`
	TLSNegotiatedProtocol         string `json:"tls_negotiated_protocol"`
	TLSNegotiatedProtocolIsMutual bool   `json:"tls_negotiated_protocol_is_mutual"`
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{}

	client := httpx.CreateHTTPClient(httpx.TlsConfig(tlsCfg),
		httpx.NetDialer(dialer), httpx.Proxy(httpx.GetProxyFunc(ins.HTTPProxyURL)),
		httpx.Timeout(time.Duration(ins.Timeout)),
		httpx.DisableKeepAlives(*ins.DisableKeepAlives),
		httpx.FollowRedirects(*ins.FollowRedirects))

	return client, err
}

// 兼容旧的方法
type Topic struct {
	Name     string `json:"name"`
	Channels []struct {
		Depth int `json:"depth"`
	} `json:"channels"`
}

type ApiData struct {
	Topics  []string `json:"topics"`
	Message string   `json:"message"`
}

func (ins *Instance) GetTopicInfo() ([]string, error) {
	req, err := http.NewRequest(ins.Method, ins.URL, ins.GetBody())
	if err != nil {
		return nil, err
	}
	ins.SetHeaders(req)

	resp, err := ins.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apidata ApiData
	if err := json.Unmarshal(body, &apidata); err != nil {
		return nil, err
	}

	return apidata.Topics, nil
}

func (ins *Instance) getQueuesInfo(topicName string) (int, error) {
	urlAll := fmt.Sprintf("%s/%s", ins.URL, topicName)

	req, err := http.NewRequest(ins.Method, urlAll, ins.GetBody())
	if err != nil {
		return 0, err
	}
	ins.SetHeaders(req)

	resp, err := ins.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}

	channels, ok := data["channels"].([]interface{})
	if !ok || len(channels) == 0 {
		return 0, nil
	}

	channel, ok := channels[0].(map[string]interface{})
	if !ok {
		return 0, nil
	}

	depth, ok := channel["depth"].(float64)
	if !ok {
		return 0, nil
	}

	return int(depth), nil
}
