//go:build no_logs

package agent

type LogsAgent struct {
}

func NewLogsAgent() AgentModule {
	return nil
}

func (la *LogsAgent) Start() error {
	return nil
}

func (la *LogsAgent) Stop() error {
	return nil
}
