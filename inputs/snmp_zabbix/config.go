package snmp_zabbix

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
)

type Config struct {
	Agents          []AgentConfig
	Version         int
	Community       string
	Username        string
	AuthPassword    string
	AuthProtocol    string
	PrivPassword    string
	PrivProtocol    string
	SecurityLevel   string
	ContextName     string
	Port            int
	Timeout         time.Duration
	Retries         int
	MaxRepetitions  int
	TemplateFiles   []string
	EnableDiscovery bool

	UnconnectedUDPSocket bool
	DebugMode            bool

	config.InternalConfig

	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	healthCheckRetries  int
}

type AgentConfig struct {
	Host          string
	Port          int
	Transport     string
	Community     string
	Version       int
	Username      string
	AuthPassword  string
	AuthProtocol  string
	PrivPassword  string
	PrivProtocol  string
	SecurityLevel string
	ContextName   string
}

func NewConfig(s *Instance) (*Config, error) {
	config := &Config{
		Version:         s.Version,
		Community:       s.Community,
		Username:        s.Username,
		AuthPassword:    s.AuthPassword,
		AuthProtocol:    s.AuthProtocol,
		PrivPassword:    s.PrivPassword,
		PrivProtocol:    s.PrivProtocol,
		SecurityLevel:   s.SecurityLevel,
		ContextName:     s.ContextName,
		Port:            s.Port,
		Timeout:         s.Timeout,
		Retries:         s.Retries,
		MaxRepetitions:  s.MaxRepetitions,
		TemplateFiles:   s.TemplateFiles,
		EnableDiscovery: s.EnableDiscovery,

		UnconnectedUDPSocket: s.UnconnectedUDPSocket,
		DebugMode:            s.DebugMod,

		healthCheckInterval: s.HealthcheckInterval,
		healthCheckTimeout:  s.HealthcheckTimeout,
		healthCheckRetries:  s.HealthcheckRetries,
	}
	config.InternalConfig = s.InternalConfig

	// 解析agents配置
	agents, err := parseAgents(s.Agents, config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse agents: %w", err)
	}
	config.Agents = agents

	return config, nil
}

func parseAgents(agentStrings []string, defaultConfig *Config) ([]AgentConfig, error) {
	var agents []AgentConfig

	for _, agentStr := range agentStrings {
		parsedAgents, err := parseAgentString(agentStr, defaultConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agent string '%s': %w", agentStr, err)
		}
		agents = append(agents, parsedAgents...)
	}

	return agents, nil
}

func parseAgentString(agentStr string, defaultConfig *Config) ([]AgentConfig, error) {
	var agents []AgentConfig
	transport := "udp"
	addrPart := agentStr

	if strings.Contains(agentStr, "://") {
		parts := strings.SplitN(agentStr, "://", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid agent format with schema: %s", agentStr)
		}
		schema := strings.ToLower(parts[0])
		switch schema {
		case "udp", "tcp", "udp4", "tcp4", "udp6", "tcp6":
			transport = schema
		default:
			return nil, fmt.Errorf("unsupported schema: %s", schema)
		}
		addrPart = parts[1]
	}

	// 检查是否包含端口
	host, portStr, err := net.SplitHostPort(addrPart)
	if err != nil {
		// 没有端口，使用默认端口
		host = addrPart
		portStr = strconv.Itoa(defaultConfig.Port)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port '%s': %w", portStr, err)
	}

	// 检查是否是CIDR网段
	if strings.Contains(host, "/") {
		// 解析网段
		ip, ipNet, err := net.ParseCIDR(host)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR '%s': %w", host, err)
		}

		// 生成网段内的所有IP
		for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incrementIP(ip) {
			agent := AgentConfig{
				Host:          ip.String(),
				Port:          port,
				Transport:     transport,
				Community:     defaultConfig.Community,
				Version:       defaultConfig.Version,
				Username:      defaultConfig.Username,
				AuthPassword:  defaultConfig.AuthPassword,
				AuthProtocol:  defaultConfig.AuthProtocol,
				PrivPassword:  defaultConfig.PrivPassword,
				PrivProtocol:  defaultConfig.PrivProtocol,
				SecurityLevel: defaultConfig.SecurityLevel,
				ContextName:   defaultConfig.ContextName,
			}
			agents = append(agents, agent)
		}
	} else {
		// 单个IP地址
		agent := AgentConfig{
			Host:          host,
			Port:          port,
			Transport:     transport,
			Community:     defaultConfig.Community,
			Version:       defaultConfig.Version,
			Username:      defaultConfig.Username,
			AuthPassword:  defaultConfig.AuthPassword,
			AuthProtocol:  defaultConfig.AuthProtocol,
			PrivPassword:  defaultConfig.PrivPassword,
			PrivProtocol:  defaultConfig.PrivProtocol,
			SecurityLevel: defaultConfig.SecurityLevel,
			ContextName:   defaultConfig.ContextName,
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func (c *Config) GetAgentAddress(agent AgentConfig) string {
	return fmt.Sprintf("%s://%s:%d", agent.Transport, agent.Host, agent.Port)
}

func (c *Config) Validate() error {
	if len(c.Agents) == 0 {
		return fmt.Errorf("no agents configured")
	}

	if c.Version < 1 || c.Version > 3 {
		return fmt.Errorf("invalid SNMP version: %d", c.Version)
	}

	if c.Version <= 2 && c.Community == "" {
		return fmt.Errorf("community string required for SNMP v1/v2")
	}

	if c.Version == 3 {
		if c.Username == "" {
			return fmt.Errorf("username required for SNMP v3")
		}

		switch c.SecurityLevel {
		case "noAuthNoPriv", "authNoPriv", "authPriv":
			// valid
		case "":
			c.SecurityLevel = "noAuthNoPriv"
		default:
			return fmt.Errorf("invalid security level: %s", c.SecurityLevel)
		}

		if c.SecurityLevel != "noAuthNoPriv" && c.AuthPassword == "" {
			return fmt.Errorf("auth password required for security level: %s", c.SecurityLevel)
		}

		if c.SecurityLevel == "authPriv" && c.PrivPassword == "" {
			return fmt.Errorf("priv password required for security level: authPriv")
		}
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if c.Retries < 0 {
		return fmt.Errorf("retries cannot be negative")
	}

	return nil
}
