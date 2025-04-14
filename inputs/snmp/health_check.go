package snmp

import (
	"log"
	"time"

	coreconfig "flashcat.cloud/categraf/config"
)

func (ins *Instance) StartHealthMonitor() {
	if ins.healthMonitorStarted {
		return
	}

	ins.healthMonitorStarted = true

	go func() {
		ticker := time.NewTicker(ins.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				for i, agent := range ins.Agents {
					go ins.checkAgentHealth(i, agent)
				}
			case <-ins.stop:
				return
			}
		}

	}()
}

func (ins *Instance) checkAgentHealth(i int, agent string) {
	status, exists := ins.targetStatus[agent]
	if !exists {
		ins.targetStatus[agent] = &TargetStatus{healthy: true, lastSeen: time.Now()}
		status = ins.targetStatus[agent]
	}

	// Don't check too frequently if already marked as unhealthy
	if !status.healthy {
		status.mu.RLock()
		timeSinceLastCheck := time.Since(status.lastSeen)
		status.mu.RUnlock()

		if timeSinceLastCheck < ins.RecoveryInterval {
			return
		}
	}

	// Create a connection with shorter timeout for health checking
	clientConfig := ins.ClientConfig
	clientConfig.Timeout = coreconfig.Duration(ins.HealthCheckTimeout)

	gs, err := NewWrapper(clientConfig)
	if err != nil {
		log.Printf("Health check: agent %s connection creation error: %s", agent, err)
		ins.markAgentUnhealthy(agent)
		return
	}

	err = gs.SetAgent(agent)
	if err != nil {
		log.Printf("Health check: agent %s set agent error: %s", agent, err)
		ins.markAgentUnhealthy(agent)
		return
	}

	if err := gs.Connect(); err != nil {
		log.Printf("Health check: agent %s connection error: %s", agent, err)
		ins.markAgentUnhealthy(agent)
		return
	}

	defer gs.Conn.Close()

	// Try a simple get of sysDescr OID to test connectivity
	oid := ".1.3.6.1.2.1.1.1.0"
	_, err = gs.Get([]string{oid})

	status.mu.Lock()
	defer status.mu.Unlock()

	if err != nil {
		// If already marked unhealthy, increment fail count
		if status.healthy {
			status.failCount = 1
		} else {
			status.failCount++
		}

		// Mark as unhealthy after reaching max fail count
		if status.failCount >= ins.MaxFailCount {
			if status.healthy {
				log.Printf("Agent %s marked as unhealthy after %d consecutive failures", agent, status.failCount)
				status.healthy = false
			}
		}
	} else {
		// If it was unhealthy before, log recovery
		if !status.healthy {
			log.Printf("Agent %s recovered and marked healthy", agent)
		}
		status.healthy = true
		status.failCount = 0
	}

	status.lastSeen = time.Now()
}

func (ins *Instance) markAgentUnhealthy(agent string) {
	status, exists := ins.targetStatus[agent]
	if !exists {
		ins.targetStatus[agent] = &TargetStatus{healthy: false, lastSeen: time.Now(), failCount: ins.MaxFailCount}
		return
	}

	status.mu.Lock()
	defer status.mu.Unlock()

	status.failCount++
	if status.failCount >= ins.MaxFailCount {
		if status.healthy {
			log.Printf("Agent %s marked as unhealthy after %d consecutive failures", agent, status.failCount)
			status.healthy = false
		}
	}
	status.lastSeen = time.Now()
}

func (ins *Instance) isAgentHealthy(agent string) bool {
	status, exists := ins.targetStatus[agent]
	if !exists {
		return true // Default to considering it healthy if no status exists
	}

	status.mu.RLock()
	defer status.mu.RUnlock()
	return status.healthy
}
