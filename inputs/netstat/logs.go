package netstat

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (n *NetStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
