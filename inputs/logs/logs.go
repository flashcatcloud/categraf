package logs

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

type Logs struct {
	Log []*logsconfig.LogsConfig `toml:"logs"`
}

func (l Logs) LogsConfig() []*logsconfig.LogsConfig {
	return l.Log
}
