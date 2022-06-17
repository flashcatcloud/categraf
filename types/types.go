package types

import "time"

type Sample struct {
	Metric    string            `json:"metric"`
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

type ClickHouseSample struct {
	Timestamp time.Time `json:"timestamp"`
	Metric    string    `json:"metric"`
	Tags      string    `json:"tags"`
	Value     float64   `json:"value"`
}
