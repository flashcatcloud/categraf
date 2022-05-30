package system

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (s *SystemStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
