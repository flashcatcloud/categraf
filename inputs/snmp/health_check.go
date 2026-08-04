package snmp

import (
	"fmt"
	"log"
	"time"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) StartHealthMonitor() {
	// Recovery-only health checks are driven by Gather. Keep this method as a
	// compatibility no-op for callers and tests that still invoke it.
	ins.healthMonitorStartOnce.Do(func() {})
}

func (ins *Instance) runtime(i int) *agentRuntime {
	if i >= 0 && i < len(ins.agentRuntimes) && ins.agentRuntimes[i] != nil {
		return ins.agentRuntimes[i]
	}
	return newAgentRuntime(time.Now())
}

func (ins *Instance) prepareAgentForGather(slist *types.SampleList, i int, agent string, topTags map[string]string) (bool, *float64) {
	rt := ins.runtime(i)
	now := time.Now()
	if rt.shouldCollect(now) {
		return true, nil
	}

	if !rt.recoveryDue(now) {
		log.Printf("Skipping unhealthy agent %s during collection", agent)
		down := float64(0)
		ins.up(slist, i, topTags, &down)
		return false, nil
	}

	if !rt.beginRecovery(now) {
		log.Printf("Skipping unhealthy agent %s during recovery probe", agent)
		down := float64(0)
		ins.up(slist, i, topTags, &down)
		return false, nil
	}

	if err := ins.recoveryProbe(agent, rt); err != nil {
		log.Printf("Recovery probe: agent %s failed: %s", agent, err)
		ins.pushRecoveryProbeStats(slist, rt, agent, "failure")
		rt.finishRecovery(false, time.Duration(ins.RecoveryInterval))
		down := float64(0)
		ins.up(slist, i, topTags, &down)
		return false, nil
	}

	ins.pushRecoveryProbeStats(slist, rt, agent, "success")
	rt.finishRecovery(true, time.Duration(ins.RecoveryInterval))
	if old := rt.detachConnection(); old != nil {
		_ = old.Close()
	}
	log.Printf("Agent %s recovered and marked healthy", agent)
	up := float64(1)
	return true, &up
}

func (ins *Instance) recoveryProbe(agent string, rt *agentRuntime) error {
	if !rt.beginRequest() {
		return fmt.Errorf("agent runtime is closed")
	}
	defer rt.endRequest()

	clientConfig := ins.ClientConfig
	clientConfig.Timeout = coreconfig.Duration(ins.HealthCheckTimeout)

	gs, err := NewWrapper(clientConfig)
	if err != nil {
		return fmt.Errorf("connection creation error: %w", err)
	}
	gs.setStats(&rt.stats)
	if err := gs.SetAgent(agent); err != nil {
		return fmt.Errorf("set agent error: %w", err)
	}
	if !rt.storeProbeConnection(gs) {
		return fmt.Errorf("agent runtime is closed")
	}
	defer rt.clearProbeConnection(gs)
	if err := gs.Connect(); err != nil {
		_ = gs.Close()
		return fmt.Errorf("connection error: %w", err)
	}
	defer func() {
		_ = gs.Close()
	}()

	rt.requestMu.Lock()
	before := rt.stats.snapshot()
	pkt, err := gs.Get([]string{".1.3.6.1.2.1.1.1.0"})
	after := rt.stats.snapshot()
	rt.requestMu.Unlock()
	responsesObserved := after.finishes - before.finishes
	if pkt != nil && responsesObserved == 0 {
		responsesObserved = 1
	}
	rt.recordOperation("probe", after.sent-before.sent, after.recv-before.recv, responsesObserved, err)
	return err
}
