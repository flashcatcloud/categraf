package tomcat

import (
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/logs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "tomcat"

type TomcatStatus struct {
	TomcatJvm        TomcatJvm         `xml:"jvm"`
	TomcatConnectors []TomcatConnector `xml:"connector"`
}

type TomcatJvm struct {
	JvmMemory      JvmMemoryStat       `xml:"memory"`
	JvmMemoryPools []JvmMemoryPoolStat `xml:"memorypool"`
}

type JvmMemoryStat struct {
	Free  int64 `xml:"free,attr"`
	Total int64 `xml:"total,attr"`
	Max   int64 `xml:"max,attr"`
}

type JvmMemoryPoolStat struct {
	Name           string `xml:"name,attr"`
	Type           string `xml:"type,attr"`
	UsageInit      int64  `xml:"usageInit,attr"`
	UsageCommitted int64  `xml:"usageCommitted,attr"`
	UsageMax       int64  `xml:"usageMax,attr"`
	UsageUsed      int64  `xml:"usageUsed,attr"`
}

type TomcatConnector struct {
	Name        string      `xml:"name,attr"`
	ThreadInfo  ThreadInfo  `xml:"threadInfo"`
	RequestInfo RequestInfo `xml:"requestInfo"`
}

type ThreadInfo struct {
	MaxThreads         int64 `xml:"maxThreads,attr"`
	CurrentThreadCount int64 `xml:"currentThreadCount,attr"`
	CurrentThreadsBusy int64 `xml:"currentThreadsBusy,attr"`
}
type RequestInfo struct {
	MaxTime        int   `xml:"maxTime,attr"`
	ProcessingTime int   `xml:"processingTime,attr"`
	RequestCount   int   `xml:"requestCount,attr"`
	ErrorCount     int   `xml:"errorCount,attr"`
	BytesReceived  int64 `xml:"bytesReceived,attr"`
	BytesSent      int64 `xml:"bytesSent,attr"`
}

type Instance struct {
	URL           string            `toml:"url"`
	Username      string            `toml:"username"`
	Password      string            `toml:"password"`
	Timeout       config.Duration   `toml:"timeout"`
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`

	tls.ClientConfig
	client  *http.Client
	request *http.Request
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return errors.New("url is blank")
	}

	if ins.Timeout <= 0 {
		ins.Timeout = config.Duration(time.Second * 3)
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return err
	}

	ins.client = client

	_, err = url.Parse(ins.URL)
	if err != nil {
		return err
	}

	request, err := http.NewRequest("GET", ins.URL, nil)
	if err != nil {
		return err
	}

	if ins.Username != "" && ins.Password != "" {
		request.SetBasicAuth(ins.Username, ins.Password)
	}

	ins.request = request

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	trans := &http.Transport{}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return nil, err
		}
		trans.TLSClientConfig = tlsConfig
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.Timeout),
	}

	return client, nil
}

type Tomcat struct {
	config.Interval
	Instances []*Instance `toml:"instances"`
	logs.Logs

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Tomcat{}
	})
}

func (t *Tomcat) Prefix() string {
	return inputName
}

func (t *Tomcat) Init() error {
	if len(t.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(t.Instances); i++ {
		if err := t.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tomcat) Drop() {}

func (t *Tomcat) Gather(slist *list.SafeList) {
	atomic.AddUint64(&t.Counter, 1)
	for i := range t.Instances {
		ins := t.Instances[i]
		t.wg.Add(1)
		go t.gatherOnce(slist, ins)
	}
	t.wg.Wait()
}

func (t *Tomcat) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer t.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&t.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"url": ins.URL}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	// url cannot connect? up = 0
	resp, err := ins.client.Do(ins.request)
	if err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to query tomcat url:", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! received HTTP status code:", resp.StatusCode, "expected: 200")
		return
	}

	defer resp.Body.Close()

	var status TomcatStatus
	if err := xml.NewDecoder(resp.Body).Decode(&status); err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to decode response body:", err)
		return
	}

	slist.PushFront(inputs.NewSample("up", 1, tags))

	slist.PushFront(inputs.NewSample("jvm_memory_free", status.TomcatJvm.JvmMemory.Free, tags))
	slist.PushFront(inputs.NewSample("jvm_memory_total", status.TomcatJvm.JvmMemory.Total, tags))
	slist.PushFront(inputs.NewSample("jvm_memory_max", status.TomcatJvm.JvmMemory.Max, tags))

	// add tomcat_jvm_memorypool measurements
	for _, mp := range status.TomcatJvm.JvmMemoryPools {
		tcmpTags := map[string]string{
			"name": mp.Name,
			"type": mp.Type,
		}

		tcmpFields := map[string]interface{}{
			"jvm_memorypool_init":      mp.UsageInit,
			"jvm_memorypool_committed": mp.UsageCommitted,
			"jvm_memorypool_max":       mp.UsageMax,
			"jvm_memorypool_used":      mp.UsageUsed,
		}

		inputs.PushSamples(slist, tcmpFields, tags, tcmpTags)
	}

	// add tomcat_connector measurements
	for _, c := range status.TomcatConnectors {
		name, err := strconv.Unquote(c.Name)
		if err != nil {
			name = c.Name
		}

		tccTags := map[string]string{
			"name": name,
		}

		tccFields := map[string]interface{}{
			"connector_max_threads":          c.ThreadInfo.MaxThreads,
			"connector_current_thread_count": c.ThreadInfo.CurrentThreadCount,
			"connector_current_threads_busy": c.ThreadInfo.CurrentThreadsBusy,
			"connector_max_time":             c.RequestInfo.MaxTime,
			"connector_processing_time":      c.RequestInfo.ProcessingTime,
			"connector_request_count":        c.RequestInfo.RequestCount,
			"connector_error_count":          c.RequestInfo.ErrorCount,
			"connector_bytes_received":       c.RequestInfo.BytesReceived,
			"connector_bytes_sent":           c.RequestInfo.BytesSent,
		}

		inputs.PushSamples(slist, tccFields, tags, tccTags)
	}
}
