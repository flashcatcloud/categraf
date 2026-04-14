package agent

import (
	"errors"

	"k8s.io/klog/v2"
)

type Agent struct {
	agents []AgentModule
}

// AgentModule is the interface for agent modules
// Use NewXXXAgent() to create a new agent module
// if the agent module is not needed, return nil
type AgentModule interface {
	Start() error
	Stop() error
}

func NewAgent() (*Agent, error) {
	agent := &Agent{
		agents: []AgentModule{
			NewMetricsAgent(),
			NewLogsAgent(),
			NewPrometheusAgent(),
			NewIbexAgent(),
		},
	}
	for _, ag := range agent.agents {
		if ag != nil {
			return agent, nil
		}
	}
	return nil, errors.New("no valid running agents, please check configuration")
}

func (a *Agent) Start() {
	klog.InfoS("agent starting")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Start(); err != nil {
			klog.ErrorS(err, "start agent module failed", "module", agent)
		} else {
			klog.InfoS("agent module started", "module", agent)
		}
	}
	klog.InfoS("agent started")
}

func (a *Agent) Stop() {
	klog.InfoS("agent stopping")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Stop(); err != nil {
			klog.ErrorS(err, "stop agent module failed", "module", agent)
		} else {
			klog.InfoS("agent module stopped", "module", agent)
		}
	}
	klog.InfoS("agent stopped")
}

func (a *Agent) Reload() {
	klog.InfoS("agent reloading")
	a.Stop()
	a.Start()
	klog.InfoS("agent reloaded")
}
