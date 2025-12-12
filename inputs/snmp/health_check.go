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
		ticker := time.NewTicker(time.Duration(ins.HealthCheckInterval))
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
	val, exists := ins.targetStatus.Load(agent)
	if !exists {
		newStatus := &TargetStatus{
			healthy:  true,
			lastSeen: time.Now(),
		}
		val, _ = ins.targetStatus.LoadOrStore(agent, newStatus)
	}
	status := val.(*TargetStatus)

	// Don't check too frequently if already marked as unhealthy
	status.mu.RLock()
	healthy := status.healthy
	timeSinceLastCheck := time.Since(status.lastSeen)
	status.mu.RUnlock()

	if !healthy && timeSinceLastCheck < time.Duration(ins.RecoveryInterval) {
		return
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
	val, exists := ins.targetStatus.Load(agent)
	if !exists {
		newStatus := &TargetStatus{
			healthy:   false,
			lastSeen:  time.Now(),
			failCount: ins.MaxFailCount,
		}
		var loaded bool
		val, loaded = ins.targetStatus.LoadOrStore(agent, newStatus)
		if !loaded {
			return
		}
	}

	// Existing status found, update it
	status := val.(*TargetStatus)
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
	val, exists := ins.targetStatus.Load(agent)
	if !exists {
		return true // Default to considering it healthy if no status exists
	}

	// Existing status found, update it
	status := val.(*TargetStatus)
	status.mu.RLock()
	defer status.mu.RUnlock()
	return status.healthy
}
