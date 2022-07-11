package agent

import (
	coreconfig "flashcat.cloud/categraf/config"

	"flashcat.cloud/categraf/prometheus"
)

func (a *Agent) startPrometheusScrape() {
	if coreconfig.Config == nil ||
		!coreconfig.Config.Prometheus.Enable {
		return
	}
	go prometheus.Start()
}

func (a *Agent) stopPrometheusScrape() {
	if coreconfig.Config == nil ||
		!coreconfig.Config.Prometheus.Enable {
		return
	}
	prometheus.Stop()
}
