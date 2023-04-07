package xskyapi

import (
	"bytes"
	"encoding/json"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
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
)

const inputName = "xskyapi"

var (
	utf8BOM = []byte("\xef\xbb\xbf")
)

//default

type XskyApi struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

//default

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &XskyApi{}
	})
}

//default

func (pt *XskyApi) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	DsCluster       string            `toml:"ds_cluster"`
	Servers         []string          `toml:"servers"`
	Method          string            `toml:"method"`
	TagKeys         []string          `toml:"tag_keys"`
	ResponseTimeout config.Duration   `toml:"response_timeout"`
	Parameters      map[string]string `toml:"parameters"`
	XmsAuthToken    string            `toml:"xms_auth_token"`
	tls.ClientConfig

	client httpClient
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	// 对toml的server进行不为空和url合法性检查
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}

	for _, server := range ins.Servers {
		addr, err := url.Parse(server)
		if err != nil {
			return fmt.Errorf("failed to parse http url: %s, error: %v", server, err)
		}

		if addr.Scheme != "http" && addr.Scheme != "https" {
			return fmt.Errorf("only http and https are supported, server: %s", server)
		}
	}

	// 对toml的response_timeout数据大小进行纠正，至少为3s
	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 3)
	}

	// 对toml的method为空的情况视为Get
	if ins.Method == "" {
		ins.Method = "GET"
	}

	// 实例client初始化
	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}

	ins.client = client

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	// 抄的http_response
	dialer := &net.Dialer{}
	if err != nil {
		return nil, err
	}

	trans := &http.Transport{
		DialContext:       dialer.DialContext,
		DisableKeepAlives: true,
		TLSClientConfig:   tlsCfg,
	}

	// 也是抄的，这数据不知道从哪来的
	if ins.UseTLS {
		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ResponseTimeout),
	}

	return client, nil
}

// Gathers data for all servers.

func (ins *Instance) Gather(slist *types.SampleList) {

	wg := new(sync.WaitGroup)
	for _, server := range ins.Servers {
		// Increment the WaitGroup counter.
		wg.Add(1)
		go func(server string) {
			// Decrement the counter when the goroutine completes.
			defer wg.Done()
			ins.gather(slist, server)
		}(server)
	}
	// Wait for all goroutines to complete.
	wg.Wait()
}

// Gathers data from a particular server

func (ins *Instance) gather(slist *types.SampleList, server string) error {
	if config.Config.DebugMode {
		log.Println("D! xskyapi... server:", server)
	}

	resp, _, err := ins.sendRequest(server)
	if err != nil {
		//log.Println("E! sendrequest error", err)
		return err
	}

	if strings.Index(server, "users") != -1 {
		osUsers := OsUsers{}

		er := json.Unmarshal(resp, &osUsers)
		if er != nil {
			fmt.Printf("解析json字符串异常：%s\n", err)
		}

		labels := map[string]string{"target": ins.DsCluster, "server": server}
		fields := make(map[string]interface{})
		//log.Println("D! len(OsUsers):", len(osUsers.OsUser))

		for _, user := range osUsers.OsUser {
			labels["name"] = user.Name
			labels["id"] = strconv.Itoa(user.ID)
			fields["oss_user_quota"] = user.UserQuotaMaxSize
			slist.PushSamples(inputName, fields, labels)
		}
	} else if strings.Index(server, "buckets") != -1 {
		osBuckets := OsBuckets{}

		er := json.Unmarshal(resp, &osBuckets)
		if er != nil {
			fmt.Printf("解析json字符串异常：%s\n", err)
		}

		labels := map[string]string{"target": ins.DsCluster, "server": server}
		fields := make(map[string]interface{})
		//log.Println("D! len(OsBucket):", len(osBuckets.OsBucket))

		for _, user := range osBuckets.OsBucket {
			labels["name"] = user.Name
			labels["id"] = strconv.Itoa(user.ID)
			fields["oss_bucket_quota"] = user.BucketQuotaMaxSize
			slist.PushSamples(inputName, fields, labels)
		}
	}

	return nil
}

func (ins *Instance) sendRequest(serverURL string) ([]byte, float64, error) {
	// Prepare URL
	requestURL, _ := url.Parse(serverURL)
	log.Println("D! now parseurl:", requestURL)

	// Prepare request query and body
	data := url.Values{}
	switch {
	case ins.Method == "GET":
		params := requestURL.Query()
		for k, v := range ins.Parameters {
			params.Add(k, v)
		}
		requestURL.RawQuery = params.Encode()

	case ins.Method == "POST":
		requestURL.RawQuery = ""
		for k, v := range ins.Parameters {
			data.Add(k, v)
		}
	}

	// Create + send request
	//log.Println("D! now creating request")
	req, err := http.NewRequest(ins.Method, requestURL.String(),
		strings.NewReader(data.Encode()))
	if err != nil {
		return []byte(""), -1, err
	}

	//log.Println("D! now adding heads")
	// Add header parameters
	req.Header.Add("Xms-Auth-Token", ins.XmsAuthToken)

	start := time.Now()
	resp, err := ins.client.Do(req)
	if err != nil {
		return []byte(nil), -1, err
	}

	defer resp.Body.Close()
	responseTime := time.Since(start).Seconds()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return body, responseTime, err
	}
	//log.Println("D! len(init url response body):", len(body))
	body = bytes.TrimPrefix(body, utf8BOM)
	//log.Println("D! len(after trimprefix url response body):", len(body))

	// Process response
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("response from url %q has status code %d (%s), expected %d (%s)",
			requestURL.String(),
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			http.StatusOK,
			http.StatusText(http.StatusOK))
		return body, responseTime, err
	}

	return body, responseTime, err
}

type osUser struct {
	//DisplayName string `json:"display_name"`
	//Email       string `json:"email"`
	ID int `json:"id"`
	//MaxBuckets                    int         `json:"max_buckets"`
	Name string `json:"name"`

	//Status              string    `json:"status"`
	//Suspended           bool      `json:"suspended"`
	//Update              time.Time `json:"update"`
	//UserQuotaMaxObjects int       `json:"user_quota_max_objects"`
	UserQuotaMaxSize int64 `json:"user_quota_max_size"`
}

type OsUsers struct {
	OsUser []*osUser `json:"os_users"`
}

type osBucket struct {
	ID int `json:"id"`
	//MaxBuckets                    int         `json:"max_buckets"`
	Name string `json:"name"`

	BucketQuotaMaxSize int64 `json:"quota_max_size"`
}

type OsBuckets struct {
	OsBucket []*osBucket `json:"os_buckets"`
}
