package kernel_vmstat

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (s *KernelVmstat) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
