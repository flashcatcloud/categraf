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
	return &IbexAgent{}
}

func (ia *IbexAgent) Start() error {
	if coreconfig.Config == nil ||
		coreconfig.Config.Ibex == nil ||
		!coreconfig.Config.Ibex.Enable {
		log.Println("I! ibex agent disabled!")
		return nil
	}

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
