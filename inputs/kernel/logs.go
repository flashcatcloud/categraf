package kernel

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (s *KernelStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
