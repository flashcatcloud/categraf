package logstash

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/pkg/jsonx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "logstash"

type Logstash struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Logstash{}
	})
}
func (l *Logstash) Clone() inputs.Input {
	return &Logstash{}
}

func (l *Logstash) Name() string {
	return inputName
}

func (l *Logstash) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(l.Instances))
	for i := 0; i < len(l.Instances); i++ {
		ret[i] = l.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URL            string            `toml:"url"`
	SinglePipeline bool              `toml:"single_pipeline"`
	Collect        []string          `toml:"collect"`
	Username       string            `toml:"username"`
	Password       string            `toml:"password"`
	Headers        map[string]string `toml:"headers"`
	Timeout        config.Duration   `toml:"timeout"`

	tls.ClientConfig
	client *http.Client
}

type ProcessStats struct {
	ID      string      `json:"id"`
	Process interface{} `json:"process"`
	Name    string      `json:"name"`
	Host    string      `json:"host"`
	Version string      `json:"version"`
}

type JVMStats struct {
	ID      string      `json:"id"`
	JVM     interface{} `json:"jvm"`
	Name    string      `json:"name"`
	Host    string      `json:"host"`
	Version string      `json:"version"`
}

type PipelinesStats struct {
	ID        string              `json:"id"`
	Pipelines map[string]Pipeline `json:"pipelines"`
	Name      string              `json:"name"`
	Host      string              `json:"host"`
	Version   string              `json:"version"`
}

type PipelineStats struct {
	ID       string   `json:"id"`
	Pipeline Pipeline `json:"pipeline"`
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Version  string   `json:"version"`
}

type Pipeline struct {
	Events  interface{}     `json:"events"`
	Plugins PipelinePlugins `json:"plugins"`
	Reloads interface{}     `json:"reloads"`
	Queue   PipelineQueue   `json:"queue"`
}

type Plugin struct {
	ID           string                 `json:"id"`
	Events       interface{}            `json:"events"`
	Name         string                 `json:"name"`
	BulkRequests map[string]interface{} `json:"bulk_requests"`
	Documents    map[string]interface{} `json:"documents"`
}

type PipelinePlugins struct {
	Inputs  []Plugin `json:"inputs"`
	Filters []Plugin `json:"filters"`
	Outputs []Plugin `json:"outputs"`
}

type PipelineQueue struct {
	Events              float64     `json:"events"`
	EventsCount         *float64    `json:"events_count"`
	Type                string      `json:"type"`
	Capacity            interface{} `json:"capacity"`
	Data                interface{} `json:"data"`
	QueueSizeInBytes    *float64    `json:"queue_size_in_bytes"`
	MaxQueueSizeInBytes *float64    `json:"max_queue_size_in_bytes"`
}

const jvmStats = "/_node/stats/jvm"
const processStats = "/_node/stats/process"
const pipelinesStats = "/_node/stats/pipelines"
const pipelineStats = "/_node/stats/pipeline"

func (ins *Instance) Init() error {
	if len(ins.URL) == 0 {
		return types.ErrInstancesEmpty
	}
	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(5 * time.Second)
	}
	if len(ins.Collect) == 0 {
		ins.Collect = []string{"pipelines", "process", "jvm"}
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return err
	}

	ins.client = client
	err = choice.CheckSlice(ins.Collect, []string{"pipelines", "process", "jvm"})
	if err != nil {
		return fmt.Errorf(`cannot verify "collect" setting: %v`, err)
	}
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if choice.Contains("jvm", ins.Collect) {
		jvmURL, err := url.Parse(ins.URL + jvmStats)
		if err != nil {
			log.Println("E! failed to parse url:", ins.URL+jvmStats)
			return
		}
		if err := ins.gatherJVMStats(jvmURL.String(), slist); err != nil {
			log.Println("E! failed to gather jvm stats:", err)
			return
		}
	}

	if choice.Contains("process", ins.Collect) {
		processURL, err := url.Parse(ins.URL + processStats)
		if err != nil {
			log.Println("E! failed to parse url:", ins.URL+processStats)
			return
		}
		if err := ins.gatherProcessStats(processURL.String(), slist); err != nil {
			log.Println("E! failed to gather process stats:", err)
			return
		}
	}

	if choice.Contains("pipelines", ins.Collect) {
		if ins.SinglePipeline {
			pipelineURL, err := url.Parse(ins.URL + pipelineStats)
			if err != nil {
				log.Println("E! failed to parse url:", ins.URL+pipelineStats)
				return
			}
			if err := ins.gatherPipelineStats(pipelineURL.String(), slist); err != nil {
				log.Println("E! failed to gather pipeline stats:", err)
				return
			}
		} else {
			pipelinesURL, err := url.Parse(ins.URL + pipelinesStats)
			if err != nil {
				log.Println("E! failed to parse url:", ins.URL+pipelinesStats)
				return
			}
			if err := ins.gatherPipelinesStats(pipelinesURL.String(), slist); err != nil {
				log.Println("E! failed to gather pipelines stats:", err)
				return
			}
		}
	}

}

// createHTTPClient create a clients to access API
func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsConfig, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: time.Duration(ins.Timeout),
	}

	return client, nil
}

// gatherJSONData query the data source and parse the response JSON
func (ins *Instance) gatherJSONData(address string, value interface{}) error {
	request, err := http.NewRequest("GET", address, nil)
	if err != nil {
		return err
	}

	if (ins.Username != "") || (ins.Password != "") {
		request.SetBasicAuth(ins.Username, ins.Password)
	}

	for header, value := range ins.Headers {
		if strings.ToLower(header) == "host" {
			request.Host = value
		} else {
			request.Header.Add(header, value)
		}
	}

	response, err := ins.client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// ignore the err here; LimitReader returns io.EOF and we're not interested in read errors.
		body, _ := io.ReadAll(io.LimitReader(response.Body, 200))
		return fmt.Errorf("%s returned HTTP status %s: %q", address, response.Status, body)
	}

	err = json.NewDecoder(response.Body).Decode(value)
	if err != nil {
		return err
	}

	return nil
}

// gatherJVMStats gather the JVM metrics and add results to list
func (ins *Instance) gatherJVMStats(address string, slist *types.SampleList) error {
	jvmStats := &JVMStats{}

	err := ins.gatherJSONData(address, jvmStats)
	if err != nil {
		return err
	}

	tags := map[string]string{
		"node_id":      jvmStats.ID,
		"node_name":    jvmStats.Name,
		"node_version": jvmStats.Version,
		"source":       jvmStats.Host,
	}

	jsonParser := jsonx.JSONFlattener{}
	err = jsonParser.FlattenJSON("", jvmStats.JVM)
	if err != nil {
		return err
	}
	for key, val := range jsonParser.Fields {
		slist.PushSample(inputName, "jvm_"+key, val, tags)
	}

	return nil
}

// gatherJVMStats gather the Process metrics and add results to list
func (ins *Instance) gatherProcessStats(address string, slist *types.SampleList) error {
	processStats := &ProcessStats{}

	err := ins.gatherJSONData(address, processStats)
	if err != nil {
		return err
	}

	tags := map[string]string{
		"node_id":      processStats.ID,
		"node_name":    processStats.Name,
		"node_version": processStats.Version,
		"source":       processStats.Host,
	}
	jsonParser := jsonx.JSONFlattener{}
	err = jsonParser.FlattenJSON("", processStats.Process)
	if err != nil {
		return err
	}

	for key, val := range jsonParser.Fields {
		slist.PushSample(inputName, "process_"+key, val, tags)
	}
	return nil
}

// gatherJVMStats gather the Pipeline metrics and add results to list (for Logstash < 6)
func (ins *Instance) gatherPipelineStats(address string, slist *types.SampleList) error {
	pipelineStats := &PipelineStats{}

	err := ins.gatherJSONData(address, pipelineStats)
	if err != nil {
		return err
	}

	tags := map[string]string{
		"node_id":      pipelineStats.ID,
		"node_name":    pipelineStats.Name,
		"node_version": pipelineStats.Version,
		"source":       pipelineStats.Host,
	}

	jsonParser := jsonx.JSONFlattener{}
	err = jsonParser.FlattenJSON("", pipelineStats.Pipeline.Events)
	if err != nil {
		return err
	}
	for key, val := range jsonParser.Fields {
		slist.PushSample(inputName, "events_"+key, val, tags)
	}

	err = ins.gatherPluginsStats(pipelineStats.Pipeline.Plugins.Inputs, "input", tags, slist)
	if err != nil {
		return err
	}
	err = ins.gatherPluginsStats(pipelineStats.Pipeline.Plugins.Filters, "filter", tags, slist)
	if err != nil {
		return err
	}
	err = ins.gatherPluginsStats(pipelineStats.Pipeline.Plugins.Outputs, "output", tags, slist)
	if err != nil {
		return err
	}

	err = ins.gatherQueueStats(&pipelineStats.Pipeline.Queue, tags, slist)
	if err != nil {
		return err
	}

	return nil
}

func (ins *Instance) gatherQueueStats(
	queue *PipelineQueue,
	tags map[string]string,
	slist *types.SampleList,
) error {
	queueTags := map[string]string{
		"queue_type": queue.Type,
	}
	for tag, value := range tags {
		queueTags[tag] = value
	}

	events := queue.Events
	if queue.EventsCount != nil {
		events = *queue.EventsCount
	}

	queueFields := map[string]interface{}{
		"events": events,
	}

	if queue.Type != "memory" {
		jsonParser := jsonx.JSONFlattener{}
		err := jsonParser.FlattenJSON("", queue.Capacity)
		if err != nil {
			return err
		}
		err = jsonParser.FlattenJSON("", queue.Data)
		if err != nil {
			return err
		}
		for field, value := range jsonParser.Fields {
			queueFields[field] = value
		}

		if queue.MaxQueueSizeInBytes != nil {
			queueFields["max_queue_size_in_bytes"] = *queue.MaxQueueSizeInBytes
		}

		if queue.QueueSizeInBytes != nil {
			queueFields["queue_size_in_bytes"] = *queue.QueueSizeInBytes
		}

	}
	for key, val := range queueFields {
		slist.PushSample(inputName, "queue_"+key, val, queueTags)
	}
	return nil
}

// gatherJVMStats gather the Pipelines metrics and add results to list  (for Logstash >= 6)
func (ins *Instance) gatherPipelinesStats(address string, slist *types.SampleList) error {
	pipelinesStats := &PipelinesStats{}

	err := ins.gatherJSONData(address, pipelinesStats)
	if err != nil {
		return err
	}

	for pipelineName, pipeline := range pipelinesStats.Pipelines {
		tags := map[string]string{
			"node_id":      pipelinesStats.ID,
			"node_name":    pipelinesStats.Name,
			"node_version": pipelinesStats.Version,
			"pipeline":     pipelineName,
			"source":       pipelinesStats.Host,
		}

		jsonParser := jsonx.JSONFlattener{}
		err := jsonParser.FlattenJSON("", pipeline.Events)
		if err != nil {
			return err
		}

		for key, val := range jsonParser.Fields {
			slist.PushSample(inputName, "events_"+key, val, tags)
		}

		err = ins.gatherPluginsStats(pipeline.Plugins.Inputs, "input", tags, slist)
		if err != nil {
			return err
		}
		err = ins.gatherPluginsStats(pipeline.Plugins.Filters, "filter", tags, slist)
		if err != nil {
			return err
		}
		err = ins.gatherPluginsStats(pipeline.Plugins.Outputs, "output", tags, slist)
		if err != nil {
			return err
		}

		err = ins.gatherQueueStats(&pipeline.Queue, tags, slist)
		if err != nil {
			return err
		}
	}

	return nil
}

// gatherPluginsStats go through a list of plugins and add their metrics to list
func (ins *Instance) gatherPluginsStats(
	plugins []Plugin,
	pluginType string,
	tags map[string]string,
	slist *types.SampleList,
) error {
	for _, plugin := range plugins {
		pluginTags := map[string]string{
			"plugin_name": plugin.Name,
			"plugin_id":   plugin.ID,
			"plugin_type": pluginType,
		}
		for tag, value := range tags {
			pluginTags[tag] = value
		}
		jsonParser := jsonx.JSONFlattener{}
		err := jsonParser.FlattenJSON("", plugin.Events)
		if err != nil {
			return err
		}
		for key, val := range jsonParser.Fields {
			slist.PushSample(inputName, "plugins_"+key, val, pluginTags)
		}
		/*
			The elasticsearch/opensearch output produces additional stats around
			bulk requests and document writes (that are elasticsearch/opensearch specific).
			Collect those here
		*/
		if pluginType == "output" && (plugin.Name == "elasticsearch" || plugin.Name == "opensearch") {
			/*
				The "bulk_requests" section has details about batch writes
				into Elasticsearch
				  "bulk_requests" : {
					"successes" : 2870,
					"responses" : {
					  "200" : 2870
					},
					"failures": 262,
					"with_errors": 9089
				  },
			*/
			jsonParser := jsonx.JSONFlattener{}
			err := jsonParser.FlattenJSON("", plugin.BulkRequests)
			if err != nil {
				return err
			}
			for k, v := range jsonParser.Fields {
				if strings.HasPrefix(k, "bulk_requests") {
					continue
				}
				newKey := fmt.Sprintf("bulk_requests_%s", k)
				jsonParser.Fields[newKey] = v
				delete(jsonParser.Fields, k)
			}

			for key, val := range jsonParser.Fields {
				slist.PushSample(inputName, "plugins_"+key, val, pluginTags)
			}

			/*
				The "documents" section has counts of individual documents
				written/retried/etc.
				  "documents" : {
					"successes" : 2665549,
					"retryable_failures": 13733
				  }
			*/
			jsonParser = jsonx.JSONFlattener{}
			err = jsonParser.FlattenJSON("", plugin.Documents)
			if err != nil {
				return err
			}
			for k, v := range jsonParser.Fields {
				if strings.HasPrefix(k, "documents") {
					continue
				}
				newKey := fmt.Sprintf("documents_%s", k)
				jsonParser.Fields[newKey] = v
				delete(jsonParser.Fields, k)
			}
			for key, val := range jsonParser.Fields {
				slist.PushSample(inputName, "plugins_"+key, val, pluginTags)
			}
		}
	}

	return nil
}
