package snmp_zabbix

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

type SNMPClientManager struct {
	config  *Config
	clients map[string]*ClientWrapper
	mu      sync.RWMutex

	agentLocks map[string]*sync.Mutex

	// 健康检查相关
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	maxRetries          int
	stopHealthCheck     chan struct{}
	healthCheckRunning  bool
}

// ClientWrapper 包装SNMP客户端，添加健康状态
type ClientWrapper struct {
	client     *gosnmp.GoSNMP
	agent      string
	healthy    bool
	lastCheck  time.Time
	lastError  error
	retryCount int
	mu         sync.RWMutex

	// 连接统计
	successCount uint64
	errorCount   uint64
	lastSuccess  time.Time
	createTime   time.Time
}

func NewSNMPClientManager(config *Config) *SNMPClientManager {
	manager := &SNMPClientManager{
		config:              config,
		clients:             make(map[string]*ClientWrapper),
		healthCheckInterval: config.healthCheckInterval,
		healthCheckTimeout:  config.healthCheckTimeout,
		maxRetries:          config.healthCheckRetries,
		stopHealthCheck:     make(chan struct{}),
	}

	// 启动健康检查
	manager.startHealthCheck()

	return manager
}
func (m *SNMPClientManager) acquire(agent string) func() {
	m.mu.Lock()
	if m.agentLocks == nil {
		m.agentLocks = make(map[string]*sync.Mutex)
	}
	l, ok := m.agentLocks[agent]
	if !ok {
		l = &sync.Mutex{}
		m.agentLocks[agent] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// GetClient 获取健康的客户端连接
func (m *SNMPClientManager) GetClient(agent string) (*gosnmp.GoSNMP, error) {
	m.mu.RLock()
	wrapper, exists := m.clients[agent]
	m.mu.RUnlock()

	if exists {
		// 检查客户端健康状态
		if wrapper.IsHealthy() {
			wrapper.recordSuccess()
			return wrapper.client, nil
		}

		// 客户端不健康，尝试重连
		log.Printf("Client for %s is unhealthy, attempting to reconnect", agent)
		if err := m.reconnectClient(agent); err != nil {
			return nil, fmt.Errorf("failed to reconnect unhealthy client: %w", err)
		}

		// 重连成功后再次获取
		m.mu.RLock()
		wrapper = m.clients[agent]
		m.mu.RUnlock()

		if wrapper != nil && wrapper.IsHealthy() {
			return wrapper.client, nil
		}
	}

	// 创建新客户端
	if err := m.createNewClient(agent); err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	m.mu.RLock()
	wrapper = m.clients[agent]
	m.mu.RUnlock()

	if wrapper != nil && wrapper.client != nil {
		return wrapper.client, nil
	}

	return nil, fmt.Errorf("failed to get client for agent %s", agent)
}

// createNewClient 创建新的SNMP客户端
func (m *SNMPClientManager) createNewClient(agent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if wrapper, exists := m.clients[agent]; exists && wrapper.IsHealthy() {
		return nil
	}

	client, err := m.createClient(agent)
	if err != nil {
		return err
	}

	// 创建wrapper
	wrapper := &ClientWrapper{
		client:      client,
		agent:       agent,
		healthy:     true,
		lastCheck:   time.Now(),
		createTime:  time.Now(),
		lastSuccess: time.Now(),
	}

	// 执行初始健康检查
	if err := m.performHealthCheckNoLock(wrapper); err != nil {
		log.Printf("Initial health check failed for %s: %v", agent, err)
		wrapper.healthy = false
		wrapper.lastError = err
	}

	m.clients[agent] = wrapper
	return nil
}

// reconnectClient 重连客户端
func (m *SNMPClientManager) reconnectClient(agent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wrapper, exists := m.clients[agent]
	if !exists {
		return fmt.Errorf("client not found for agent %s", agent)
	}

	// 关闭旧连接
	if wrapper.client != nil && wrapper.client.Conn != nil {
		wrapper.client.Conn.Close()
	}

	// 创建新连接
	newClient, err := m.createClient(agent)
	if err != nil {
		wrapper.recordError(err)
		return err
	}

	// 更新wrapper
	wrapper.client = newClient
	wrapper.lastCheck = time.Now()

	// 执行健康检查
	if err := m.performHealthCheckNoLock(wrapper); err != nil {
		wrapper.recordError(err)
		return fmt.Errorf("health check failed after reconnect: %w", err)
	}

	wrapper.healthy = true
	wrapper.lastError = nil
	wrapper.retryCount = 0
	wrapper.lastSuccess = time.Now()

	log.Printf("Successfully reconnected client for %s", agent)
	return nil
}

// startHealthCheck 启动定期健康检查
func (m *SNMPClientManager) startHealthCheck() {
	if m.healthCheckRunning {
		return
	}

	m.healthCheckRunning = true
	go m.healthCheckLoop()
}

// healthCheckLoop 健康检查循环
func (m *SNMPClientManager) healthCheckLoop() {
	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performAllHealthChecks()
		case <-m.stopHealthCheck:
			log.Println("Stopping health check loop")
			return
		}
	}
}

// performAllHealthChecks 对所有客户端执行健康检查
func (m *SNMPClientManager) performAllHealthChecks() {
	m.mu.RLock()
	agents := make([]string, 0, len(m.clients))
	for agent := range m.clients {
		agents = append(agents, agent)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, agent := range agents {
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			m.checkClientHealth(a)
		}(agent)
	}

	// 等待所有检查完成，但设置超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有检查完成
	case <-time.After(m.healthCheckTimeout * 2):
		log.Println("Health check timeout, some checks may not have completed")
	}
}

// checkClientHealth 检查单个客户端健康状态
func (m *SNMPClientManager) checkClientHealth(agent string) {
	m.mu.RLock()
	wrapper, exists := m.clients[agent]
	m.mu.RUnlock()

	if !exists {
		return
	}

	// 执行健康检查
	err := m.performHealthCheckWithLock(agent, wrapper)

	wrapper.mu.Lock()
	defer wrapper.mu.Unlock()

	if err != nil {
		wrapper.lastError = err
		wrapper.retryCount++

		// 检查是否超过最大重试次数
		if wrapper.retryCount >= m.maxRetries {
			wrapper.healthy = false
			log.Printf("Client %s marked unhealthy after %d retries: %v",
				agent, wrapper.retryCount, err)

			// 尝试重连
			go func() {
				time.Sleep(5 * time.Second) // 延迟重连
				if err := m.reconnectClient(agent); err != nil {
					log.Printf("Failed to reconnect %s: %v", agent, err)
				}
			}()
		}
	} else {
		// 健康检查成功
		wrapper.healthy = true
		wrapper.lastError = nil
		wrapper.retryCount = 0
		wrapper.lastCheck = time.Now()
		wrapper.lastSuccess = time.Now()
	}
}

// performHealthCheck 执行实际的健康检查
func (m *SNMPClientManager) performHealthCheckNoLock(wrapper *ClientWrapper) error {
	if wrapper.client == nil {
		return fmt.Errorf("client is nil")
	}

	// 创建带超时的context
	ctx, cancel := context.WithTimeout(context.Background(), m.healthCheckTimeout)
	defer cancel()

	// 使用goroutine执行SNMP请求
	errChan := make(chan error, 1)
	go func() {
		// 执行简单的SNMP Get请求 (sysUpTime)
		_, err := wrapper.client.Get([]string{"1.3.6.1.2.1.1.3.0"})
		if err != nil {
			errChan <- fmt.Errorf("health check SNMP get failed: %w", err)
			return
		}

		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("health check timeout")
	}
}

func (m *SNMPClientManager) performHealthCheckWithLock(agent string, wrapper *ClientWrapper) error {
	unlock := m.acquire(agent)
	defer unlock()
	return m.performHealthCheckNoLock(wrapper)
}

// ClientWrapper 方法

func (w *ClientWrapper) IsHealthy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// 检查健康状态和最后成功时间
	if !w.healthy {
		return false
	}

	// 如果超过5分钟没有成功的操作，认为可能不健康
	if time.Since(w.lastSuccess) > 5*time.Minute {
		return false
	}

	return true
}

func (w *ClientWrapper) recordSuccess() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.successCount++
	w.lastSuccess = time.Now()
	w.healthy = true
	w.retryCount = 0
}

func (w *ClientWrapper) recordError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.errorCount++
	w.lastError = err
	w.retryCount++
}

// GetStatistics 获取客户端统计信息
func (w *ClientWrapper) GetStatistics() ClientStatistics {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return ClientStatistics{
		Agent:        w.agent,
		Healthy:      w.healthy,
		SuccessCount: w.successCount,
		ErrorCount:   w.errorCount,
		LastSuccess:  w.lastSuccess,
		LastCheck:    w.lastCheck,
		LastError:    w.lastError,
		CreateTime:   w.createTime,
		RetryCount:   w.retryCount,
	}
}

type ClientStatistics struct {
	Agent        string
	Healthy      bool
	SuccessCount uint64
	ErrorCount   uint64
	LastSuccess  time.Time
	LastCheck    time.Time
	LastError    error
	CreateTime   time.Time
	RetryCount   int
}

// Close 关闭所有连接并停止健康检查
func (m *SNMPClientManager) Close() {
	// 停止健康检查
	if m.healthCheckRunning {
		close(m.stopHealthCheck)
		m.healthCheckRunning = false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for agent, wrapper := range m.clients {
		if wrapper.client != nil && wrapper.client.Conn != nil {
			wrapper.client.Conn.Close()
			log.Printf("Closed connection for agent %s", agent)
		}
		delete(m.clients, agent)
	}
}

// ForceHealthCheck 强制执行一次健康检查
func (m *SNMPClientManager) ForceHealthCheck() {
	log.Println("Forcing health check on all clients")
	go m.performAllHealthChecks()
}

// snmp_client.go - 补充 createClient 方法

func (m *SNMPClientManager) createClient(agent string) (*gosnmp.GoSNMP, error) {
	// 查找对应的agent配置
	var agentConfig *AgentConfig
	for i := range m.config.Agents {
		// Use a pointer to the element in the slice to avoid copying
		ac := &m.config.Agents[i]
		if m.config.GetAgentAddress(*ac) == agent {
			agentConfig = ac
			break
		}
	}

	if agentConfig == nil {
		transport := "udp"
		addrPart := agent
		if strings.Contains(agent, "://") {
			parts := strings.SplitN(agent, "://", 2)
			if len(parts) == 2 && parts[0] != "" {
				transport = parts[0]
				addrPart = parts[1]
			}
		}

		// 如果没有找到配置，尝试解析agent字符串并使用默认配置
		host, portStr, err := net.SplitHostPort(addrPart)
		if err != nil {
			// 没有端口，使用默认端口
			host = addrPart
			portStr = strconv.Itoa(m.config.Port)
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port '%s': %w", portStr, err)
		}

		agentConfig = &AgentConfig{
			Host:          host,
			Port:          port,
			Transport:     transport,
			Community:     m.config.Community,
			Version:       m.config.Version,
			Username:      m.config.Username,
			AuthPassword:  m.config.AuthPassword,
			AuthProtocol:  m.config.AuthProtocol,
			PrivPassword:  m.config.PrivPassword,
			PrivProtocol:  m.config.PrivProtocol,
			SecurityLevel: m.config.SecurityLevel,
			ContextName:   m.config.ContextName,
		}
	}

	// 转换版本号
	var agVersion gosnmp.SnmpVersion
	switch agentConfig.Version {
	case 3:
		agVersion = gosnmp.Version3
	case 2, 0:
		agVersion = gosnmp.Version2c
	case 1:
		agVersion = gosnmp.Version1
	default:
		return nil, fmt.Errorf("invalid SNMP version: %d", agentConfig.Version)
	}

	// 创建SNMP客户端
	client := &gosnmp.GoSNMP{
		Target:                  agentConfig.Host,
		Port:                    uint16(agentConfig.Port),
		Community:               agentConfig.Community,
		Version:                 agVersion,
		Timeout:                 m.config.Timeout,
		Retries:                 m.config.Retries,
		MaxRepetitions:          uint32(m.config.MaxRepetitions),
		MaxOids:                 gosnmp.MaxOids, // 默认值
		Transport:               agentConfig.Transport,
		UseUnconnectedUDPSocket: m.config.UnconnectedUDPSocket,
	}
	if m.config.DebugMode {
		client.Logger = gosnmp.NewLogger(log.New(log.Writer(), "", 0))
	}

	if agentConfig.Version == 3 {
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = gosnmp.NoAuthNoPriv

		if agentConfig.Username != "" {
			client.SecurityParameters = &gosnmp.UsmSecurityParameters{
				UserName: agentConfig.Username,
			}

			switch agentConfig.SecurityLevel {
			case "noAuthNoPriv":
				client.MsgFlags = gosnmp.NoAuthNoPriv
			case "authNoPriv":
				client.MsgFlags = gosnmp.AuthNoPriv
				if err := m.setAuthProtocol(client, agentConfig); err != nil {
					return nil, err
				}
			case "authPriv":
				client.MsgFlags = gosnmp.AuthPriv
				if err := m.setAuthProtocol(client, agentConfig); err != nil {
					return nil, err
				}
				if err := m.setPrivProtocol(client, agentConfig); err != nil {
					return nil, err
				}
			default:
				// 默认为 noAuthNoPriv
				client.MsgFlags = gosnmp.NoAuthNoPriv
			}
		}

		if agentConfig.ContextName != "" {
			client.ContextName = agentConfig.ContextName
		}
	}

	// 连接到SNMP代理
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to SNMP agent %s:%d: %w",
			agentConfig.Host, agentConfig.Port, err)
	}

	log.Printf("Successfully created SNMP client for %s://%s:%d (version: %d)",
		agentConfig.Transport, agentConfig.Host, agentConfig.Port, agentConfig.Version)

	return client, nil
}

func (m *SNMPClientManager) setAuthProtocol(client *gosnmp.GoSNMP, config *AgentConfig) error {
	params, ok := client.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return fmt.Errorf("invalid security parameters type")
	}

	params.AuthenticationPassphrase = config.AuthPassword

	switch strings.ToUpper(config.AuthProtocol) {
	case "MD5", "":
		params.AuthenticationProtocol = gosnmp.MD5
	case "SHA", "SHA1":
		params.AuthenticationProtocol = gosnmp.SHA
	case "SHA224":
		params.AuthenticationProtocol = gosnmp.SHA224
	case "SHA256":
		params.AuthenticationProtocol = gosnmp.SHA256
	case "SHA384":
		params.AuthenticationProtocol = gosnmp.SHA384
	case "SHA512":
		params.AuthenticationProtocol = gosnmp.SHA512
	default:
		return fmt.Errorf("unsupported auth protocol: %s", config.AuthProtocol)
	}

	log.Printf("Set auth protocol to %s for user %s", config.AuthProtocol, config.Username)
	return nil
}

func (m *SNMPClientManager) setPrivProtocol(client *gosnmp.GoSNMP, config *AgentConfig) error {
	params, ok := client.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return fmt.Errorf("invalid security parameters type")
	}

	params.PrivacyPassphrase = config.PrivPassword

	switch strings.ToUpper(config.PrivProtocol) {
	case "DES", "":
		params.PrivacyProtocol = gosnmp.DES
	case "AES", "AES128":
		params.PrivacyProtocol = gosnmp.AES
	case "AES192":
		params.PrivacyProtocol = gosnmp.AES192
	case "AES256":
		params.PrivacyProtocol = gosnmp.AES256
	case "AES192C":
		params.PrivacyProtocol = gosnmp.AES192C
	case "AES256C":
		params.PrivacyProtocol = gosnmp.AES256C
	default:
		return fmt.Errorf("unsupported priv protocol: %s", config.PrivProtocol)
	}

	log.Printf("Set privacy protocol to %s for user %s", config.PrivProtocol, config.Username)
	return nil
}

func getHostFromAgentStr(agentStr string) string {
	hostPort := agentStr
	if idx := strings.Index(agentStr, "://"); idx != -1 {
		hostPort = agentStr[idx+3:]
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort
	}
	return host
}

func (m *SNMPClientManager) GetHealthReport() HealthReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := HealthReport{
		TotalClients:     len(m.clients),
		HealthyClients:   0,
		UnhealthyClients: 0,
		ClientDetails:    make([]ClientHealthDetail, 0, len(m.clients)),
		CheckTime:        time.Now(),
	}

	for agent, wrapper := range m.clients {
		isHealthy := wrapper.IsHealthy()
		if isHealthy {
			report.HealthyClients++
		} else {
			report.UnhealthyClients++
		}

		detail := ClientHealthDetail{
			Agent:        agent,
			Healthy:      isHealthy,
			LastCheck:    wrapper.lastCheck,
			LastSuccess:  wrapper.lastSuccess,
			RetryCount:   wrapper.retryCount,
			ErrorCount:   wrapper.errorCount,
			SuccessCount: wrapper.successCount,
		}

		if wrapper.lastError != nil {
			detail.LastError = wrapper.lastError.Error()
		}

		report.ClientDetails = append(report.ClientDetails, detail)
	}

	return report
}

type HealthReport struct {
	TotalClients     int
	HealthyClients   int
	UnhealthyClients int
	ClientDetails    []ClientHealthDetail
	CheckTime        time.Time
}

type ClientHealthDetail struct {
	Agent        string
	Healthy      bool
	LastCheck    time.Time
	LastSuccess  time.Time
	LastError    string
	RetryCount   int
	ErrorCount   uint64
	SuccessCount uint64
}