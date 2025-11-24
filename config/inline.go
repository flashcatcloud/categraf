package config

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	"flashcat.cloud/categraf/pkg/filter"
	modelLabel "flashcat.cloud/categraf/pkg/prom/labels"
	"flashcat.cloud/categraf/pkg/relabel"
	"flashcat.cloud/categraf/types"
)

const agentHostnameLabelKey = "agent_hostname"

type ProcessorEnum struct {
	Metrics       []string `toml:"metrics"` // support glob
	MetricsFilter filter.Filter
	ValueMappings map[string]float64 `toml:"value_mappings"`
}

type InternalConfig struct {
	// append labels
	Labels map[string]string `toml:"labels"`

	// metrics drop and pass filter
	MetricsDrop       []string `toml:"metrics_drop"`
	MetricsPass       []string `toml:"metrics_pass"`
	MetricsDropFilter filter.Filter
	MetricsPassFilter filter.Filter

	// metric name prefix
	MetricsNamePrefix string `toml:"metrics_name_prefix"`

	// mapping value
	ProcessorEnum []*ProcessorEnum `toml:"processor_enum"`

	// whether instance initial success
	inited bool `toml:"-"`

	RelabelConfigs []*RelabelConfig  `toml:"relabel_configs"`
	relabelConfigs []*relabel.Config `toml:"-"`

	// whether debug
	DebugMod bool `toml:"-"`
}
type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels model.LabelNames `toml:"source_labels,flow,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `toml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex string `toml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `toml:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `toml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `toml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action relabel.Action `toml:"action,omitempty"`
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
	ic.DebugMod = Config.DebugMode

	if len(ic.MetricsPass) > 0 {
		var err error
		ic.MetricsPassFilter, err = filter.Compile(ic.MetricsPass)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(ic.ProcessorEnum); i++ {
		if len(ic.ProcessorEnum[i].Metrics) > 0 {
			var err error
			ic.ProcessorEnum[i].MetricsFilter, err = filter.Compile(ic.ProcessorEnum[i].Metrics)
			if err != nil {
				return err
			}
		}
	}
	if len(ic.RelabelConfigs) != 0 {
		for _, rc := range ic.RelabelConfigs {
			if len(rc.Regex) == 0 {
				rc.Regex = "(.*)"
			}
			if len(rc.Action) == 0 {
				rc.Action = relabel.Replace
			}
			if len(rc.Replacement) == 0 {
				rc.Replacement = "$1"
			}
			if rc.Separator == "" {
				rc.Separator = ";"
			}
			reg, err := relabel.NewRegexp(rc.Regex)
			if err != nil {
				msg := fmt.Errorf("relabel_configs regex:%s compile error:%s", rc.Regex, err)
				return msg
			}
			r := &relabel.Config{
				SourceLabels: rc.SourceLabels,
				Separator:    rc.Separator,
				Regex:        reg,
				Modulus:      rc.Modulus,
				TargetLabel:  rc.TargetLabel,
				Replacement:  rc.Replacement,
				Action:       rc.Action,
			}
			ic.relabelConfigs = append(ic.relabelConfigs, r)
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

		// mapping values
		for j := 0; j < len(ic.ProcessorEnum); j++ {
			if ic.ProcessorEnum[j].MetricsFilter.Match(ss[i].Metric) {
				v, has := ic.ProcessorEnum[j].ValueMappings[fmt.Sprint(ss[i].Value)]
				if has {
					ss[i].Value = v
				}
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
			ss[i].Labels[k] = Expand(v)
		}

		// add global labels
		for k, v := range GlobalLabels() {
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
		// relabel
		if len(ic.relabelConfigs) != 0 {
			newName := ss[i].Metric
			all := make(modelLabel.Labels, 0, len(ss[i].Labels)+1)
			all = append(all, modelLabel.Label{Name: modelLabel.MetricName, Value: newName})
			for k, v := range ss[i].Labels {
				all = append(all, modelLabel.Label{Name: k, Value: v})
			}
			newAll, keep := relabel.Process(all, ic.relabelConfigs...)
			if !keep {
				continue
			}
			newLabel := make(map[string]string, len(newAll))
			for _, l := range newAll {
				if l.Name == modelLabel.MetricName {
					newName = l.Value
					continue
				}
				newLabel[l.Name] = l.Value
			}
			if newName != "" && newName != ss[i].Metric {
				ss[i].Metric = newName
			}
			ss[i].Labels = newLabel
		}

		nlst.PushFront(ss[i])
	}

	return nlst
}

func (ic *InternalConfig) Initialized() bool {
	return ic.inited
}

func (ic *InternalConfig) SetInitialized() {
	ic.inited = true
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
