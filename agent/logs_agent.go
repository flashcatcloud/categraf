//go:build !no_logs

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
	"os"
	"time"

	"flashcat.cloud/categraf/logs/auditor"
	"flashcat.cloud/categraf/logs/client"
	"flashcat.cloud/categraf/logs/diagnostic"
	"flashcat.cloud/categraf/logs/input/container"
	"flashcat.cloud/categraf/logs/input/file"
	"flashcat.cloud/categraf/logs/input/journald"
	"flashcat.cloud/categraf/logs/input/kubernetes"
	"flashcat.cloud/categraf/logs/input/listener"
	"flashcat.cloud/categraf/logs/pipeline"
	"flashcat.cloud/categraf/logs/restart"
	"flashcat.cloud/categraf/logs/status"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	logService "flashcat.cloud/categraf/logs/service"
)

const (
	intakeTrackType         = "logs"
	AgentJSONIntakeProtocol = "agent-json"
	invalidProcessingRules  = "invalid_global_processing_rules"
)

// LogsAgent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend
// + ------------------------------------------------------ +
// |                                                        |
// | Collector -> Decoder -> Processor -> Sender -> Auditor |
// |                                                        |
// + ------------------------------------------------------ +
type LogsAgent struct {
	sources         *logsconfig.LogSources
	services        *logService.Services
	processingRules []*logsconfig.ProcessingRule
	endpoints       *logsconfig.Endpoints

	auditor                   auditor.Auditor
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	inputs                    []restart.Restartable
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
}

// NewLogsAgent returns a new Logs LogsAgent
func NewLogsAgent() AgentModule {
	if coreconfig.Config == nil ||
		!coreconfig.Config.Logs.Enable ||
		(len(coreconfig.Config.Logs.Items) == 0 && coreconfig.Config.Logs.CollectContainerAll == false) {
		return nil
	}

	endpoints, err := BuildEndpoints(intakeTrackType, AgentJSONIntakeProtocol, logsconfig.DefaultIntakeOrigin)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError("invalid endpoints", message)
		log.Println("E!", errors.New(message))
		return nil
	}
	processingRules, err := GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		log.Println("E!", errors.New(message))
		return nil
	}

	sources := logsconfig.NewLogSources()
	services := logService.NewServices()
	log.Println("I! Starting logs-agent...")

	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(23) * time.Hour
	_, err = os.Stat(coreconfig.GetLogRunPath())
	if os.IsNotExist(err) {
		os.MkdirAll(coreconfig.GetLogRunPath(), 0755)
	}
	auditor := auditor.New(coreconfig.GetLogRunPath(), auditor.DefaultRegistryFilename, auditorTTL)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(logsconfig.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

	validatePodContainerID := coreconfig.ValidatePodContainerID()
	//
	containerLaunchables := []container.Launchable{
		{
			IsAvailable: kubernetes.IsAvailable,
			Launcher: func() restart.Restartable {
				return kubernetes.NewLauncher(sources, services, coreconfig.GetContainerCollectAll())
			},
		},
	}

	// setup the inputs
	inputs := []restart.Restartable{
		file.NewScanner(sources, coreconfig.OpenLogsLimit(), pipelineProvider, auditor,
			file.DefaultSleepDuration, validatePodContainerID, time.Duration(time.Duration(coreconfig.FileScanPeriod())*time.Second)),
		listener.NewLauncher(sources, coreconfig.LogFrameSize(), pipelineProvider),
		journald.NewLauncher(sources, pipelineProvider, auditor),
	}
	if coreconfig.GetContainerCollectAll() {
		log.Println("collect docker logs...")
		inputs = append(inputs, container.NewLauncher(containerLaunchables))
	}

	return &LogsAgent{
		sources:                   sources,
		services:                  services,
		processingRules:           processingRules,
		endpoints:                 endpoints,
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		inputs:                    inputs,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

func (la *LogsAgent) Start() error {
	la.startInner()
	if coreconfig.GetContainerCollectAll() {
		// collect container all
		if coreconfig.Config.DebugMode {
			log.Println("Adding ContainerCollectAll source to the Logs Agent")
		}
		kubesource := logsconfig.NewLogSource(logsconfig.ContainerCollectAll,
			&logsconfig.LogsConfig{
				Type:    coreconfig.Kubernetes,
				Service: "docker",
				Source:  "docker",
			})
		la.sources.AddSource(kubesource)
		go kubernetes.NewScanner(la.services).Scan()
	}

	// add source
	for _, c := range coreconfig.Config.Logs.Items {
		if c == nil {
			continue
		}
		source := logsconfig.NewLogSource(c.Name, c)
		if err := c.Validate(); err != nil {
			log.Println("W! Invalid logs configuration:", err)
			source.Status.Error(err)
			continue
		}
		la.sources.AddSource(source)
	}
	return nil
}

// startInner starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *LogsAgent) startInner() {
	starter := restart.NewStarter(a.destinationsCtx, a.auditor, a.pipelineProvider, a.diagnosticMessageReceiver)
	for _, input := range a.inputs {
		starter.Add(input)
	}
	starter.Start()
}

// Flush flushes synchronously the pipelines managed by the Logs LogsAgent.
func (a *LogsAgent) Flush(ctx context.Context) {
	a.pipelineProvider.Flush(ctx)
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *LogsAgent) Stop() error {
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
			log.Println("W! Force close of the Logs LogsAgent, dumping the Go routines.")
		}
	}
	return nil
}

// GlobalProcessingRules returns the global processing rules to apply to all logs.
func GlobalProcessingRules() ([]*logsconfig.ProcessingRule, error) {
	rules := coreconfig.Config.Logs.GlobalProcessingRules
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
