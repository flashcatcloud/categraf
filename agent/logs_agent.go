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

	"flashcat.cloud/categraf/logs/auditor"
	"flashcat.cloud/categraf/logs/client"
	"flashcat.cloud/categraf/logs/diagnostic"
	"flashcat.cloud/categraf/logs/input/docker"
	"flashcat.cloud/categraf/logs/input/file"
	"flashcat.cloud/categraf/logs/input/journald"
	"flashcat.cloud/categraf/logs/input/listener"
	"flashcat.cloud/categraf/logs/pipeline"
	"flashcat.cloud/categraf/logs/restart"
	"flashcat.cloud/categraf/logs/status"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	logService "flashcat.cloud/categraf/logs/service"
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
func NewLogAgent(sources *logsconfig.LogSources, services *logService.Services, processingRules []*logsconfig.ProcessingRule, endpoints *logsconfig.Endpoints) *LogAgent {
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

	containerLaunchables := []docker.Launchable{
		{
			IsAvailable: docker.IsAvailable,
			Launcher: func() restart.Restartable {
				return docker.NewDockerLauncher(
					// time.Duration(coreConfig.Datadog.GetInt("logs_config.docker_client_read_timeout"))*time.Second,
					time.Duration(30)*time.Second,
					sources,
					services,
					pipelineProvider,
					auditor,
					// TODO
					true,
					false)
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
		inputs = append(inputs, docker.NewLauncher(containerLaunchables))
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
	if coreconfig.Config == nil ||
		!coreconfig.Config.Logs.Enable ||
		(len(coreconfig.Config.Logs.Items) == 0 &&
			!coreconfig.Config.Logs.CollectContainerAll) {
		log.Println("there is no logs config, logs agent quiting")
		return
	}

	endpoints, err := BuildEndpoints(intakeTrackType, AgentJSONIntakeProtocol, logsconfig.DefaultIntakeOrigin)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError("invalid endpoints", message)
		log.Println("E!", errors.New(message))
		return
	}
	processingRules, err := GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		log.Println("E!", errors.New(message))
		return
	}

	sources := logsconfig.NewLogSources()
	services := logService.NewServices()
	log.Println("I! Starting logs-agent...")
	logAgent = NewLogAgent(sources, services, processingRules, endpoints)
	logAgent.Start()

	if coreconfig.GetContainerCollectAll() {
		// collect container all
		if coreconfig.Config.DebugMode {
			log.Println("Adding ContainerCollectAll source to the Logs Agent")
		}
		dockersource := logsconfig.NewLogSource(coreconfig.CollectContainerAll,
			&logsconfig.LogsConfig{
				Type:    coreconfig.Docker,
				Service: "docker",
				Source:  "docker",
			})
		// dockersource.SetSourceType(logsconfig.DockerSourceType)
		sources.AddSource(dockersource)
	}

	log.Println("DEBUG docker source add done, add file source")

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
		sources.AddSource(source)
	}
}

func (a *Agent) stopLogAgent() {
	if logAgent != nil {
		logAgent.Stop()
	}
}

func GetContainerColloectAll() bool {
	return false
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
