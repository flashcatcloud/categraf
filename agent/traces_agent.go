package agent

import (
	"context"
	"log"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/traces"
)

func (a *Agent) startTracesAgent() (err error) {
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

	a.TraceCollector = col
	return nil
}

func (a *Agent) stopTracesAgent() (err error) {
	if config.Config.Traces == nil || !config.Config.Traces.Enable {
		return nil
	}

	if a.TraceCollector == nil {
		return nil
	}

	defer func() {
		if err != nil {
			log.Println("E! failed to stop tracing agent:", err)
		}
	}()

	return a.TraceCollector.Shutdown(context.Background())
}
