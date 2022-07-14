package agent

import (
	"log"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/prometheus"
)

func (a *Agent) startPrometheusScrape() {
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		log.Println("I! prometheus scraping disabled!")
		return
	}
	go prometheus.Start()
	log.Println("I! prometheus scraping started!")
}

func (a *Agent) stopPrometheusScrape() {
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		return
	}
	prometheus.Stop()
	log.Println("I! prometheus scraping stopped!")
}
