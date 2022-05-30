package ntp

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (n *NTPStat) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
