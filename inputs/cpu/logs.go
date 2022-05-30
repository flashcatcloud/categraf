package cpu

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (c *CPUStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
