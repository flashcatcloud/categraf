package mem

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (c *MemStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
