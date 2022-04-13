package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"

	"flashcat.cloud/categraf/pkg/cfg"
	"github.com/toolkits/pkg/file"
)

type Global struct {
	PrintConfig  bool
	Hostname     string
	OmitHostname bool
	Labels       map[string]string
}

type WriterOption struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type ConfigType struct {
	ConfigDir string
	DebugMode bool

	Global  Global
	Writers []WriterOption
}

var Config *ConfigType

func InitConfig(configDir, debugMode string) error {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	debug, err := strconv.ParseBool(debugMode)
	if err != nil {
		return fmt.Errorf("failed to parse bool(%s): %v", debugMode, err)
	}

	Config = &ConfigType{
		ConfigDir: configDir,
		DebugMode: debug,
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

	if Config.Global.PrintConfig {
		bs, _ := json.MarshalIndent(Config, "", "    ")
		fmt.Println(string(bs))
	}

	return nil
}
