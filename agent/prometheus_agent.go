//go:build !no_prometheus

package agent

import (
	"log"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/prometheus"
)

type PrometheusAgent struct {
}

func NewPrometheusAgent() AgentModule {
	return &PrometheusAgent{}
}

func (pa *PrometheusAgent) Start() error {
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		log.Println("I! prometheus scraping disabled!")
		return nil
	}
	go prometheus.Start()
	log.Println("I! prometheus scraping started!")
	return nil
}

func (pa *PrometheusAgent) Stop() error {
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		return nil
	}
	prometheus.Stop()
	log.Println("I! prometheus scraping stopped!")
	return nil
}
