package types

type Sample struct {
	Metric    string            `json:"metric"`
	Timestamp int64             `json:"timestamp"`
	Value     interface{}       `json:"value"`
	Labels    map[string]string `json:"labels"`
}
