package procstat

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (c *Procstat) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
