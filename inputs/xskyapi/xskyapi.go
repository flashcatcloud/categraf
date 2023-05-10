package xskyapi

import (
	"bytes"
	"encoding/json"
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

const inputName = "xskyapi"

var (
	utf8BOM = []byte("\xef\xbb\xbf")
)

type XskyApi struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &XskyApi{}
	})
}

func (pt *XskyApi) Clone() inputs.Input {
	return &XskyApi{}
}

func (pt *XskyApi) Name() string {
	return inputName
}

func (pt *XskyApi) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	DssType         string            `toml:"dss_type"`
	Servers         []string          `toml:"servers"`
	TagKeys         []string          `toml:"tag_keys"`
	ResponseTimeout config.Duration   `toml:"response_timeout"`
	Parameters      map[string]string `toml:"parameters"`
	XmsAuthTokens   []string          `toml:"xms_auth_tokens"`
	tls.ClientConfig

	client httpClient
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (ins *Instance) Init() error {
	// check servers
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

	// check response_timeout
	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 3)
	}

	// initiate http client
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

	dialer := &net.Dialer{}
	if err != nil {
		return nil, err
	}

	trans := &http.Transport{
		DialContext:       dialer.DialContext,
		DisableKeepAlives: true,
		TLSClientConfig:   tlsCfg,
	}

	if ins.UseTLS {
		trans.TLSClientConfig = tlsCfg
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ResponseTimeout),
	}

	return client, nil
}

func (ins *Instance) Gather(slist *types.SampleList) {

	wg := new(sync.WaitGroup)
	for i, server := range ins.Servers {
		// Increment the WaitGroup counter.
		wg.Add(1)
		go func(server string, token string) {
			// Decrement the counter when the goroutine completes.
			defer wg.Done()
			ins.gather(slist, server, token)
		}(server, ins.XmsAuthTokens[i])
	}
	// Wait for all goroutines to complete.
	wg.Wait()
}

// Gathers data from a particular server

func (ins *Instance) gather(slist *types.SampleList, server string, token string) {
	if config.Config.DebugMode {
		log.Println("D! xskyapi... server:", server)
	}

	// acquire quota data of 3 mainstream distributed storage service provided by Xsky
	switch ins.DssType {
	case "oss": // object storage

		// oss users

		urlUsers := server + "/v1/os-users"

		resp, _, err := ins.sendRequest(urlUsers, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlUsers, "error:", err)
		}

		osUsers := OsUsers{}

		er := json.Unmarshal(resp, &osUsers)
		if er != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels := map[string]string{"server": server}
		fields := make(map[string]interface{})

		for _, user := range osUsers.OsUser {
			labels["name"] = user.Name
			labels["id"] = strconv.Itoa(user.ID)
			fields["oss_user_quota"] = user.UserQuotaMaxSize
			fields["oss_user_used_size"] = user.Samples[0].AllocatedSize
			slist.PushSamples(inputName, fields, labels)
		}

		// oss buckets

		urlBuckets := server + "/v1/os-buckets"

		resp, _, err = ins.sendRequest(urlBuckets, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlBuckets, "error:", err)
		}
		osBuckets := OsBuckets{}

		err = json.Unmarshal(resp, &osBuckets)
		if err != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels = map[string]string{"server": server}
		fields = make(map[string]interface{})

		for _, bucket := range osBuckets.OsBucket {
			labels["name"] = bucket.Name
			labels["id"] = strconv.Itoa(bucket.ID)
			labels["user_id"] = strconv.Itoa(bucket.Owner.ID)
			labels["user_name"] = bucket.Owner.Name
			fields["oss_bucket_quota"] = bucket.BucketQuotaMaxSize
			fields["oss_bucket_used_size"] = bucket.Samples[0].AllocatedSize
			slist.PushSamples(inputName, fields, labels)
		}

	case "gfs":

		// gfs dfs

		urlDfs := server + "/v1/dfs-quotas"

		resp, _, err := ins.sendRequest(urlDfs, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlDfs, "error:", err)
		}

		dfsQuotas := DfsQuotas{}

		er := json.Unmarshal(resp, &dfsQuotas)
		if er != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels := map[string]string{"server": server}
		fields := make(map[string]interface{})

		for _, dfsQuota := range dfsQuotas.DfsQuota {
			labels["name"] = dfsQuota.DfsPath.Name
			labels["id"] = strconv.Itoa(dfsQuota.DfsPath.ID)
			fields["dfs_quota"] = dfsQuota.SizeHardQuota
			slist.PushSamples(inputName, fields, labels)
		}

		// gfs block volumes

		urlBV := server + "/v1/block-volumes"

		resp, _, err = ins.sendRequest(urlBV, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlDfs, "error:", err)
		}

		blockVolumes := BlockVolumes{}

		err = json.Unmarshal(resp, &blockVolumes)
		if err != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels = map[string]string{"server": server}
		fields = make(map[string]interface{})

		for _, blockVolume := range blockVolumes.BlockVolume {
			labels["name"] = blockVolume.Name
			labels["id"] = strconv.Itoa(blockVolume.ID)
			fields["block_volume_quota"] = blockVolume.Size
			fields["block_volume_used_size"] = blockVolume.AllocatedSize
			slist.PushSamples(inputName, fields, labels)
		}

	case "eus":

		// eus-folder

		urlDfs := server + "/v1/fs-folders"

		resp, _, err := ins.sendRequest(urlDfs, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlDfs, "error:", err)
		}

		fsFolders := FsFolders{}

		er := json.Unmarshal(resp, &fsFolders)
		if er != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels := map[string]string{"server": server}
		fields := make(map[string]interface{})

		for _, fsFolder := range fsFolders.FsFolder {
			labels["name"] = fsFolder.Name
			labels["id"] = strconv.Itoa(fsFolder.ID)
			fields["dfs_quota"] = fsFolder.Size
			slist.PushSamples(inputName, fields, labels)
		}

		// eus block volumes

		urlBV := server + "/v1/block-volumes"

		resp, _, err = ins.sendRequest(urlBV, token)
		if err != nil {
			log.Println("E! failed to send request to xskyapi url:", urlDfs, "error:", err)
		}

		blockVolumes := BlockVolumes{}

		err = json.Unmarshal(resp, &blockVolumes)
		if err != nil {
			log.Printf("Parsing JSON string exception：%s\n", err)
		}

		labels = map[string]string{"server": server}
		fields = make(map[string]interface{})
		// log.Println("D! len(OsUsers):", len(osUsers.OsUser))

		for _, blockVolume := range blockVolumes.BlockVolume {
			labels["name"] = blockVolume.Name
			labels["id"] = strconv.Itoa(blockVolume.ID)
			fields["block_volume_quota"] = blockVolume.Size
			fields["block_volume_used_size"] = blockVolume.AllocatedSize
			slist.PushSamples(inputName, fields, labels)
		}
	default:
		log.Printf("E! dss_type %s not suppported, expected oss, gfs or eus", ins.DssType)
	}
}

func (ins *Instance) sendRequest(serverURL string, token string) ([]byte, float64, error) {
	// Prepare URL
	requestURL, _ := url.Parse(serverURL)
	log.Println("D! now parseurl:", requestURL)

	// Prepare request query and body
	data := url.Values{}

	params := requestURL.Query()
	for k, v := range ins.Parameters {
		params.Add(k, v)
	}
	requestURL.RawQuery = params.Encode()

	// Create + send request
	req, err := http.NewRequest("GET", requestURL.String(),
		strings.NewReader(data.Encode()))
	if err != nil {
		return []byte(""), -1, err
	}

	// Add header parameters
	req.Header.Add("Xms-Auth-Token", token)

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
	body = bytes.TrimPrefix(body, utf8BOM)

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
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Samples []struct {
		AllocatedSize int `json:"allocated_size"`
	} `json:"samples"`
	UserQuotaMaxSize int64 `json:"user_quota_max_size"`
}

type OsUsers struct {
	OsUser []*osUser `json:"os_users"`
}

type osBucket struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Owner struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"owner"`
	Samples []struct {
		AllocatedSize int `json:"allocated_size"`
	} `json:"samples"`
	BucketQuotaMaxSize int64 `json:"quota_max_size"`
}

type OsBuckets struct {
	OsBucket []*osBucket `json:"os_buckets"`
}

type dfsQuota struct {
	DfsPath struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"dfs_path"`
	SizeHardQuota int `json:"size_hard_quota"`
}

type DfsQuotas struct {
	DfsQuota []*dfsQuota `json:"dfs_quotas"`
}

type blockVolume struct {
	Name          string `json:"name"`
	AllocatedSize int    `json:"allocated_size"`
	ID            int    `json:"id"`
	Size          int    `json:"size"`
}

type BlockVolumes struct {
	BlockVolume []*blockVolume `json:"block_volumes"`
}

type fsFolder struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type FsFolders struct {
	FsFolder []*fsFolder `json:"fs_folders"`
}
