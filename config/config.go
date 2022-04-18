package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"flashcat.cloud/categraf/pkg/cfg"
	"github.com/toolkits/pkg/file"
)

type Global struct {
	PrintConfigs bool              `toml:"print_configs"`
	Hostname     string            `toml:"hostname"`
	OmitHostname bool              `toml:"omit_hostname"`
	Labels       map[string]string `toml:"labels"`
	Precision    string            `toml:"precision"`
	Interval     Duration          `toml:"interval"`
}

type WriterOpt struct {
	Batch    int `toml:"batch"`
	ChanSize int `toml:"chan_size"`
}

type WriterOption struct {
	Url           string `toml:"url"`
	BasicAuthUser string `toml:"basic_auth_user"`
	BasicAuthPass string `toml:"basic_auth_pass"`

	Timeout               int64 `toml:"timeout"`
	DialTimeout           int64 `toml:"dial_timeout"`
	TLSHandshakeTimeout   int64 `toml:"tls_handshake_timeout"`
	ExpectContinueTimeout int64 `toml:"expect_continue_timeout"`
	IdleConnTimeout       int64 `toml:"idle_conn_timeout"`
	KeepAlive             int64 `toml:"keep_alive"`

	MaxConnsPerHost     int `toml:"max_conns_per_host"`
	MaxIdleConns        int `toml:"max_idle_conns"`
	MaxIdleConnsPerHost int `toml:"max_idle_conns_per_host"`
}

type ConfigType struct {
	// from console args
	ConfigDir string
	DebugMode bool
	TestMode  bool

	// from config.toml
	Global    Global         `toml:"global"`
	WriterOpt WriterOpt      `toml:"writer_opt"`
	Writers   []WriterOption `toml:"writers"`
}

var Config *ConfigType

func InitConfig(configDir string, debugMode bool, testMode bool) error {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	Config = &ConfigType{
		ConfigDir: configDir,
		DebugMode: debugMode,
		TestMode:  testMode,
	}

	if err := cfg.LoadConfigs(configDir, Config); err != nil {
		return fmt.Errorf("failed to load configs of dir: %s", configDir)
	}

	if Config.Global.Hostname == "" {
		name, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %v", err)
		}
		Config.Global.Hostname = name
	}

	if Config.Global.PrintConfigs {
		bs, _ := json.MarshalIndent(Config, "", "    ")
		fmt.Println(string(bs))
	}

	return nil
}

func GetInterval() time.Duration {
	if Config.Global.Interval <= 0 {
		return time.Second * 15
	}

	return time.Duration(Config.Global.Interval)
}
