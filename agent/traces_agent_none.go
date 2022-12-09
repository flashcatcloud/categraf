//go:build no_traces

package agent

type TracesAgent struct {
}

func NewTracesAgent() AgentModule {
	return nil
}

func (ta *TracesAgent) Start() (err error) {
	return nil
}

func (ta *TracesAgent) Stop() (err error) {
	return nil
}
