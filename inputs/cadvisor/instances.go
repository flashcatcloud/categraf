package cadvisor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cache"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/kubernetes"
	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const (
	acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,*/*;q=0.1`
	cadvisorPath = "/metrics/cadvisor"
)

const (
	ContainerType CadvisorType = "cadvisor"
	NodeType      CadvisorType = "kubelet"
)

type (
	CadvisorType string
	Instance     struct {
		config.InstanceConfig

		URL  string       `toml:"url"`
		Type CadvisorType `toml:"type"`
		u    *url.URL

		NamePrefix        string          `toml:"name_prefix"`
		BearerTokenString string          `toml:"bearer_token_string"`
		BearerTokeFile    string          `toml:"bearer_token_file"`
		Username          string          `toml:"username"`
		Password          string          `toml:"password"`
		Timeout           config.Duration `toml:"timeout"`
		IgnoreMetrics     []string        `toml:"ignore_metrics"`
		IgnoreLabelKeys   []string        `toml:"ignore_label_keys"`
		Headers           []string        `toml:"headers"`

		ChooseLabelKeys []string `toml:"choose_label_keys"`

		config.UrlLabel

		ignoreMetricsFilter   filter.Filter
		ignoreLabelKeysFilter filter.Filter
		chooseLabelKeysFilter filter.Filter
		tls.ClientConfig
		client *http.Client

		*cache.BasicCache[string]
		stop chan struct{}
	}
)

func (ins *Instance) Empty() bool {
	if len(ins.URL) > 0 {
		return false
	}

	return true
}

func (ins *Instance) Init() error {
	if ins.Empty() {
		return types.ErrInstancesEmpty
	}

	ins.URL = config.Expand(ins.URL)
	u, err := url.Parse(ins.URL)
	if err != nil {
		return fmt.Errorf("failed to parse scrape url: %s, error: %s", ins.URL, err)
	}
	ins.u = u
	if ins.u.Path == "" {
		ins.u.Path = cadvisorPath
		ins.Type = NodeType
	}

	ins.stop = make(chan struct{})
	ins.BasicCache = cache.NewBasicCache[string]()
	go ins.cache()

	if ins.Timeout <= 0 {
		ins.Timeout = config.Duration(time.Second * 3)
	}
	client, err := ins.createHTTPClient()
	if err != nil {
		return err
	} else {
		ins.client = client
	}

	if len(ins.IgnoreMetrics) > 0 {
		ins.ignoreMetricsFilter, err = filter.Compile(ins.IgnoreMetrics)
		if err != nil {
			return err
		}
	}

	if len(ins.IgnoreLabelKeys) > 0 {
		ins.ignoreLabelKeysFilter, err = filter.Compile(ins.IgnoreLabelKeys)
		if err != nil {
			return err
		}
	}

	if len(ins.ChooseLabelKeys) > 0 {
		ins.chooseLabelKeysFilter, err = filter.Compile(ins.ChooseLabelKeys)
		if err != nil {
			return err
		}
	}

	if err := ins.PrepareUrlTemplate(); err != nil {
		return err
	}

	return nil
}

func (ins *Instance) cache() {
	if ins.Type == ContainerType {
		return
	}

	podUrl := *ins.u
	podUrl.Path = "/pods"
	req, err := http.NewRequest("GET", podUrl.String(), nil)
	if err != nil {
		log.Println("E! failed to new request for url:", podUrl.String(), "error:", err)
		return
	}
	ins.setHeaders(req)
	timer := time.NewTimer(0 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			resp, err := ins.client.Do(req)
			if err != nil {
				log.Println("E! failed to request for url:", podUrl.String(), "error:", err)
				continue
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Println("E! failed to read body for url:", podUrl.String(), "error:", err)
				continue
			}
			resp.Body.Close()
			pods := kubernetes.PodList{}
			err = json.Unmarshal(body, &pods)
			if err != nil {
				log.Println("E! unmarshal pods info", err)
				continue
			}
			for _, pod := range pods.Items {
				ins.BasicCache.Add(cacheKey(pod.Metadata.Namespace, pod.Metadata.Name), pod)
			}
			timer.Reset(1 * time.Minute)
		case <-ins.stop:
			return
		}
	}
}

func cacheKey(ns, pod string) string {
	return ns + "||" + pod
}

func (ins *Instance) Drop() {
	log.Println("I! cadvisor instance stop")
	close(ins.stop)
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

func (ins *Instance) Gather(slist *types.SampleList) {

	req, err := http.NewRequest("GET", ins.u.String(), nil)
	if err != nil {
		log.Println("E! failed to new request for url:", ins.u.String(), "error:", err)
		return
	}

	ins.setHeaders(req)

	labels, err := ins.GenerateLabel(ins.u)
	if err != nil {
		log.Println("E! failed to generate url label value:", err)
		return
	}

	res, err := ins.client.Do(req)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to query url:", ins.u.String(), "error:", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to query url:", ins.u.String(), "status code:", res.StatusCode)
		return
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		slist.PushFront(types.NewSample("", "up", 0, labels))
		log.Println("E! failed to read response body, url:", ins.u.String(), "error:", err)
		return
	}

	slist.PushFront(types.NewSample("", "up", 1, labels))

	ins.gather(body, res.Header, labels, slist)

}

func (ins *Instance) gather(buf []byte, header http.Header, defaultLabels map[string]string, slist *types.SampleList) {
	metricFamilies, err := util.Parse(buf, header)
	if err != nil {
		log.Println("E! failed to parse metrics, url:", ins.u.String(), "error:", err)
		return
	}

	for metricName, mf := range metricFamilies {
		if ins.ignoreMetricsFilter != nil && ins.ignoreMetricsFilter.Match(metricName) {
			continue
		}
		for _, m := range mf.Metric {
			// reading tags
			tags := ins.makeLabels(m, defaultLabels)

			if mf.GetType() == dto.MetricType_SUMMARY {
				util.HandleSummary(ins.NamePrefix, m, tags, metricName, nil, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				util.HandleHistogram(ins.NamePrefix, m, tags, metricName, nil, slist)
			} else {
				util.HandleGaugeCounter(ins.NamePrefix, m, tags, metricName, nil, slist)
			}
		}
	}
}

func (ins *Instance) ignoreLabel(label string) bool {
	if ins.chooseLabelKeysFilter != nil {
		if ins.chooseLabelKeysFilter.Match(label) {
			return false
		} else {
			return true
		}
	}

	if ins.ignoreLabelKeysFilter != nil && ins.ignoreLabelKeysFilter.Match(label) {
		return true
	}

	return false
}

func (ins *Instance) makeLabels(m *dto.Metric, defaultLabels map[string]string) map[string]string {
	var (
		podName, namespace string
		result             = map[string]string{}
	)

	for _, label := range m.Label {
		if ins.ignoreLabel(label.GetName()) {
			continue
		}
		result[label.GetName()] = label.GetValue()

		if ins.Type == NodeType {
			if label.GetName() != "pod" && label.GetName() != "namespace" {
				continue
			}
			if label.GetName() == "pod" {
				podName = label.GetValue()
			}
			if label.GetName() == "namespace" {
				namespace = label.GetValue()
			}
			if strings.TrimSpace(podName) == "" || strings.TrimSpace(namespace) == "" {
				continue
			}

			if val, ok := ins.BasicCache.Get(cacheKey(namespace, podName)); ok {
				if pod, ok := val.(*kubernetes.Pod); ok {
					for k, v := range pod.Metadata.Labels {
						if ins.ignoreLabel(k) {
							continue
						}
						result[k] = v
					}

					for k, v := range pod.Metadata.Annotations {
						if ins.ignoreLabel(k) {
							continue
						}
						result[k] = v
					}
				}
			} else {
				if ins.DebugMod {
					log.Println(cacheKey(namespace, podName), "not in cache")
				}
			}
		}
	}

	for key, value := range defaultLabels {
		result[key] = value
	}

	return result
}

func (ins *Instance) setHeaders(req *http.Request) {
	if ins.Username != "" && ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	if ins.BearerTokeFile != "" {
		content, err := os.ReadFile(ins.BearerTokeFile)
		if err != nil {
			log.Println("E! failed to read bearer token file:", ins.BearerTokeFile, "error:", err)
			return
		}

		ins.BearerTokenString = strings.TrimSpace(string(content))
	}

	if ins.BearerTokenString != "" {
		req.Header.Set("Authorization", "Bearer "+ins.BearerTokenString)
	}

	req.Header.Set("Accept", acceptHeader)

	for i := 0; i < len(ins.Headers); i += 2 {
		req.Header.Set(ins.Headers[i], ins.Headers[i+1])
	}
}
