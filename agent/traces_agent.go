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
	return &TracesAgent{}
}

func (ta *TracesAgent) Start() (err error) {
	if config.Config.Traces == nil || !config.Config.Traces.Enable {
		return nil
	}

	defer func() {
		if err != nil {
			log.Println("E! failed to start tracing agent:", err)
		}
	}()

	col, err := traces.New(config.Config.Traces)
	if err != nil {
		return err
	}

	err = col.Run(context.Background())
	if err != nil {
		return err
	}

	ta.TraceCollector = col
	return nil
}

func (ta *TracesAgent) Stop() (err error) {
	if config.Config.Traces == nil || !config.Config.Traces.Enable {
		return nil
	}

	if ta.TraceCollector == nil {
		return nil
	}

	defer func() {
		if err != nil {
			log.Println("E! failed to stop tracing agent:", err)
		}
	}()

	return ta.TraceCollector.Shutdown(context.Background())
}
