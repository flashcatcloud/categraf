package agent

import (
	"errors"
	"log"
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
			NewTracesAgent(),
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
	log.Println("I! agent starting")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Start(); err != nil {
			log.Printf("E! start [%T] err: [%+v]", agent, err)
		} else {
			log.Printf("I! [%T] started", agent)
		}
	}
	log.Println("I! agent started")
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Stop(); err != nil {
			log.Printf("E! stop [%T] err: [%+v]", agent, err)
		} else {
			log.Printf("I! [%T] stopped", agent)
		}
	}
	log.Println("I! agent stopped")
}

func (a *Agent) Reload() {
	log.Println("I! agent reloading")
	a.Stop()
	a.Start()
	log.Println("I! agent reloaded")
}
