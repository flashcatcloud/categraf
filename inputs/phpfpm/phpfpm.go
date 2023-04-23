package phpfpm

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	fcgiclient "github.com/tomasen/fcgi_client"
)

const (
	inputName = "phpfpm"

	PfPool               = "pool"
	PfStartSince         = "start since"
	PfAcceptedConn       = "accepted conn"
	PfListenQueue        = "listen queue"
	PfMaxListenQueue     = "max listen queue"
	PfListenQueueLen     = "listen queue len"
	PfIdleProcesses      = "idle processes"
	PfActiveProcesses    = "active processes"
	PfTotalProcesses     = "total processes"
	PfMaxActiveProcesses = "max active processes"
	PfMaxChildrenReached = "max children reached"
	PfSlowRequests       = "slow requests"
)

type PhpFpm struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type metric map[string]int64
type poolStat map[string]metric

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &PhpFpm{}
	})
}

func (pt *PhpFpm) Clone() inputs.Input {
	return &PhpFpm{}
}

func (pt *PhpFpm) Name() string {
	return inputName
}

func (pt *PhpFpm) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Urls []string `toml:"urls"`

	ResponseTimeout config.Duration `toml:"response_timeout"`
	FollowRedirects bool            `toml:"follow_redirects"`
	Username        string          `toml:"username"`
	Password        string          `toml:"password"`
	Headers         []string        `toml:"headers"`

	tls.ClientConfig

	client *http.Client
}

func (ins *Instance) Init() error {
	if len(ins.Urls) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.ResponseTimeout < config.Duration(time.Second) {
		ins.ResponseTimeout = config.Duration(time.Second * 5)
	}
	return nil
}

func (ins *Instance) Gather(sList *types.SampleList) {
	var wg sync.WaitGroup

	if len(ins.Urls) == 0 {
		return
	}

	urls, err := expandUrls(ins.Urls)
	if err != nil {
		log.Println("E! failed to parse urls:", err)
		return
	}

	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if err := ins.gather(url, sList); err != nil {
				log.Println("E!", err)
			}
		}(u)
	}

	wg.Wait()
}

func (ins *Instance) gather(addr string, sList *types.SampleList) error {
	if config.Config.DebugMode {
		log.Println("D! php-fpm... url:", addr)
	}

	var resp *http.Response
	var respErr error

	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		resp, respErr = ins.gatherHTTP(addr)
	} else if strings.HasPrefix(addr, "fcgi://") || strings.HasPrefix(addr, "cgi://") {
		resp, respErr = ins.gatherFCGI(addr, "tcp")
	} else {
		resp, respErr = ins.gatherFCGI(addr, "unix")
	}

	if respErr != nil {
		return respErr
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("E! failed to close the body of client:", err)
		}
	}(resp.Body)

	importMetric(resp.Body, sList, addr)
	return nil
}

// gatherHttp handles links for the HTTP protocol.
func (ins *Instance) gatherHTTP(addr string) (*http.Response, error) {
	ins.initHTTPClient()

	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %s, error: %s", addr, err)
	}

	var body io.Reader
	request, err := http.NewRequest("GET", u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create an HTTP request, url: %s, error: %s", u.String(), err)
	}

	for i := 0; i < len(ins.Headers); i += 2 {
		request.Header.Add(ins.Headers[i], ins.Headers[i+1])
		if ins.Headers[i] == "Host" {
			request.Host = ins.Headers[i+1]
		}
	}

	if ins.Username != "" || ins.Password != "" {
		request.SetBasicAuth(ins.Username, ins.Password)
	}

	resp, err := ins.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to request the url: %s, error: %s", u.String(), err)
	}
	return resp, nil
}

// gatherFCGI handles links for the FastCGI protocol.
func (ins *Instance) gatherFCGI(addr, network string) (*http.Response, error) {
	networkAddr := ""
	scriptName := "/status"
	if network == "unix" {
		socketPath, statusPath := splitUnixSocketAddr(addr)
		if statusPath != "" {
			scriptName = statusPath
		}
		networkAddr = socketPath
	} else {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url: %s, error: %s", addr, err)
		}
		networkAddr = u.Host
		if u.Path != "" && u.Path != "/" {
			scriptName = u.Path
		}
	}

	env := make(map[string]string)
	env["SCRIPT_FILENAME"] = ""
	env["SCRIPT_NAME"] = scriptName
	env["SERVER_SOFTWARE"] = "go / fcgiClient "
	env["REMOTE_ADDR"] = "127.0.0.1"

	fcgi, err := fcgiclient.Dial(network, networkAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect the url: %s, error: %s", addr, err)
	}

	resp, err := fcgi.Get(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create a request,url: %s, error: %s", addr, err)
	}
	return resp, nil
}

// initHTTPClient initializes the HTTP client, if not already created
func (ins *Instance) initHTTPClient() {
	if ins.client == nil {
		client, err := ins.createHTTPClient()
		if err != nil {
			log.Printf("failed to create http client: %v", err)
		}
		ins.client = client
	}
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

// functions
func expandUrls(urls []string) ([]string, error) {
	newUrls := make([]string, 0, len(urls))
	for _, u := range urls {
		if isNetworkURL(u) {
			newUrls = append(newUrls, u)
			continue
		}
		// Unix socket file
		paths, err := expandUnixSocket(u)
		if err != nil {
			return nil, err
		}
		newUrls = append(newUrls, paths...)
	}
	return newUrls, nil
}

func isNetworkURL(addr string) bool {
	return strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") || strings.HasPrefix(addr, "fcgi://") || strings.HasPrefix(addr, "cgi://")
}

func expandUnixSocket(addr string) ([]string, error) {
	pathPattern, status := splitUnixSocketAddr(addr)

	paths, err := globUnixSocketPath(pathPattern)
	if err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("could not compile glob %s", pathPattern)
	}

	addresses := make([]string, 0, len(paths))
	for _, path := range paths {
		if status != "" {
			path = path + ":" + status
		}
		addresses = append(addresses, path)
	}

	return addresses, nil
}

func splitUnixSocketAddr(addr string) (socketPath string, statusPath string) {
	socketAddr := strings.Split(addr, ":")
	if len(socketAddr) >= 2 {
		socketPath = socketAddr[0]
		statusPath = socketAddr[1]
	} else {
		socketPath = socketAddr[0]
		statusPath = ""
	}

	return socketPath, statusPath
}

func globUnixSocketPath(pathPattern string) ([]string, error) {
	if len(pathPattern) == 0 {
		return nil, fmt.Errorf("the file is not exist")
	}

	// Check whether the path is an absolute path.
	if !filepath.IsAbs(pathPattern) {
		return nil, fmt.Errorf("the file is not absolute: %s", pathPattern)
	}

	if !strings.ContainsAny(pathPattern, "*?[") {
		file, err := os.Stat(pathPattern)
		// 无法识别文件或者该文件不是 unix socket 类型
		if err != nil || !isUnixSocketFile(file) {
			return nil, fmt.Errorf("the file is not of type socket：%s", pathPattern)
		}
		return []string{pathPattern}, nil
	}

	paths, err := filepath.Glob(pathPattern)
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("could not compile glob %s", pathPattern)
	}
	validPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		file, err := os.Stat(path)
		if err != nil || !isUnixSocketFile(file) {
			// 无法识别文件或者该文件不是 unix socket 类型
			continue
		}
		validPaths = append(validPaths, path)
	}
	return validPaths, nil
}

func isUnixSocketFile(info os.FileInfo) bool {
	return info.Mode().String()[0] == os.ModeSocket.String()[0]
}

func importMetric(r io.Reader, sList *types.SampleList, addr string) {
	stats := make(poolStat)
	var currentPool string

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		statLine := scanner.Text()
		keyValue := strings.Split(statLine, ":")

		if len(keyValue) < 2 {
			continue
		}
		fieldName := strings.Trim(keyValue[0], " ")
		// We start to gather data for a new pool here
		if fieldName == PfPool {
			currentPool = strings.Trim(keyValue[1], " ")
			stats[currentPool] = make(metric)
			continue
		}

		// Start to parse metric for current pool
		switch fieldName {
		case PfStartSince,
			PfAcceptedConn,
			PfListenQueue,
			PfMaxListenQueue,
			PfListenQueueLen,
			PfIdleProcesses,
			PfActiveProcesses,
			PfTotalProcesses,
			PfMaxActiveProcesses,
			PfMaxChildrenReached,
			PfSlowRequests:
			fieldValue, err := strconv.ParseInt(strings.Trim(keyValue[1], " "), 10, 64)
			if err == nil {
				stats[currentPool][fieldName] = fieldValue
			}
		}
	}

	// Finally, we push the pool metric
	for pool := range stats {
		tags := map[string]string{
			"pool": pool,
			"url":  addr,
		}
		fields := make(map[string]interface{})
		for k, v := range stats[pool] {
			fields[strings.ReplaceAll(k, " ", "_")] = v
		}
		sList.PushSamples("phpfpm", fields, tags)
	}
}
