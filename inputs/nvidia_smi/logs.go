package nvidia_smi

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (g *GPUStats) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
