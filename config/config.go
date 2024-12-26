package config

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/tls"
	jsoniter "github.com/json-iterator/go"
	"github.com/toolkits/pkg/file"
)

const (
	defaultProbeAddr = "223.5.5.5:80"
)

var envVarEscaper = strings.NewReplacer(
	`"`, `\"`,
	`\`, `\\`,
)

type Global struct {
	PrintConfigs bool              `toml:"print_configs"`
	Hostname     string            `toml:"hostname"`
	OmitHostname bool              `toml:"omit_hostname"`
	Labels       map[string]string `toml:"labels"`
	Precision    string            `toml:"precision"`
	Interval     Duration          `toml:"interval"`
	Providers    []string          `toml:"providers"`
	Concurrency  int               `toml:"concurrency"`
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

	tls.ClientConfig
}

type HTTP struct {
	Enable         bool   `toml:"enable"`
	Address        string `toml:"address"`
	PrintAccess    bool   `toml:"print_access"`
	RunMode        string `toml:"run_mode"`
	IgnoreHostname bool   `toml:"ignore_hostname"`
	// The tag used to name the agent host
	AgentHostTag       string `toml:"agent_host_tag"`
	IgnoreGlobalLabels bool   `toml:"ignore_global_labels"`
	CertFile           string `toml:"cert_file"`
	KeyFile            string `toml:"key_file"`
	ReadTimeout        int    `toml:"read_timeout"`
	WriteTimeout       int    `toml:"write_timeout"`
	IdleTimeout        int    `toml:"idle_timeout"`
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
	DebugLevel   int
	TestMode     bool
	InputFilters string

	// from config.toml
	Global     Global           `toml:"global"`
	WriterOpt  WriterOpt        `toml:"writer_opt"`
	Writers    []WriterOption   `toml:"writers"`
	Logs       Logs             `toml:"logs"`
	HTTP       *HTTP            `toml:"http"`
	Prometheus *Prometheus      `toml:"prometheus"`
	Ibex       *IbexConfig      `toml:"ibex"`
	Heartbeat  *HeartbeatConfig `toml:"heartbeat"`
	Log        Log              `toml:"log"`

	HTTPProviderConfig *HTTPProviderConfig `toml:"http_provider"`
}

var Config *ConfigType

func InitConfig(configDir string, debugLevel int, debugMode, testMode bool, interval int64, inputFilters string) error {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return fmt.Errorf("configuration file(%s) not found", configFile)
	}

	Config = &ConfigType{
		ConfigDir:    configDir,
		DebugMode:    debugMode,
		DebugLevel:   debugLevel,
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

	Config.Global.Hostname = strings.TrimSpace(Config.Global.Hostname)

	if err := InitHostInfo(); err != nil {
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

func (c *ConfigType) GetHostname() string {
	ret := c.Global.Hostname

	name := HostInfo.GetHostname()
	if ret == "" {
		return name
	}

	ret = strings.Replace(ret, "$hostname", name, -1)
	ret = strings.Replace(ret, "$ip", c.GetHostIP(), -1)
	ret = strings.Replace(ret, "$sn", c.GetHostSN(), -1)
	ret = os.Expand(ret, GetEnv)

	return ret
}
func (c *ConfigType) GetHostIP() string {
	ret := HostInfo.GetIP()
	if ret == "" {
		return c.GetHostname()
	}

	return ret
}
func (c *ConfigType) GetHostSN() string {
	ret := HostInfo.GetSN()
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

func GetConcurrency() int {
	if Config.Global.Concurrency <= 0 {
		return runtime.NumCPU() * 10
	}
	return Config.Global.Concurrency
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
	addr := defaultProbeAddr
	if len(Config.Writers) == 0 {
		log.Printf("E! writers is not configured, use %s as default probe address", defaultProbeAddr)
	}
	for _, v := range Config.Writers {
		if len(v.Url) != 0 {
			u, err := url.Parse(v.Url)
			if err != nil {
				log.Printf("W! parse writers url %s error %s", v.Url, err)
				continue
			} else {
				if strings.Contains(u.Host, "localhost") || strings.Contains(u.Host, "127.0.0.1") {
					continue
				}
				if len(u.Port()) == 0 {
					if u.Scheme == "http" {
						u.Host = u.Host + ":80"
					}
					if u.Scheme == "https" {
						u.Host = u.Host + ":443"
					}
				}
				addr = u.Host
				break
			}
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

func GetBiosSn() (string, error) {
	var sn string
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("cmd", "/C", "wmic bios get serialnumber").Output()
		if err != nil {
			return "", fmt.Errorf("failed to get bios sn: %v", err)
		}
		str := string(out)
		lines := strings.Split(str, "\r\n")
		if len(lines) > 2 {
			// 获取第二行
			sn = strings.TrimSpace(lines[1])
		}
	case "darwin":
		out, err := exec.Command("system_profiler", "SPHardwareDataType").Output()
		if err != nil {
			return "", fmt.Errorf("failed to get bios sn: %v", err)
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Serial Number (system)") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					sn = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	case "linux":
		out, err := exec.Command("dmidecode", "-s", "system-serial-number").Output()
		if err != nil {
			return "", fmt.Errorf("failed to get bios sn: %v", err)
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "#") {
				continue
			}
			if len(line) > 0 {
				sn = strings.TrimSpace(line)
				break
			}
		}
	default:
		return "", fmt.Errorf("not support os to get sn")
	}
	return sn, nil
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
	nv = strings.Replace(nv, "$ip", Config.GetHostIP(), -1)
	nv = strings.Replace(nv, "$sn", Config.GetHostSN(), -1)
	nv = os.Expand(nv, GetEnv)
	return nv
}
