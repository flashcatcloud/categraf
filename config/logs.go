//go:build !no_logs

package config

import (
	"github.com/IBM/sarama"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/pkg/tls"
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
		ContainerInclude      []string                     `json:"container_include" toml:"container_include"`
		ContainerExclude      []string                     `json:"container_exclude" toml:"container_exclude"`
		GlobalProcessingRules []*logsconfig.ProcessingRule `json:"processing_rules" toml:"processing_rules"`
		Items                 []*logsconfig.LogsConfig     `json:"items" toml:"items"`
		Accuracy              string                       `toml:"accuracy" json:"accuracy"`
		KafkaConfig
		KubeConfig
	}
	KafkaConfig struct {
		Topic   string   `json:"topic" toml:"topic"`
		Brokers []string `json:"brokers" toml:"brokers"`
		*sarama.Config

		KafkaVersion     string `toml:"kafka_version"`
		SaslEnable       bool   `toml:"sasl_enable"`
		SaslMechanism    string `toml:"sasl_mechanism"`
		SaslVersion      int16  `toml:"sasl_version"`
		SaslHandshake    bool   `toml:"sasl_handshake"`
		SaslUser         string `toml:"sasl_user"`
		SaslPassword     string `toml:"sasl_password"`
		SaslAuthIdentity string `toml:"sasl_auth_identity"`

		CertificateAuth []string `toml:"certificate_authorities"`
		tls.ClientConfig
		PartitionStrategy string `toml:"partition_strategy"`
	}
	KubeConfig struct {
		KubeletHTTPPort  int    `json:"kubernetes_http_kubelet_port" toml:"kubernetes_http_kubelet_port"`
		KubeletHTTPSPort int    `json:"kubernetes_https_kubelet_port" toml:"kubernetes_https_kubelet_port"`
		KubeletTokenPath string `json:"kubelet_auth_token_path" toml:"kubelet_auth_token_path"`
		KubeletCAPath    string `json:"kubelet_client_ca" toml:"kubelet_client_ca"`
	}
)

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

func GetContainerIncludeList() []string {
	if Config.Logs.ContainerInclude == nil {
		return []string{}
	}
	return Config.Logs.ContainerInclude
}

func GetContainerExcludeList() []string {
	if Config.Logs.ContainerExclude == nil {
		return []string{}
	}
	return Config.Logs.ContainerExclude
}
