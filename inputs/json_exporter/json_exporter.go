package json_exporter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/httpx"
	"flashcat.cloud/categraf/types"
	"k8s.io/client-go/util/jsonpath"
)

const inputName = "json_exporter"

type ScrapeType string

const (
	ValueScrape  ScrapeType = "value"
	ObjectScrape ScrapeType = "object"
)

type Metric struct {
	Name            string            `toml:"name"`
	Path            string            `toml:"path"`
	Labels          map[string]string `toml:"labels"`
	Type            ScrapeType        `toml:"type"`
	EpochTimestamp  string            `toml:"epoch_timestamp"`
	Values          map[string]string `toml:"values"`
	AllowMissingKey bool              `toml:"allow_missing_key"`
}

type Instance struct {
	config.InstanceConfig
	config.HTTPCommonConfig
	config.UrlLabel

	Targets          []string `toml:"targets"`
	ValidStatusCodes []int    `toml:"valid_status_codes"`
	Metrics          []Metric `toml:"metrics"`

	client           httpClient
	validStatusCodes map[int]struct{}
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type JSONExporter struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &JSONExporter{}
	})
}

func (j *JSONExporter) Clone() inputs.Input {
	return &JSONExporter{}
}

func (j *JSONExporter) Name() string {
	return inputName
}

func (j *JSONExporter) GetInstances() []inputs.Instance {
	instances := make([]inputs.Instance, len(j.Instances))
	for i := range j.Instances {
		instances[i] = j.Instances[i]
	}
	return instances
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}
	if len(ins.Metrics) == 0 {
		return errors.New("json_exporter metrics are required")
	}

	for i := range ins.Targets {
		ins.Targets[i] = expandTarget(ins.Targets[i])
		parsed, err := url.Parse(ins.Targets[i])
		if err != nil {
			return fmt.Errorf("invalid target %q: %w", ins.Targets[i], err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("target %q must use http or https", ins.Targets[i])
		}
	}

	for i := range ins.Metrics {
		if err := validateMetric(ins.Metrics[i]); err != nil {
			return fmt.Errorf("invalid metric at index %d: %w", i, err)
		}
	}

	ins.InitHTTPClientConfig()
	if ins.Headers == nil {
		ins.Headers = make(map[string]string)
	}
	if _, ok := ins.Headers["Accept"]; !ok {
		ins.Headers["Accept"] = "application/json"
	}

	if err := ins.PrepareUrlTemplate(); err != nil {
		return fmt.Errorf("prepare URL label template: %w", err)
	}

	ins.validStatusCodes = make(map[int]struct{}, len(ins.ValidStatusCodes))
	for _, statusCode := range ins.ValidStatusCodes {
		if statusCode < 100 || statusCode > 599 {
			return fmt.Errorf("invalid HTTP status code %d", statusCode)
		}
		ins.validStatusCodes[statusCode] = struct{}{}
	}

	client, err := ins.createHTTPClient()
	if err != nil {
		return fmt.Errorf("create HTTP client: %w", err)
	}
	ins.client = client
	return nil
}

func expandTarget(target string) string {
	if config.Config != nil {
		return config.Expand(target)
	}
	return os.ExpandEnv(target)
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsConfig, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}
	proxy, err := ins.Proxy()
	if err != nil {
		return nil, err
	}

	return httpx.CreateHTTPClient(
		httpx.TlsConfig(tlsConfig),
		httpx.Proxy(proxy),
		httpx.Timeout(time.Duration(ins.Timeout)),
		httpx.DisableKeepAlives(*ins.DisableKeepAlives),
		httpx.FollowRedirects(*ins.FollowRedirects),
	), nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup
	for _, target := range ins.Targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ins.gatherTarget(slist, target)
		}(target)
	}
	wg.Wait()
}

func (ins *Instance) gatherTarget(slist *types.SampleList, target string) {
	startedAt := time.Now()
	labels := ins.targetLabels(target)
	defer func() {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(startedAt).Seconds(), labels)
	}()

	data, err := ins.fetch(target)
	if err != nil {
		slist.PushSample(inputName, "up", float64(0), labels)
		log.Printf("E! json_exporter failed to scrape target %s: %v", target, err)
		return
	}
	if !json.Valid(data) {
		slist.PushSample(inputName, "up", float64(0), labels)
		log.Printf("E! json_exporter target %s returned invalid JSON", target)
		return
	}

	slist.PushSample(inputName, "up", float64(1), labels)
	for _, metric := range ins.Metrics {
		if err := collectMetric(slist, data, metric, labels); err != nil {
			log.Printf("E! json_exporter failed to collect metric %s from %s: %v", metric.Name, target, err)
		}
	}
}

func (ins *Instance) targetLabels(target string) map[string]string {
	parsed, err := url.Parse(target)
	if err != nil {
		return map[string]string{"target": target}
	}
	labels, err := ins.GenerateLabel(parsed)
	if err != nil {
		log.Printf("E! json_exporter failed to generate labels for %s: %v", target, err)
		return map[string]string{"target": target}
	}
	return labels
}

func (ins *Instance) fetch(target string) ([]byte, error) {
	var body io.Reader
	if ins.Body != "" {
		body = strings.NewReader(ins.Body)
	}
	req, err := http.NewRequest(ins.Method, target, body)
	if err != nil {
		return nil, err
	}
	ins.SetHeaders(req)

	resp, err := ins.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if !ins.acceptStatus(resp.StatusCode) {
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}

func (ins *Instance) acceptStatus(statusCode int) bool {
	if len(ins.validStatusCodes) == 0 {
		return statusCode >= 200 && statusCode < 300
	}
	_, ok := ins.validStatusCodes[statusCode]
	return ok
}

func validateMetric(metric Metric) error {
	if metric.Name == "" {
		return errors.New("name is required")
	}
	if metric.Path == "" {
		return fmt.Errorf("path is required for %q", metric.Name)
	}
	if metric.Type == "" {
		metric.Type = ValueScrape
	}
	if metric.Type != ValueScrape && metric.Type != ObjectScrape {
		return fmt.Errorf("unknown type %q for %q", metric.Type, metric.Name)
	}
	if metric.Type == ObjectScrape && len(metric.Values) == 0 {
		return fmt.Errorf("values are required for object metric %q", metric.Name)
	}

	paths := []string{metric.Path}
	for _, path := range metric.Labels {
		paths = append(paths, path)
	}
	if metric.EpochTimestamp != "" {
		paths = append(paths, metric.EpochTimestamp)
	}
	for _, path := range metric.Values {
		paths = append(paths, path)
	}
	for _, path := range paths {
		if err := jsonpath.New("validation").Parse(path); err != nil {
			return fmt.Errorf("invalid JSONPath %q for %q: %w", path, metric.Name, err)
		}
	}
	return nil
}

func collectMetric(slist *types.SampleList, data []byte, metric Metric, baseLabels map[string]string) error {
	metricType := metric.Type
	if metricType == "" {
		metricType = ValueScrape
	}

	switch metricType {
	case ValueScrape:
		return collectValueMetric(slist, data, metric.Name, metric.Path, metric.EpochTimestamp, metric.Labels, metric.AllowMissingKey, baseLabels)
	case ObjectScrape:
		objects, missing, err := extractValue(data, metric.Path, true, metric.AllowMissingKey)
		if err != nil || missing {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(objects), &items); err != nil {
			return fmt.Errorf("decode objects selected by %q: %w", metric.Path, err)
		}
		for _, item := range items {
			for suffix, valuePath := range metric.Values {
				name := metric.Name + "_" + suffix
				if err := collectValueMetric(slist, item, name, valuePath, metric.EpochTimestamp, metric.Labels, metric.AllowMissingKey, baseLabels); err != nil {
					log.Printf("E! json_exporter failed to collect object metric %s: %v", name, err)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown metric type %q", metric.Type)
	}
}

func collectValueMetric(slist *types.SampleList, data []byte, name, valuePath, timestampPath string, labelPaths map[string]string, allowMissingKey bool, baseLabels map[string]string) error {
	value, missing, err := extractValue(data, valuePath, false, allowMissingKey)
	if err != nil || missing {
		return err
	}
	floatValue, err := sanitizeValue(value)
	if err != nil {
		return fmt.Errorf("convert value %q selected by %q: %w", value, valuePath, err)
	}

	labels := make(map[string]string, len(labelPaths))
	for name, path := range labelPaths {
		labelValue, missing, err := extractValue(data, path, false, allowMissingKey)
		if err != nil {
			return fmt.Errorf("extract label %q: %w", name, err)
		}
		if missing {
			labelValue = ""
		}
		labels[name] = labelValue
	}

	sample := types.NewSample(inputName, name, floatValue, baseLabels, labels)
	if timestampPath != "" {
		timestamp, missing, err := extractValue(data, timestampPath, false, allowMissingKey)
		if err != nil {
			return fmt.Errorf("extract timestamp: %w", err)
		}
		if !missing {
			epochMilliseconds, err := sanitizeIntValue(timestamp)
			if err != nil {
				return fmt.Errorf("convert timestamp %q: %w", timestamp, err)
			}
			sample.SetTime(time.UnixMilli(epochMilliseconds))
		}
	}
	slist.PushFront(sample)
	return nil
}

// extractValue is adapted from prometheus-community/json_exporter. The source
// project is licensed under Apache-2.0 and uses Kubernetes JSONPath syntax.
func extractValue(data []byte, path string, enableJSONOutput, allowMissingKey bool) (string, bool, error) {
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return "", false, err
	}

	parser := jsonpath.New("json_exporter")
	parser.EnableJSONOutput(enableJSONOutput)
	parser.AllowMissingKeys(allowMissingKey)
	if err := parser.Parse(path); err != nil {
		return "", false, err
	}

	var output bytes.Buffer
	if err := parser.Execute(&output, jsonData); err != nil {
		return "", false, err
	}
	if output.Len() == 0 && allowMissingKey {
		return "", true, nil
	}
	if result, err := jsonpath.UnquoteExtend(output.String()); err == nil {
		return result, false, nil
	}
	return output.String(), false, nil
}
