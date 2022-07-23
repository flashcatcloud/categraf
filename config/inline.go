package config

type Interval struct {
	Interval Duration `toml:"interval"`
}

func (i Interval) GetInterval() Duration {
	return i.Interval
}

type InstanceConfig struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
}

func (ic InstanceConfig) GetLabels() map[string]string {
	if ic.Labels != nil {
		return ic.Labels
	}

	return map[string]string{}
}

func (ic InstanceConfig) GetIntervalTimes() int64 {
	return ic.IntervalTimes
}
