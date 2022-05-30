package net

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (n *NetIOStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
