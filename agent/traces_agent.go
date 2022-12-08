//go:build !no_traces

package agent

import (
	"context"
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/traces"
)

type TracesAgent struct {
	TraceCollector *traces.Collector
}

func NewTracesAgent() AgentModule {
	if config.Config.Traces == nil || !config.Config.Traces.Enable {
		log.Println("I! traces agent disabled!")
		return nil
	}
	col, err := traces.New(config.Config.Traces)
	if err != nil {
		log.Println("E! failed to create traces agent:", err)
		return nil
	}
	if col == nil {
		log.Println("E! failed to create traces agent, collector is nil")
		return nil
	}
	return &TracesAgent{
		TraceCollector: col,
	}
}

func (ta *TracesAgent) Start() (err error) {
	return ta.TraceCollector.Run(context.Background())
}

func (ta *TracesAgent) Stop() (err error) {
	return ta.TraceCollector.Shutdown(context.Background())
}
