package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
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
	Logs      Logs           `toml:"logs"`
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

	if err := Config.fillHostname(); err != nil {
		return err
	}

	if Config.Global.PrintConfigs {
		bs, _ := json.MarshalIndent(Config, "", "    ")
		fmt.Println(string(bs))
	}

	return nil
}

func (c *ConfigType) fillHostname() error {
	if c.Global.Hostname == "" {
		name, err := GetHostname()
		if err != nil {
			return err
		}

		c.Global.Hostname = name
		return nil
	}

	if strings.Contains(c.Global.Hostname, "$hostname") {
		name, err := GetHostname()
		if err != nil {
			return err
		}

		c.Global.Hostname = strings.Replace(c.Global.Hostname, "$hostname", name, -1)
	}

	if strings.Contains(c.Global.Hostname, "$ip") {
		ip, err := GetOutboundIP()
		if err != nil {
			return err
		}

		c.Global.Hostname = strings.Replace(c.Global.Hostname, "$ip", fmt.Sprint(ip), -1)
	}

	return nil
}

func GetInterval() time.Duration {
	if Config.Global.Interval <= 0 {
		return time.Second * 15
	}

	return time.Duration(Config.Global.Interval)
}

func GetHostname() (string, error) {
	return os.Hostname()
}

// Get preferred outbound ip of this machine
func GetOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, fmt.Errorf("failed to get outbound ip: %v", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
