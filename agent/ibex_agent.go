//go:build !no_ibex

package agent

import (
	"log"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex"
)

type IbexAgent struct {
}

func NewIbexAgent() AgentModule {
	if coreconfig.Config == nil ||
		coreconfig.Config.Ibex == nil ||
		!coreconfig.Config.Ibex.Enable {
		log.Println("I! ibex agent disabled!")
		return nil
	}
	if coreconfig.Config.Ibex.MetaDir == "" {
		coreconfig.Config.Ibex.MetaDir = "tasks.d"
	}

	return &IbexAgent{}
}

func (ia *IbexAgent) Start() error {
	go ibex.Start()
	return nil
}

func (ia *IbexAgent) Stop() error {
	if coreconfig.Config == nil ||
		coreconfig.Config.Ibex == nil ||
		!coreconfig.Config.Ibex.Enable {
		return nil
	}
	ibex.Stop()
	return nil
}
