// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	coreconfig "flashcat.cloud/categraf/config"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/pkg/logs/auditor"
	"flashcat.cloud/categraf/pkg/logs/client"
	"flashcat.cloud/categraf/pkg/logs/client/http"
	"flashcat.cloud/categraf/pkg/logs/diagnostic"
	"flashcat.cloud/categraf/pkg/logs/input/file"
	"flashcat.cloud/categraf/pkg/logs/input/journald"
	"flashcat.cloud/categraf/pkg/logs/input/listener"
	"flashcat.cloud/categraf/pkg/logs/pipeline"
	"flashcat.cloud/categraf/pkg/logs/restart"
	"flashcat.cloud/categraf/pkg/logs/service"
	logService "flashcat.cloud/categraf/pkg/logs/service"
	"flashcat.cloud/categraf/pkg/logs/status"
)

// LogAgent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend
// + ------------------------------------------------------ +
// |                                                        |
// | Collector -> Decoder -> Processor -> Sender -> Auditor |
// |                                                        |
// + ------------------------------------------------------ +
type LogAgent struct {
	auditor                   auditor.Auditor
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	inputs                    []restart.Restartable
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
}

// NewAgent returns a new Logs LogAgent
func NewLogAgent(sources *logsconfig.LogSources, services *service.Services, processingRules []*logsconfig.ProcessingRule, endpoints *logsconfig.Endpoints) *LogAgent {
	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(23) * time.Hour
	auditor := auditor.New(coreconfig.GetLogRunPath(), auditor.DefaultRegistryFilename, auditorTTL)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(logsconfig.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

	validatePodContainerID := coreconfig.ValidatePodContainerID()

	// setup the inputs
	inputs := []restart.Restartable{
		file.NewScanner(sources, coreconfig.OpenLogsLimit(), pipelineProvider, auditor,
			file.DefaultSleepDuration, validatePodContainerID, time.Duration(time.Duration(coreconfig.FileScanPeriod())*time.Second)),
		listener.NewLauncher(sources, coreconfig.LogFrameSize(), pipelineProvider),
		journald.NewLauncher(sources, pipelineProvider, auditor),
	}

	return &LogAgent{
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		inputs:                    inputs,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *LogAgent) Start() {
	starter := restart.NewStarter(a.destinationsCtx, a.auditor, a.pipelineProvider, a.diagnosticMessageReceiver)
	for _, input := range a.inputs {
		starter.Add(input)
	}
	starter.Start()
}

// Flush flushes synchronously the pipelines managed by the Logs LogAgent.
func (a *LogAgent) Flush(ctx context.Context) {
	a.pipelineProvider.Flush(ctx)
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *LogAgent) Stop() {
	inputs := restart.NewParallelStopper()
	for _, input := range a.inputs {
		inputs.Add(input)
	}
	stopper := restart.NewSerialStopper(
		inputs,
		a.pipelineProvider,
		a.auditor,
		a.destinationsCtx,
		a.diagnosticMessageReceiver,
	)

	// This will try to stop everything in order, including the potentially blocking
	// parts like the sender. After StopTimeout it will just stop the last part of the
	// pipeline, disconnecting it from the auditor, to make sure that the pipeline is
	// flushed before stopping.
	// TODO: Add this feature in the stopper.
	c := make(chan struct{})
	go func() {
		stopper.Stop()
		close(c)
	}()
	timeout := time.Duration(30) * time.Second
	select {
	case <-c:
	case <-time.After(timeout):
		log.Println("I! Timed out when stopping logs-agent, forcing it to stop now")
		// We force all destinations to read/flush all the messages they get without
		// trying to write to the network.
		a.destinationsCtx.Stop()
		// Wait again for the stopper to complete.
		// In some situation, the stopper unfortunately never succeed to complete,
		// we've already reached the grace period, give it some more seconds and
		// then force quit.
		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-c:
		case <-timeout.C:
			log.Println("W! Force close of the Logs LogAgent, dumping the Go routines.")
		}
	}
}

var (
	logAgent *LogAgent
)

const (
	intakeTrackType         = "logs"
	AgentJSONIntakeProtocol = "agent-json"
	invalidProcessingRules  = "invalid_global_processing_rules"
)

func (a *Agent) startLogAgent() {
	if !coreconfig.LogConfig.Enable {
		return
	}
	logSources := logsconfig.NewLogSources()
	for _, c := range coreconfig.LogConfig.Items {
		if c == nil {
			continue
		}
		source := logsconfig.NewLogSource(c.Name, c)
		if err := c.Validate(); err != nil {
			log.Println("W! Invalid logs configuration:", err)
			source.Status.Error(err)
			continue
		}
		logSources.AddSource(source)
	}

	if coreconfig.LogConfig != nil && len(coreconfig.LogConfig.Items) == 0 {
		return
	}
	httpConnectivity := logsconfig.HTTPConnectivityFailure
	if endpoints, err := BuildHTTPEndpoints(intakeTrackType, AgentJSONIntakeProtocol, logsconfig.DefaultIntakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	endpoints, err := BuildEndpoints(httpConnectivity, intakeTrackType, AgentJSONIntakeProtocol, logsconfig.DefaultIntakeOrigin)
	processingRules, err := GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		log.Println("E!", errors.New(message))
		return
	}

	services := logService.NewServices()
	log.Println("I! Starting logs-agent...")
	logAgent := NewLogAgent(logSources, services, processingRules, endpoints)
	logAgent.Start()
}

func stopLogAgent() {
	if logAgent != nil {
		logAgent.Stop()
	}
}

func GetContainerColloectAll() bool {
	return false
}

// GlobalProcessingRules returns the global processing rules to apply to all logs.
func GlobalProcessingRules() ([]*logsconfig.ProcessingRule, error) {
	rules := coreconfig.LogConfig.GlobalProcessingRules
	err := logsconfig.ValidateProcessingRules(rules)
	if err != nil {
		return nil, err
	}
	err = logsconfig.CompileProcessingRules(rules)
	if err != nil {
		return nil, err
	}
	return rules, nil
}
