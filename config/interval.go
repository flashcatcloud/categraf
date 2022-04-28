package config

type Interval struct {
	Interval Duration `toml:"interval"`
}

func (i Interval) GetInterval() Duration {
	return i.Interval
}
