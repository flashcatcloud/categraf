package config

import (
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"flashcat.cloud/categraf/config/traces"
	"flashcat.cloud/categraf/pkg/cfg"
	jsoniter "github.com/json-iterator/go"
	"github.com/toolkits/pkg/file"
)

var envVarEscaper = strings.NewReplacer(
	`"`, `\"`,
	`\`, `\\`,
)

type Global struct {
	PrintConfigs bool              `toml:"print_configs"`
	Hostname     string            `toml:"hostname"`
	IP           string            `toml:"-"`
	OmitHostname bool              `toml:"omit_hostname"`
	Labels       map[string]string `toml:"labels"`
	Precision    string            `toml:"precision"`
	Interval     Duration          `toml:"interval"`
	Providers    []string          `toml:"providers"`
}

type WriterOpt struct {
	Batch    int `toml:"batch"`
	ChanSize int `toml:"chan_size"`
}

type WriterOption struct {
	Url           string   `toml:"url"`
	BasicAuthUser string   `toml:"basic_auth_user"`
	BasicAuthPass string   `toml:"basic_auth_pass"`
	Headers       []string `toml:"headers"`

	Timeout             int64 `toml:"timeout"`
	DialTimeout         int64 `toml:"dial_timeout"`
	MaxIdleConnsPerHost int   `toml:"max_idle_conns_per_host"`
}

type HTTP struct {
	Enable       bool   `toml:"enable"`
	Address      string `toml:"address"`
	PrintAccess  bool   `toml:"print_access"`
	RunMode      string `toml:"run_mode"`
	CertFile     string `toml:"cert_file"`
	KeyFile      string `toml:"key_file"`
	ReadTimeout  int    `toml:"read_timeout"`
	WriteTimeout int    `toml:"write_timeout"`
	IdleTimeout  int    `toml:"idle_timeout"`
}

type ConfigType struct {
	// from console args
	ConfigDir string
	DebugMode bool
	TestMode  bool

	DisableUsageReport bool `toml:"disable_usage_report"`

	// from config.toml
	Global       Global         `toml:"global"`
	WriterOpt    WriterOpt      `toml:"writer_opt"`
	Writers      []WriterOption `toml:"writers"`
	Logs         Logs           `toml:"logs"`
	MetricsHouse MetricsHouse   `toml:"metricshouse"`
	Traces       *traces.Config `toml:"traces"`
	HTTP         *HTTP          `toml:"http"`
	Prometheus   *Prometheus    `toml:"prometheus"`

	HttpRemoteProviderConfig *HttpRemoteProviderConfig `toml:"http_remote_provider"`
}

var Config *ConfigType

func InitConfig(configDir string, debugMode, testMode bool, interval int64) error {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	Config = &ConfigType{
		ConfigDir: configDir,
		DebugMode: debugMode,
		TestMode:  testMode,
	}

	if err := cfg.LoadConfigByDir(configDir, Config); err != nil {
		return fmt.Errorf("failed to load configs of dir: %s err:%s", configDir, err)
	}

	if interval > 0 {
		Config.Global.Interval = Duration(time.Duration(interval) * time.Second)
	}

	if err := Config.fillIP(); err != nil {
		return err
	}

	if err := InitHostname(); err != nil {
		return err
	}

	if err := traces.Parse(Config.Traces); err != nil {
		return err
	}

	if Config.Global.PrintConfigs {
		json := jsoniter.ConfigCompatibleWithStandardLibrary
		bs, err := json.MarshalIndent(Config, "", "    ")
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(string(bs))
		}
	}

	return nil
}

func (c *ConfigType) fillIP() error {
	if !strings.Contains(c.Global.Hostname, "$ip") {
		return nil
	}

	ip, err := GetOutboundIP()
	if err != nil {
		return err
	}

	c.Global.IP = fmt.Sprint(ip)
	return nil
}

func (c *ConfigType) GetHostname() string {
	ret := c.Global.Hostname

	name := Hostname.Get()
	if ret == "" {
		return name
	}

	ret = strings.Replace(ret, "$hostname", name, -1)
	ret = strings.Replace(ret, "$ip", c.Global.IP, -1)
	ret = os.Expand(ret, GetEnv)

	return ret
}

func GetEnv(key string) string {
	v := os.Getenv(key)
	return envVarEscaper.Replace(v)
}

func GetInterval() time.Duration {
	if Config.Global.Interval <= 0 {
		return time.Second * 15
	}

	return time.Duration(Config.Global.Interval)
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
