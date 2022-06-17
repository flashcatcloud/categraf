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

var envVarEscaper = strings.NewReplacer(
	`"`, `\"`,
	`\`, `\\`,
)

type Global struct {
	WriteClickHouse bool              `toml:"write_clickhouse"`
	PrintConfigs    bool              `toml:"print_configs"`
	Hostname        string            `toml:"hostname"`
	IP              string            `toml:"-"`
	OmitHostname    bool              `toml:"omit_hostname"`
	Labels          map[string]string `toml:"labels"`
	Precision       string            `toml:"precision"`
	Interval        Duration          `toml:"interval"`
}

type WriterOpt struct {
	Batch    int `toml:"batch"`
	ChanSize int `toml:"chan_size"`
}

type WriterOption struct {
	Url                 string   `toml:"url"`
	ClickHouseEndpoints []string `toml:"clickhouse_endpoints"`
	StorageType         string   `toml:"storage_type"`
	ClickHouseDB        string   `toml:"clickhouse_db"`
	BasicAuthUser       string   `toml:"basic_auth_user"`
	BasicAuthPass       string   `toml:"basic_auth_pass"`

	Timeout             int64 `toml:"timeout"`
	DialTimeout         int64 `toml:"dial_timeout"`
	MaxIdleConnsPerHost int   `toml:"max_idle_conns_per_host"`
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

	if err := Config.fillIP(); err != nil {
		return err
	}

	if err := InitHostname(); err != nil {
		return err
	}

	if Config.Global.PrintConfigs {
		bs, _ := json.MarshalIndent(Config, "", "    ")
		fmt.Println(string(bs))
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
	ret = os.Expand(ret, getEnv)

	return ret
}

func getEnv(key string) string {
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
