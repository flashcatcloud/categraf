package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "jenkins"

type Jenkins struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Jenkins{}
	})
}

func (j *Jenkins) Clone() inputs.Input {
	return &Jenkins{}
}

func (j *Jenkins) Name() string {
	return inputName
}

func (j *Jenkins) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(j.Instances))
	for i := 0; i < len(j.Instances); i++ {
		ret[i] = j.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URL      string `toml:"jenkins_url"`
	Username string `toml:"jenkins_username"`
	Password string `toml:"jenkins_password"`
	Source   string `toml:"jenkins_source"`
	Port     string `toml:"jenkins_port"`
	// HTTP Timeout specified as a string - 3s, 1m, 1h
	ResponseTimeout config.Duration

	tls.ClientConfig
	client *client

	MaxConnections    int             `toml:"max_connections"`
	MaxBuildAge       config.Duration `toml:"max_build_age"`
	MaxSubJobDepth    int             `toml:"max_subjob_depth"`
	MaxSubJobPerLayer int             `toml:"max_subjob_per_layer"`
	JobExclude        []string        `toml:"job_exclude"`
	JobInclude        []string        `toml:"job_include"`
	jobFilter         filter.Filter

	NodeExclude []string `toml:"node_exclude"`
	NodeInclude []string `toml:"node_include"`
	nodeFilter  filter.Filter

	semaphore chan struct{}
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.client == nil {
		client, err := ins.newHTTPClient()
		if err != nil {
			log.Println("E! failed to new HTTPClient:", err)
			return
		}

		if err = ins.initialize(client); err != nil {
			log.Println("E! failed to initialize:", err)
			return
		}
	}

	ins.gatherNodesData(slist)
	ins.gatherJobs(slist)
}

// ///////////////////////////////////////////////////////////

// measurement
const (
	measurementNode = "node_"
	measurementJob  = "job_"
)

func (ins *Instance) newHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("error parse jenkins config[%s]: %v", ins.URL, err)
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
			MaxIdleConns:    ins.MaxConnections,
		},
		Timeout: time.Duration(ins.ResponseTimeout),
	}, nil
}

// separate the client as dependency to use httptest Client for mocking
func (ins *Instance) initialize(client *http.Client) error {
	var err error

	// init jenkins tags
	u, err := url.Parse(ins.URL)
	if err != nil {
		return err
	}
	if u.Port() == "" {
		if u.Scheme == "http" {
			ins.Port = "80"
		} else if u.Scheme == "https" {
			ins.Port = "443"
		}
	} else {
		ins.Port = u.Port()
	}
	ins.Source = u.Hostname()

	// init filters
	ins.jobFilter, err = filter.NewIncludeExcludeFilter(ins.JobInclude, ins.JobExclude)
	if err != nil {
		return fmt.Errorf("error compiling job filters[%s]: %v", ins.URL, err)
	}
	ins.nodeFilter, err = filter.NewIncludeExcludeFilter(ins.NodeInclude, ins.NodeExclude)
	if err != nil {
		return fmt.Errorf("error compiling node filters[%s]: %v", ins.URL, err)
	}

	// init tcp pool with default value
	if ins.MaxConnections <= 0 {
		ins.MaxConnections = 5
	}

	// default sub jobs can be acquired
	if ins.MaxSubJobPerLayer <= 0 {
		ins.MaxSubJobPerLayer = 10
	}

	ins.semaphore = make(chan struct{}, ins.MaxConnections)

	ins.client = newClient(client, ins.URL, ins.Username, ins.Password, ins.MaxConnections)

	return ins.client.init()
}

func (ins *Instance) gatherNodeData(n node, slist *types.SampleList) error {
	tags := map[string]string{}
	if n.DisplayName == "" {
		return fmt.Errorf("error empty node name")
	}
	// filter out excluded or not included node_name
	if !ins.nodeFilter.Match(tags["node_name"]) {
		return nil
	}
	tags["node_name"] = n.DisplayName

	monitorData := n.MonitorData

	if monitorData.HudsonNodeMonitorsArchitectureMonitor != "" {
		tags["arch"] = monitorData.HudsonNodeMonitorsArchitectureMonitor
	}

	if n.Offline {
		slist.PushSample(inputName, "up", 0, tags)
	} else {
		slist.PushSample(inputName, "up", 1, tags)
	}

	tags["source"] = ins.Source
	tags["port"] = ins.Port

	fields := make(map[string]interface{})
	fields[measurementNode+"num_executors"] = n.NumExecutors

	if monitorData.HudsonNodeMonitorsResponseTimeMonitor != nil {
		fields[measurementNode+"response_time"] = monitorData.HudsonNodeMonitorsResponseTimeMonitor.Average
	}
	if monitorData.HudsonNodeMonitorsDiskSpaceMonitor != nil {
		tags["disk_path"] = monitorData.HudsonNodeMonitorsDiskSpaceMonitor.Path
		fields[measurementNode+"disk_available"] = monitorData.HudsonNodeMonitorsDiskSpaceMonitor.Size
	}
	if monitorData.HudsonNodeMonitorsTemporarySpaceMonitor != nil {
		tags["temp_path"] = monitorData.HudsonNodeMonitorsTemporarySpaceMonitor.Path
		fields[measurementNode+"temp_available"] = monitorData.HudsonNodeMonitorsTemporarySpaceMonitor.Size
	}
	if monitorData.HudsonNodeMonitorsSwapSpaceMonitor != nil {
		fields[measurementNode+"swap_available"] = monitorData.HudsonNodeMonitorsSwapSpaceMonitor.SwapAvailable
		fields[measurementNode+"memory_available"] = monitorData.HudsonNodeMonitorsSwapSpaceMonitor.MemoryAvailable
		fields[measurementNode+"swap_total"] = monitorData.HudsonNodeMonitorsSwapSpaceMonitor.SwapTotal
		fields[measurementNode+"memory_total"] = monitorData.HudsonNodeMonitorsSwapSpaceMonitor.MemoryTotal
	}
	slist.PushSamples(inputName, fields, tags)
	return nil
}

func (ins *Instance) gatherNodesData(slist *types.SampleList) {
	nodeResp, err := ins.client.getAllNodes(context.Background())
	if err != nil {
		log.Println("E! gatherNodesData", err)
		return
	}

	// get total and busy executors
	tags := map[string]string{"source": ins.Source, "port": ins.Port}
	fields := make(map[string]interface{})
	fields["busy_executors"] = nodeResp.BusyExecutors
	fields["total_executors"] = nodeResp.TotalExecutors
	slist.PushSamples(inputName, fields, tags)
	// get node data
	for _, node := range nodeResp.Computers {
		err = ins.gatherNodeData(node, slist)
		if err == nil {
			continue
		}
	}
}

func (ins *Instance) gatherJobs(slist *types.SampleList) {
	js, err := ins.client.getJobs(context.Background(), nil)
	if err != nil {
		log.Println("E! gatherJobs", err)
		return
	}
	var wg sync.WaitGroup
	for _, job := range js.Jobs {
		wg.Add(1)
		go func(name string, wg *sync.WaitGroup, slist *types.SampleList) {
			defer wg.Done()
			if err := ins.getJobDetail(jobRequest{
				name:    name,
				parents: []string{},
				layer:   0,
			}, slist); err != nil {
				log.Println("E! getJobDetail", err)
			}
		}(job.Name, &wg, slist)
	}
	wg.Wait()
}

func (ins *Instance) getJobDetail(jr jobRequest, slist *types.SampleList) error {
	if ins.MaxSubJobDepth > 0 && jr.layer == ins.MaxSubJobDepth {
		return nil
	}
	// filter out excluded or not included jobs
	if !ins.jobFilter.Match(jr.hierarchyName()) {
		return nil
	}
	js, err := ins.client.getJobs(context.Background(), &jr)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for k, ij := range js.Jobs {
		if k < len(js.Jobs)-ins.MaxSubJobPerLayer-1 {
			continue
		}
		wg.Add(1)
		// schedule tcp fetch for inner jobs
		go func(ij innerJob, jr jobRequest, slist *types.SampleList) {
			defer wg.Done()
			if err := ins.getJobDetail(jobRequest{
				name:    ij.Name,
				parents: jr.combined(),
				layer:   jr.layer + 1,
			}, slist); err != nil {
				log.Println("E! getJobDetail", err)
			}
		}(ij, jr, slist)
	}
	wg.Wait()

	// collect build info
	number := js.LastBuild.Number
	if number < 1 {
		// no build info
		return nil
	}
	build, err := ins.client.getBuild(context.Background(), jr, number)
	if err != nil {
		return err
	}

	if build.Building {
		if config.Config.DebugMode {
			log.Println("Ignore running build on ", jr.name, "build", number)
		}
		return nil
	}

	// stop if build is too old
	// Higher up in gatherJobs
	cutoff := time.Now().Add(-1 * time.Duration(ins.MaxBuildAge))

	// Here we just test
	if build.GetTimestamp().Before(cutoff) {
		return nil
	}

	ins.gatherJobBuild(jr, build, slist)
	return nil
}

type nodeResponse struct {
	Computers      []node `json:"computer"`
	BusyExecutors  int    `json:"busyExecutors"`
	TotalExecutors int    `json:"totalExecutors"`
}

type node struct {
	DisplayName  string      `json:"displayName"`
	Offline      bool        `json:"offline"`
	NumExecutors int         `json:"numExecutors"`
	MonitorData  monitorData `json:"monitorData"`
}

type monitorData struct {
	HudsonNodeMonitorsArchitectureMonitor   string               `json:"hudson.node_monitors.ArchitectureMonitor"`
	HudsonNodeMonitorsDiskSpaceMonitor      *nodeSpaceMonitor    `json:"hudson.node_monitors.DiskSpaceMonitor"`
	HudsonNodeMonitorsResponseTimeMonitor   *responseTimeMonitor `json:"hudson.node_monitors.ResponseTimeMonitor"`
	HudsonNodeMonitorsSwapSpaceMonitor      *swapSpaceMonitor    `json:"hudson.node_monitors.SwapSpaceMonitor"`
	HudsonNodeMonitorsTemporarySpaceMonitor *nodeSpaceMonitor    `json:"hudson.node_monitors.TemporarySpaceMonitor"`
}

type nodeSpaceMonitor struct {
	Path string  `json:"path"`
	Size float64 `json:"size"`
}

type responseTimeMonitor struct {
	Average int64 `json:"average"`
}

type swapSpaceMonitor struct {
	SwapAvailable   float64 `json:"availableSwapSpace"`
	SwapTotal       float64 `json:"totalSwapSpace"`
	MemoryAvailable float64 `json:"availablePhysicalMemory"`
	MemoryTotal     float64 `json:"totalPhysicalMemory"`
}

type jobResponse struct {
	LastBuild jobBuild   `json:"lastBuild"`
	Jobs      []innerJob `json:"jobs"`
	Name      string     `json:"name"`
}

type innerJob struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Color string `json:"color"`
}

type jobBuild struct {
	Number int64
	URL    string
}

type buildResponse struct {
	Building  bool   `json:"building"`
	Duration  int64  `json:"duration"`
	Number    int64  `json:"number"`
	Result    string `json:"result"`
	Timestamp int64  `json:"timestamp"`
}

func (b *buildResponse) GetTimestamp() time.Time {
	return time.Unix(0, b.Timestamp*int64(time.Millisecond))
}

const (
	nodePath = "/computer/api/json"
	jobPath  = "/api/json"
)

type jobRequest struct {
	name    string
	parents []string
	layer   int
}

func (jr jobRequest) combined() []string {
	path := make([]string, len(jr.parents))
	copy(path, jr.parents)
	return append(path, jr.name)
}

func (jr jobRequest) combinedEscaped() []string {
	jobs := jr.combined()
	for index, job := range jobs {
		jobs[index] = url.PathEscape(job)
	}
	return jobs
}

func (jr jobRequest) URL() string {
	return "/job/" + strings.Join(jr.combinedEscaped(), "/job/") + jobPath
}

func (jr jobRequest) buildURL(number int64) string {
	return "/job/" + strings.Join(jr.combinedEscaped(), "/job/") + "/" + strconv.Itoa(int(number)) + jobPath
}

func (jr jobRequest) hierarchyName() string {
	return strings.Join(jr.combined(), "/")
}

func (jr jobRequest) parentsString() string {
	return strings.Join(jr.parents, "/")
}

func (ins *Instance) gatherJobBuild(jr jobRequest, b *buildResponse, slist *types.SampleList) {
	tags := map[string]string{"name": jr.name, "parents": jr.parentsString(), "result": b.Result, "source": ins.Source, "port": ins.Port}
	fields := make(map[string]interface{})
	fields[measurementJob+"duration"] = b.Duration
	fields[measurementJob+"result_code"] = mapResultCode(b.Result)
	fields[measurementJob+"number"] = b.Number
	slist.PushSamples(inputName, fields, tags)
}

// perform status mapping
func mapResultCode(s string) int {
	switch strings.ToLower(s) {
	case "success":
		return 0
	case "failure":
		return 1
	case "not_built":
		return 2
	case "unstable":
		return 3
	case "aborted":
		return 4
	}
	return -1
}

// ////////////////////////////////////////////////////

type client struct {
	baseURL       string
	httpClient    *http.Client
	username      string
	password      string
	sessionCookie *http.Cookie
	semaphore     chan struct{}
}

func newClient(httpClient *http.Client, url, username, password string, maxConnections int) *client {
	return &client{
		baseURL:    url,
		httpClient: httpClient,
		username:   username,
		password:   password,
		semaphore:  make(chan struct{}, maxConnections),
	}
}

func (c *client) init() error {
	// get session cookie
	req, err := http.NewRequest("GET", c.baseURL, nil)
	if err != nil {
		return err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for _, cc := range resp.Cookies() {
		if strings.Contains(cc.Name, "JSESSIONID") {
			c.sessionCookie = cc
			break
		}
	}

	// first api fetch
	return c.doGet(context.Background(), jobPath, new(jobResponse))
}

func (c *client) doGet(ctx context.Context, url string, v interface{}) error {
	req, err := createGetRequest(c.baseURL+url, c.username, c.password, c.sessionCookie)
	if err != nil {
		return err
	}
	select {
	case c.semaphore <- struct{}{}:
		break
	case <-ctx.Done():
		return ctx.Err()
	}
	resp, err := c.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		<-c.semaphore
		return err
	}
	defer func() {
		// Ignore the returned error as we cannot do anything about it anyway
		//nolint:errcheck,revive
		resp.Body.Close()
		<-c.semaphore
	}()
	// Clear invalid token if unauthorized
	if resp.StatusCode == http.StatusUnauthorized {
		c.sessionCookie = nil
		return APIError{
			URL:        url,
			StatusCode: resp.StatusCode,
			Title:      resp.Status,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{
			URL:        url,
			StatusCode: resp.StatusCode,
			Title:      resp.Status,
		}
	}
	if resp.StatusCode == http.StatusNoContent {
		return APIError{
			URL:        url,
			StatusCode: resp.StatusCode,
			Title:      resp.Status,
		}
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

type APIError struct {
	URL         string
	StatusCode  int
	Title       string
	Description string
}

func (e APIError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("[%s] %s: %s", e.URL, e.Title, e.Description)
	}
	return fmt.Sprintf("[%s] %s", e.URL, e.Title)
}

func createGetRequest(url string, username, password string, sessionCookie *http.Cookie) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	req.Header.Add("Accept", "application/json")
	return req, nil
}

func (c *client) getJobs(ctx context.Context, jr *jobRequest) (js *jobResponse, err error) {
	js = new(jobResponse)
	url := jobPath
	if jr != nil {
		url = jr.URL()
	}
	err = c.doGet(ctx, url, js)
	return js, err
}

func (c *client) getBuild(ctx context.Context, jr jobRequest, number int64) (b *buildResponse, err error) {
	b = new(buildResponse)
	url := jr.buildURL(number)
	err = c.doGet(ctx, url, b)
	return b, err
}

func (c *client) getAllNodes(ctx context.Context) (nodeResp *nodeResponse, err error) {
	nodeResp = new(nodeResponse)
	err = c.doGet(ctx, nodePath, nodeResp)
	return nodeResp, err
}
