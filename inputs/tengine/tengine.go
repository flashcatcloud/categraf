package tengine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "tengine"

type Tengine struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`

	Mappings map[string]map[string]string `toml:"mappings"`
}

type TengineStatus struct {
	host                  string
	schema                string
	bytesIn               uint64
	bytesOut              uint64
	connTotal             uint64
	reqTotal              uint64
	http2xx               uint64
	http3xx               uint64
	http4xx               uint64
	http5xx               uint64
	httpOtherStatus       uint64
	rt                    uint64
	upsReq                uint64
	upsRt                 uint64
	upsTries              uint64
	http200               uint64
	http206               uint64
	http302               uint64
	http304               uint64
	http403               uint64
	http404               uint64
	http416               uint64
	http499               uint64
	http500               uint64
	http502               uint64
	http503               uint64
	http504               uint64
	http508               uint64
	httpOtherDetailStatus uint64
	httpUps4xx            uint64
	httpUps5xx            uint64
}

func (ins *Instance) Init() error {
	if len(ins.Urls) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 5)
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}
	ins.client = client

	for _, u := range ins.Urls {
		addr, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("failed to parse the url: %s, error: %v", u, err)
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			return fmt.Errorf("only http and https are supported, url: %s", u)
		}
	}

	return nil
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Tengine{}
	})
}

func (t *Tengine) Clone() inputs.Input {
	return &Tengine{}
}

func (t *Tengine) Name() string {
	return inputName
}

func (t *Tengine) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(t.Instances))
	for i := 0; i < len(t.Instances); i++ {
		if len(t.Instances[i].Mappings) == 0 {
			t.Instances[i].Mappings = t.Mappings
		} else {
			m := make(map[string]map[string]string)
			for k, v := range t.Mappings {
				m[k] = v
			}
			for k, v := range t.Instances[i].Mappings {
				m[k] = v
			}
			t.Instances[i].Mappings = m
		}
		ret[i] = t.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup

	if len(ins.Urls) == 0 {
		return
	}

	for _, u := range ins.Urls {
		addr, err := url.Parse(u)
		if err != nil {
			log.Println("E! failed to parse the url:", u, "error:", err)
			continue
		}
		wg.Add(1)
		go func(addr *url.URL) {
			defer wg.Done()
			if err := ins.gather(addr, slist); err != nil {
				log.Println("E!", err)
			}
		}(addr)
	}

	wg.Wait()
}

func (ins *Instance) gather(addr *url.URL, slist *types.SampleList) error {
	if ins.DebugMod {
		log.Println("D! tengine... url:", addr)
	}
	var tengineStatus TengineStatus

	resp, err := ins.client.Get(addr.String())
	if err != nil {
		return fmt.Errorf("error making HTTP request to %q: %w", addr.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP status %s", addr.String(), resp.Status)
	}
	r := bufio.NewReader(resp.Body)

	for {
		line, err := r.ReadString('\n')
		if err != nil || errors.Is(err, io.EOF) {
			break
		}

		lineSplit := strings.Split(strings.TrimSpace(line), ",")
		if len(lineSplit) != 31 {
			continue
		}
		tengineStatus.host = lineSplit[0]
		if err != nil {
			return err
		}
		tengineStatus.schema = strings.Split(lineSplit[1], ":")[1]
		if err != nil {
			return err
		}
		tengineStatus.bytesIn, err = strconv.ParseUint(lineSplit[2], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.bytesOut, err = strconv.ParseUint(lineSplit[3], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.connTotal, err = strconv.ParseUint(lineSplit[4], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.reqTotal, err = strconv.ParseUint(lineSplit[5], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http2xx, err = strconv.ParseUint(lineSplit[6], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http3xx, err = strconv.ParseUint(lineSplit[7], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http4xx, err = strconv.ParseUint(lineSplit[8], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http5xx, err = strconv.ParseUint(lineSplit[9], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.httpOtherStatus, err = strconv.ParseUint(lineSplit[10], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.rt, err = strconv.ParseUint(lineSplit[11], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.upsReq, err = strconv.ParseUint(lineSplit[12], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.upsRt, err = strconv.ParseUint(lineSplit[13], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.upsTries, err = strconv.ParseUint(lineSplit[14], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http200, err = strconv.ParseUint(lineSplit[15], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http206, err = strconv.ParseUint(lineSplit[16], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http302, err = strconv.ParseUint(lineSplit[17], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http304, err = strconv.ParseUint(lineSplit[18], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http403, err = strconv.ParseUint(lineSplit[19], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http404, err = strconv.ParseUint(lineSplit[20], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http416, err = strconv.ParseUint(lineSplit[21], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http499, err = strconv.ParseUint(lineSplit[22], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http500, err = strconv.ParseUint(lineSplit[23], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http502, err = strconv.ParseUint(lineSplit[24], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http503, err = strconv.ParseUint(lineSplit[25], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http504, err = strconv.ParseUint(lineSplit[26], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.http508, err = strconv.ParseUint(lineSplit[27], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.httpOtherDetailStatus, err = strconv.ParseUint(lineSplit[28], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.httpUps4xx, err = strconv.ParseUint(lineSplit[29], 10, 64)
		if err != nil {
			return err
		}
		tengineStatus.httpUps5xx, err = strconv.ParseUint(lineSplit[30], 10, 64)
		if err != nil {
			return err
		}
		tags := getTags(addr, tengineStatus.host, tengineStatus.schema)

		// Add extra tags in batches
		if m, ok := ins.Mappings[addr.String()]; ok {
			for k, v := range m {
				tags[k] = v
			}
		}

		fields := map[string]interface{}{
			"bytes_in":                 tengineStatus.bytesIn,
			"bytes_out":                tengineStatus.bytesOut,
			"conn_total":               tengineStatus.connTotal,
			"req_total":                tengineStatus.reqTotal,
			"http_2xx":                 tengineStatus.http2xx,
			"http_3xx":                 tengineStatus.http3xx,
			"http_4xx":                 tengineStatus.http4xx,
			"http_5xx":                 tengineStatus.http5xx,
			"http_other_status":        tengineStatus.httpOtherStatus,
			"rt":                       tengineStatus.rt,
			"ups_req":                  tengineStatus.upsReq,
			"ups_rt":                   tengineStatus.upsRt,
			"ups_tries":                tengineStatus.upsTries,
			"http_200":                 tengineStatus.http200,
			"http_206":                 tengineStatus.http206,
			"http_302":                 tengineStatus.http302,
			"http_304":                 tengineStatus.http304,
			"http_403":                 tengineStatus.http403,
			"http_404":                 tengineStatus.http404,
			"http_416":                 tengineStatus.http416,
			"http_499":                 tengineStatus.http499,
			"http_500":                 tengineStatus.http500,
			"http_502":                 tengineStatus.http502,
			"http_503":                 tengineStatus.http503,
			"http_504":                 tengineStatus.http504,
			"http_508":                 tengineStatus.http508,
			"http_other_detail_status": tengineStatus.httpOtherDetailStatus,
			"http_ups_4xx":             tengineStatus.httpUps4xx,
			"http_ups_5xx":             tengineStatus.httpUps5xx,
		}
		slist.PushSamples(inputName, fields, tags)
	}
	return nil
}

func getTags(addr *url.URL, serverName, serverSchema string) map[string]string {
	h := addr.Host
	host, port, err := net.SplitHostPort(h)
	if err != nil {
		host = addr.Host
		if addr.Scheme == "http" {
			port = "80"
		} else if addr.Scheme == "https" {
			port = "443"
		} else {
			port = ""
		}
	}

	if serverSchema == "443" {
		serverSchema = "https"
	} else {
		serverSchema = "http"
	}

	return map[string]string{"target": host, "target_port": port, "server_name": serverName, "server_schema": serverSchema}
}

type Instance struct {
	config.InstanceConfig

	Urls []string `toml:"urls"`

	ResponseTimeout config.Duration `toml:"response_timeout"`
	FollowRedirects bool            `toml:"follow_redirects"`
	Username        string          `toml:"username"`
	Password        string          `toml:"password"`
	Headers         []string        `toml:"headers"`

	// Mappings Set the mapping of extra tags in batches
	Mappings map[string]map[string]string `toml:"mappings"`

	tls.ClientConfig

	client *http.Client
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	trans := &http.Transport{
		DisableKeepAlives: true,
	}
	if ins.UseTLS {
		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ResponseTimeout),
	}

	if !ins.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client, nil
}
