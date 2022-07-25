package config

import (
	"time"

	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const agentHostnameLabelKey = "agent_hostname"

type InternalConfig struct {
	Labels            map[string]string `toml:"labels"`
	MetricsDrop       []string          `toml:"metrics_drop"`
	MetricsPass       []string          `toml:"metrics_pass"`
	MetricsNamePrefix string            `toml:"metrics_name_prefix"`
	MetricsDropFilter filter.Filter
	MetricsPassFilter filter.Filter
}

func (ic *InternalConfig) GetLabels() map[string]string {
	if ic.Labels != nil {
		return ic.Labels
	}

	return map[string]string{}
}

func (ic *InternalConfig) InitInternalConfig() error {
	if len(ic.MetricsDrop) > 0 {
		var err error
		ic.MetricsDropFilter, err = filter.Compile(ic.MetricsDrop)
		if err != nil {
			return err
		}
	}

	if len(ic.MetricsPass) > 0 {
		var err error
		ic.MetricsPassFilter, err = filter.Compile(ic.MetricsPass)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ic *InternalConfig) Process(slist *types.SampleList) *types.SampleList {
	nlst := types.NewSampleList()
	if slist.Len() == 0 {
		return nlst
	}

	now := time.Now()
	ss := slist.PopBackAll()

	for i := range ss {
		if ss[i] == nil {
			continue
		}

		// drop metrics
		if ic.MetricsDropFilter != nil {
			if ic.MetricsDropFilter.Match(ss[i].Metric) {
				continue
			}
		}

		// pass metrics
		if ic.MetricsPassFilter != nil {
			if !ic.MetricsPassFilter.Match(ss[i].Metric) {
				continue
			}
		}

		if ss[i].Timestamp.IsZero() {
			ss[i].Timestamp = now
		}

		// name prefix
		if len(ic.MetricsNamePrefix) > 0 {
			ss[i].Metric = ic.MetricsNamePrefix + ss[i].Metric
		}

		// add instance labels
		labels := ic.GetLabels()
		for k, v := range labels {
			if v == "-" {
				delete(ss[i].Labels, k)
				continue
			}
			ss[i].Labels[k] = v
		}

		// add global labels
		for k, v := range Config.Global.Labels {
			if _, has := ss[i].Labels[k]; !has {
				ss[i].Labels[k] = v
			}
		}

		// add label: agent_hostname
		if _, has := ss[i].Labels[agentHostnameLabelKey]; !has {
			if !Config.Global.OmitHostname {
				ss[i].Labels[agentHostnameLabelKey] = Config.GetHostname()
			}
		}

		nlst.PushFront(ss[i])
	}

	return nlst
}

type PluginConfig struct {
	InternalConfig
	Interval Duration `toml:"interval"`
}

func (pc *PluginConfig) GetInterval() Duration {
	return pc.Interval
}

type InstanceConfig struct {
	InternalConfig
	IntervalTimes int64 `toml:"interval_times"`
}

func (ic *InstanceConfig) GetIntervalTimes() int64 {
	return ic.IntervalTimes
}
