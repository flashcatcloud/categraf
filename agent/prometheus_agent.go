//go:build !no_prometheus

package agent

import (
	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/prometheus"
	"k8s.io/klog/v2"
)

type PrometheusAgent struct {
}

func NewPrometheusAgent() AgentModule {
	if coreconfig.Config == nil ||
		coreconfig.Config.Prometheus == nil ||
		!coreconfig.Config.Prometheus.Enable {
		klog.Info("prometheus scraping disabled")
		return nil
	}
	return &PrometheusAgent{}
}

func (pa *PrometheusAgent) Start() error {
	go prometheus.Start()
	klog.Info("prometheus scraping started")
	return nil
}

func (pa *PrometheusAgent) Stop() error {
	prometheus.Stop()
	klog.Info("prometheus scraping stopped")
	return nil
}
