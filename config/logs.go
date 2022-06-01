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

type (
	Logs struct {
		APIKey                string                       `json:"api_key" toml:"api_key"`
		Enable                bool                         `json:"enable" toml:"enable"`
		SendTo                string                       `json:"send_to" toml:"send_to"`
		SendType              string                       `json:"send_type" toml:"send_type"`
		UseCompression        bool                         `json:"use_compression" toml:"use_compression"`
		CompressionLevel      int                          `json:"compression_level" toml:"compression_level"`
		SendWithTLS           bool                         `json:"send_with_tls" toml:"send_with_tls"`
		BatchWait             int                          `json:"batch_wait" toml:"batch_wait"`
		RunPath               string                       `json:"run_path" toml:"run_path"`
		OpenFilesLimit        int                          `json:"open_files_limit" toml:"open_files_limit"`
		ScanPeriod            int                          `json:"scan_period" toml:"scan_period"`
		FrameSize             int                          `json:"frame_size" toml:"frame_size"`
		CollectContainerAll   bool                         `json:"collect_container_all" toml:"collect_container_all"`
		GlobalProcessingRules []*logsconfig.ProcessingRule `json:"processing_rules" toml:"processing_rules"`
		Items                 []*logsconfig.LogsConfig     `json:"items" toml:"items"`
	}
	LogType struct {
		Logs *Logs `json:"logs" toml:"logs"`
	}
)

var (
	LogConfig *Logs
)

func InitLogConfig(configDir string) error {
	var (
		err error
	)
	configFile := path.Join(configDir, "logs.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}
	data := &LogType{}
	err = cfg.LoadConfig(configFile, data)
	if err != nil {
		return fmt.Errorf("failed to load config: %s, err: %s", configFile, err)
	}

	LogConfig = data.Logs
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
