package ping

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (c *Ping) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
