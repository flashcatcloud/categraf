package config

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
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
	CollectContainerAll   bool                         `toml:"container_collect_all"`
	GlobalProcessingRules []*logsconfig.ProcessingRule `toml:"processing_rules"`
}

func GetLogRunPath() string {
	if len(Config.Logs.RunPath) == 0 {
		Config.Logs.RunPath = "/opt/categraf/run"
	}
	return Config.Logs.RunPath
}
func GetLogReadTimeout() int {
	return 30
}

func OpenLogsLimit() int {
	if Config.Logs.OpenFilesLimit == 0 {
		Config.Logs.OpenFilesLimit = 100
	}
	return Config.Logs.OpenFilesLimit
}

func FileScanPeriod() int {
	if Config.Logs.ScanPeriod == 0 {
		Config.Logs.ScanPeriod = 10
	}
	return Config.Logs.ScanPeriod
}

func LogFrameSize() int {
	if Config.Logs.FrameSize == 0 {
		Config.Logs.FrameSize = 9000
	}
	return Config.Logs.FrameSize
}

func ValidatePodContainerID() bool {
	return false
}

func IsFeaturePresent(t string) bool {
	return false
}

func GetContainerCollectAll() bool {
	return Config.Logs.CollectContainerAll
}
