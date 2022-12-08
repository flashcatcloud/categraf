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
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		log.Println("I! prometheus scraping disabled!")
		return nil
	}
	return &PrometheusAgent{}
}

func (pa *PrometheusAgent) Start() error {
	go prometheus.Start()
	log.Println("I! prometheus scraping started!")
	return nil
}

func (pa *PrometheusAgent) Stop() error {
	prometheus.Stop()
	log.Println("I! prometheus scraping stopped!")
	return nil
}
