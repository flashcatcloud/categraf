package kubernetes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const (
	inputName                 = "kubernetes"
	defaultServiceAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

type Kubernetes struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Kubernetes{}
	})
}

func (k *Kubernetes) Clone() inputs.Input {
	return &Kubernetes{}
}

func (k *Kubernetes) Name() string {
	return inputName
}

func (k *Kubernetes) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(k.Instances))
	for i := 0; i < len(k.Instances); i++ {
		ret[i] = k.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URL string

	// Bearer Token authorization file path
	BearerToken       string `toml:"bearer_token"`
	BearerTokenString string `toml:"bearer_token_string"`

	LabelInclude []string `toml:"label_include"`
	LabelExclude []string `toml:"label_exclude"`

	labelFilter filter.Filter

	GatherSystemContainerMetrics bool `toml:"gather_system_container_metrics"`
	GatherNodeMetrics            bool `toml:"gather_node_metrics"`
	GatherPodContainerMetrics    bool `toml:"gather_pod_container_metrics"`
	GatherPodVolumeMetrics       bool `toml:"gather_pod_volume_metrics"`
	GatherPodNetworkMetrics      bool `toml:"gather_pod_network_metrics"`

	// HTTP Timeout specified as a string - 3s, 1m, 1h
	ResponseTimeout config.Duration

	tls.ClientConfig

	RoundTripper http.RoundTripper
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return types.ErrInstancesEmpty
	}

	ins.URL = os.Expand(ins.URL, config.GetEnv)

	// If neither are provided, use the default service account.
	if ins.BearerToken == "" && ins.BearerTokenString == "" {
		ins.BearerToken = defaultServiceAccountPath
	}

	if ins.BearerToken != "" {
		token, err := os.ReadFile(ins.BearerToken)
		if err != nil {
			return err
		}
		ins.BearerTokenString = strings.TrimSpace(string(token))
	}

	labelFilter, err := filter.NewIncludeExcludeFilter(ins.LabelInclude, ins.LabelExclude)
	if err != nil {
		return err
	}
	ins.labelFilter = labelFilter

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	summaryMetrics := &SummaryMetrics{}
	urlpath := fmt.Sprintf("%s/stats/summary", ins.URL)
	err := ins.LoadJSON(urlpath, summaryMetrics)
	if err != nil {
		log.Println("E! failed to load", urlpath, "error:", err)
		slist.PushSample(inputName, "kubelet_up", 0)
		return
	}

	slist.PushSample(inputName, "kubelet_up", 1)

	podInfos, err := ins.gatherPodInfo(ins.URL)
	if err != nil {
		log.Println("E! failed to gather pod info, error:", err)
		return
	}

	if ins.GatherSystemContainerMetrics {
		ins.buildSystemContainerMetrics(summaryMetrics, slist)
	}

	if ins.GatherNodeMetrics {
		ins.buildNodeMetrics(summaryMetrics, slist)
	}

	ins.buildPodMetrics(summaryMetrics, podInfos, ins.labelFilter, slist)
}

func (ins *Instance) buildPodMetrics(summaryMetrics *SummaryMetrics, podInfo []Metadata, labelFilter filter.Filter, slist *types.SampleList) {
	for _, pod := range summaryMetrics.Pods {
		podLabels := make(map[string]string)
		for _, info := range podInfo {
			if info.Name == pod.PodRef.Name && info.Namespace == pod.PodRef.Namespace {
				for k, v := range info.Labels {
					if labelFilter.Match(k) {
						podLabels[k] = v
					}
				}
			}
		}

		if ins.GatherPodContainerMetrics {
			for _, container := range pod.Containers {
				tags := map[string]string{
					"node":      summaryMetrics.Node.NodeName,
					"namespace": pod.PodRef.Namespace,
					"container": container.Name,
					"pod":       pod.PodRef.Name,
				}
				for k, v := range podLabels {
					tags[k] = v
				}
				fields := make(map[string]interface{})
				fields["pod_container_cpu_usage_nanocores"] = container.CPU.UsageNanoCores
				fields["pod_container_cpu_usage_core_nanoseconds"] = container.CPU.UsageCoreNanoSeconds
				fields["pod_container_memory_usage_bytes"] = container.Memory.UsageBytes
				fields["pod_container_memory_working_set_bytes"] = container.Memory.WorkingSetBytes
				fields["pod_container_memory_rss_bytes"] = container.Memory.RSSBytes
				fields["pod_container_memory_page_faults"] = container.Memory.PageFaults
				fields["pod_container_memory_major_page_faults"] = container.Memory.MajorPageFaults
				fields["pod_container_rootfs_available_bytes"] = container.RootFS.AvailableBytes
				fields["pod_container_rootfs_capacity_bytes"] = container.RootFS.CapacityBytes
				fields["pod_container_rootfs_used_bytes"] = container.RootFS.UsedBytes
				fields["pod_container_logsfs_available_bytes"] = container.LogsFS.AvailableBytes
				fields["pod_container_logsfs_capacity_bytes"] = container.LogsFS.CapacityBytes
				fields["pod_container_logsfs_used_bytes"] = container.LogsFS.UsedBytes
				slist.PushSamples(inputName, fields, tags)
			}
		}

		if ins.GatherPodVolumeMetrics {
			for _, volume := range pod.Volumes {
				tags := map[string]string{
					"node":      summaryMetrics.Node.NodeName,
					"pod":       pod.PodRef.Name,
					"namespace": pod.PodRef.Namespace,
					"volume":    volume.Name,
				}
				for k, v := range podLabels {
					tags[k] = v
				}
				fields := make(map[string]interface{})
				fields["pod_volume_available_bytes"] = volume.AvailableBytes
				fields["pod_volume_capacity_bytes"] = volume.CapacityBytes
				fields["pod_volume_used_bytes"] = volume.UsedBytes
				slist.PushSamples(inputName, fields, tags)
			}
		}

		if ins.GatherPodNetworkMetrics {
			tags := map[string]string{
				"node":      summaryMetrics.Node.NodeName,
				"pod":       pod.PodRef.Name,
				"namespace": pod.PodRef.Namespace,
			}
			for k, v := range podLabels {
				tags[k] = v
			}
			fields := make(map[string]interface{})
			fields["pod_network_rx_bytes"] = pod.Network.RXBytes
			fields["pod_network_rx_errors"] = pod.Network.RXErrors
			fields["pod_network_tx_bytes"] = pod.Network.TXBytes
			fields["pod_network_tx_errors"] = pod.Network.TXErrors
			slist.PushSamples(inputName, fields, tags)
		}
	}
}

func (ins *Instance) buildSystemContainerMetrics(summaryMetrics *SummaryMetrics, slist *types.SampleList) {
	for _, container := range summaryMetrics.Node.SystemContainers {
		tags := map[string]string{
			"node":      summaryMetrics.Node.NodeName,
			"container": container.Name,
		}

		fields := make(map[string]interface{})
		fields["system_container_cpu_usage_nanocores"] = container.CPU.UsageNanoCores
		fields["system_container_cpu_usage_core_nanoseconds"] = container.CPU.UsageCoreNanoSeconds
		fields["system_container_memory_usage_bytes"] = container.Memory.UsageBytes
		fields["system_container_memory_working_set_bytes"] = container.Memory.WorkingSetBytes
		fields["system_container_memory_rss_bytes"] = container.Memory.RSSBytes
		fields["system_container_memory_page_faults"] = container.Memory.PageFaults
		fields["system_container_memory_major_page_faults"] = container.Memory.MajorPageFaults
		fields["system_container_rootfs_available_bytes"] = container.RootFS.AvailableBytes
		fields["system_container_rootfs_capacity_bytes"] = container.RootFS.CapacityBytes
		fields["system_container_logsfs_available_bytes"] = container.LogsFS.AvailableBytes
		fields["system_container_logsfs_capacity_bytes"] = container.LogsFS.CapacityBytes

		slist.PushSamples(inputName, fields, tags)
	}
}

func (ins *Instance) buildNodeMetrics(summaryMetrics *SummaryMetrics, slist *types.SampleList) {
	tags := map[string]string{
		"node": summaryMetrics.Node.NodeName,
	}
	fields := make(map[string]interface{})
	fields["node_cpu_usage_nanocores"] = summaryMetrics.Node.CPU.UsageNanoCores
	fields["node_cpu_usage_core_nanoseconds"] = summaryMetrics.Node.CPU.UsageCoreNanoSeconds
	fields["node_memory_available_bytes"] = summaryMetrics.Node.Memory.AvailableBytes
	fields["node_memory_usage_bytes"] = summaryMetrics.Node.Memory.UsageBytes
	fields["node_memory_working_set_bytes"] = summaryMetrics.Node.Memory.WorkingSetBytes
	fields["node_memory_rss_bytes"] = summaryMetrics.Node.Memory.RSSBytes
	fields["node_memory_page_faults"] = summaryMetrics.Node.Memory.PageFaults
	fields["node_memory_major_page_faults"] = summaryMetrics.Node.Memory.MajorPageFaults
	fields["node_network_rx_bytes"] = summaryMetrics.Node.Network.RXBytes
	fields["node_network_rx_errors"] = summaryMetrics.Node.Network.RXErrors
	fields["node_network_tx_bytes"] = summaryMetrics.Node.Network.TXBytes
	fields["node_network_tx_errors"] = summaryMetrics.Node.Network.TXErrors
	fields["node_fs_available_bytes"] = summaryMetrics.Node.FileSystem.AvailableBytes
	fields["node_fs_capacity_bytes"] = summaryMetrics.Node.FileSystem.CapacityBytes
	fields["node_fs_used_bytes"] = summaryMetrics.Node.FileSystem.UsedBytes
	fields["node_runtime_image_fs_available_bytes"] = summaryMetrics.Node.Runtime.ImageFileSystem.AvailableBytes
	fields["node_runtime_image_fs_capacity_bytes"] = summaryMetrics.Node.Runtime.ImageFileSystem.CapacityBytes
	fields["node_runtime_image_fs_used_bytes"] = summaryMetrics.Node.Runtime.ImageFileSystem.UsedBytes

	slist.PushSamples(inputName, fields, tags)
}

func (ins *Instance) gatherPodInfo(baseURL string) ([]Metadata, error) {
	var podAPI Pods
	err := ins.LoadJSON(fmt.Sprintf("%s/pods", baseURL), &podAPI)
	if err != nil {
		return nil, err
	}
	var podInfos []Metadata
	for _, podMetadata := range podAPI.Items {
		podInfos = append(podInfos, podMetadata.Metadata)
	}
	return podInfos, nil
}

func (ins *Instance) LoadJSON(url string, v interface{}) error {
	var req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	var resp *http.Response
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return err
	}
	if ins.RoundTripper == nil {
		if ins.ResponseTimeout < config.Duration(time.Second) {
			ins.ResponseTimeout = config.Duration(time.Second * 5)
		}
		ins.RoundTripper = &http.Transport{
			TLSHandshakeTimeout:   5 * time.Second,
			TLSClientConfig:       tlsCfg,
			ResponseHeaderTimeout: time.Duration(ins.ResponseTimeout),
		}
	}
	req.Header.Set("Authorization", "Bearer "+ins.BearerTokenString)
	req.Header.Add("Accept", "application/json")
	resp, err = ins.RoundTripper.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("error making HTTP request to %s: %s", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP status %s", url, resp.Status)
	}

	err = json.NewDecoder(resp.Body).Decode(v)
	if err != nil {
		return fmt.Errorf(`error parsing response: %s`, err)
	}

	return nil
}
