package config

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"flashcat.cloud/categraf/config/traces"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/tls"
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

type Log struct {
	FileName   string `toml:"file_name"`
	MaxSize    int    `toml:"max_size"`
	MaxAge     int    `toml:"max_age"`
	MaxBackups int    `toml:"max_backups"`
	LocalTime  bool   `toml:"local_time"`
	Compress   bool   `toml:"compress"`
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

type IbexConfig struct {
	Enable   bool
	Interval Duration `toml:"interval"`
	MetaDir  string   `toml:"meta_dir"`
	Servers  []string `toml:"servers"`
}

type HeartbeatConfig struct {
	Enable              bool     `toml:"enable"`
	Url                 string   `toml:"url"`
	Interval            int64    `toml:"interval"`
	BasicAuthUser       string   `toml:"basic_auth_user"`
	BasicAuthPass       string   `toml:"basic_auth_pass"`
	Headers             []string `toml:"headers"`
	Timeout             int64    `toml:"timeout"`
	DialTimeout         int64    `toml:"dial_timeout"`
	MaxIdleConnsPerHost int      `toml:"max_idle_conns_per_host"`

	HTTPProxy
	tls.ClientConfig
}

type ConfigType struct {
	// from console args
	ConfigDir    string
	DebugMode    bool
	TestMode     bool
	InputFilters string

	// from config.toml
	Global     Global           `toml:"global"`
	WriterOpt  WriterOpt        `toml:"writer_opt"`
	Writers    []WriterOption   `toml:"writers"`
	Logs       Logs             `toml:"logs"`
	Traces     *traces.Config   `toml:"traces"`
	HTTP       *HTTP            `toml:"http"`
	Prometheus *Prometheus      `toml:"prometheus"`
	Ibex       *IbexConfig      `toml:"ibex"`
	Heartbeat  *HeartbeatConfig `toml:"heartbeat"`
	Log        Log              `toml:"log"`

	HTTPProviderConfig *HTTPProviderConfig `toml:"http_provider"`
}

var Config *ConfigType

func InitConfig(configDir string, debugMode, testMode bool, interval int64, inputFilters string) error {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	Config = &ConfigType{
		ConfigDir:    configDir,
		DebugMode:    debugMode,
		TestMode:     testMode,
		InputFilters: inputFilters,
	}

	if err := cfg.LoadConfigByDir(configDir, Config); err != nil {
		return fmt.Errorf("failed to load configs of dir: %s err:%s", configDir, err)
	}

	if interval > 0 {
		Config.Global.Interval = Duration(time.Duration(interval) * time.Second)
	}

	if Config.Global.Precision == "" {
		Config.Global.Precision = "ms"
	}

	if Config.WriterOpt.ChanSize <= 0 {
		Config.WriterOpt.ChanSize = 1000000
	}

	if Config.WriterOpt.Batch <= 0 {
		Config.WriterOpt.Batch = 1000
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

	// If using test mode, the logs are output to standard output for easy viewing
	if testMode {
		Config.Log.FileName = "stdout"
	}

	return nil
}

func (c *ConfigType) fillIP() error {
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

func getLocalIP() (net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifs {
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			log.Println("W! iface address error", err)
			continue
		}
		for _, addr := range addrs {
			if ip, ok := addr.(*net.IPNet); ok && ip.IP.IsLoopback() {
				continue
			} else {
				ip4 := ip.IP.To4()
				if ip4 == nil {
					continue
				}
				return ip4, nil
			}
		}
	}
	return nil, fmt.Errorf("no local ip found")
}

// Get preferred outbound ip of this machine
func GetOutboundIP() (net.IP, error) {
	var addr string
	for _, v := range Config.Writers {
		if len(v.Url) != 0 {
			u, err := url.Parse(v.Url)
			if err != nil {
				log.Println("W! parse writers url error", err)
				addr = "223.5.5.5:80"
			} else {
				if len(u.Port()) == 0 {
					if u.Scheme == "http" {
						u.Host = u.Host + ":80"
					}
					if u.Scheme == "https" {
						u.Host = u.Host + ":443"
					}
				}
				addr = u.Host
			}
			break
		}
	}

	conn, err := net.Dial("udp", addr)
	if err != nil {
		ip, err := getLocalIP()
		if err != nil {
			return nil, fmt.Errorf("failed to get local ip: %v", err)
		}
		return ip, nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}

func GlobalLabels() map[string]string {
	ret := make(map[string]string)
	for k, v := range Config.Global.Labels {
		ret[k] = Expand(v)
	}
	return ret
}

func Expand(nv string) string {
	nv = strings.Replace(nv, "$hostname", Config.GetHostname(), -1)
	nv = strings.Replace(nv, "$ip", Config.Global.IP, -1)
	nv = os.Expand(nv, GetEnv)
	return nv
}
