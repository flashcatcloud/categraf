package disk

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (s *DiskStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
