package config

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/toolkits/pkg/file"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/pkg/cfg"
)

const (
	Docker     = "docker"
	Kubernetes = "kubernetes"
)

type Logs struct {
	APIKey                string                       `toml:"api_key"`
	Enable                bool                         `toml:"enable"`
	SendTo                string                       `toml:"send_to"`
	SendType              string                       `toml:"send_type"`
	UseCompression        bool                         `toml:"use_compression"`
	CompressionLevel      int                          `toml:"compression_level"`
	SendWithTLS           bool                         `toml:"send_with_tls"`
	BatchWait             int                          `toml:"batch_wait"`
	RunPath               string                       `toml:"run_path"`
	OpenFilesLimit        int                          `toml:"open_files_limit"`
	ScanPeriod            int                          `toml:"scan_period"`
	FrameSize             int                          `toml:"frame_size"`
	CollectContainerAll   bool                         `toml:"collect_container_all"`
	GlobalProcessingRules []*logsconfig.ProcessingRule `toml:"processing_rules"`
	Items                 []*logsconfig.LogsConfig     `toml:"items"`
}

var (
	LogConfig *Logs
)

func InitLogConfig(configDir string) error {
	configFile := path.Join(configDir, "logs.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	LogConfig = &Logs{}
	if err := cfg.LoadConfig(configFile, LogConfig); err != nil {
		return fmt.Errorf("failed to load configs of dir: %s", configDir)
	}

	if Config != nil && Config.Global.PrintConfigs {
		bs, _ := json.MarshalIndent(LogConfig, "", "    ")
		fmt.Println(string(bs))
	}

	return nil
}

func GetLogRunPath() string {
	if len(LogConfig.RunPath) == 0 {
		LogConfig.RunPath = "/opt/categraf/run"
	}
	return LogConfig.RunPath
}
func GetLogReadTimeout() int {
	return 30
}

func OpenLogsLimit() int {
	if LogConfig.OpenFilesLimit == 0 {
		LogConfig.OpenFilesLimit = 100
	}
	return LogConfig.OpenFilesLimit
}

func FileScanPeriod() int {
	if LogConfig.ScanPeriod == 0 {
		LogConfig.ScanPeriod = 10
	}
	return LogConfig.ScanPeriod
}

func LogFrameSize() int {
	if LogConfig.FrameSize == 0 {
		LogConfig.FrameSize = 9000
	}
	return LogConfig.FrameSize
}

func ValidatePodContainerID() bool {
	return false
}

func IsFeaturePresent(t string) bool {
	return false
}

func GetContainerCollectAll() bool {
	return LogConfig.CollectContainerAll
}
