//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"flashcat.cloud/categraf/inputs"
	"fmt"
	"github.com/BurntSushi/toml"
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
	"flashcat.cloud/categraf/logs/util"

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

// curSourceMap is a map that holds current log sources configuration.
var curSourceMap = make(map[string]*logsconfig.LogSource)

// NewLogsAgent returns a new Logs LogsAgent
func NewLogsAgent() AgentModule {
	if coreconfig.Config == nil ||
		!coreconfig.Config.Logs.Enable ||
		(len(coreconfig.Config.Logs.Items) == 0 && coreconfig.Config.Logs.EnableCollectContainer == false) {
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
	pipelineProvider := pipeline.NewProvider(coreconfig.NumberOfPipelines(), auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

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
	if coreconfig.EnableCollectContainer() {
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
	if coreconfig.EnableCollectContainer() {
		// collect container all
		if util.Debug() {
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

	// 开启一个goroutine 用来接收从远端Http请求拉取的结果
	go la.handleHttpProviderResCh()

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

// handleHttpProviderResCh listens for HTTP provider responses and updates the sources accordingly.
func (a *LogsAgent) handleHttpProviderResCh() {
	for {
		select {
		case confResp := <-inputs.HttpProviderResponseCh:
			// Delay 2s to prioritize processing of local logs.toml
			time.Sleep(2 * time.Second)

			newSourceMap := make(map[string]*logsconfig.LogSource)
			for k := range confResp.Configs {

				// Retain only the content related to logs plugins, ignore other data
				if k != "logs" {
					continue
				}
				for kk, vv := range confResp.Configs[k] {
					if vv != nil {
						configData := (*vv).Config
						configFormat := (*vv).Format
						log.Printf("I! HttpProviderResponseCh k: %s, kk: %+v, config: %s, format: %+v\n",
							k, kk, configData, configFormat)
						logs := &coreconfig.Logs{}
						err := toml.Unmarshal([]byte(configData), logs)
						if err != nil {
							log.Println("E! configData parse toml err:", err)
						}
						for _, c := range logs.Items {

							if c == nil {
								continue
							}
							if _, exists := curSourceMap[a.itemIdentify(c)]; !exists {
								source := logsconfig.NewLogSource(c.Name, c)
								newSourceMap[a.itemIdentify(c)] = source
							} else {
								newSourceMap[a.itemIdentify(c)] = curSourceMap[a.itemIdentify(c)]
							}
						}
					} else {
						log.Printf("I! HttpProviderResponseCh: nil pointer for key: %+v\n", kk)
					}
				}
			}

			oldSourceMap := curSourceMap
			log.Printf("I! oldSourceMap: : %+v\n", oldSourceMap)
			log.Printf("I! newSourceMap: : %+v\n", newSourceMap)
			for key, newSource := range newSourceMap {
				// Check if the key exists in oldSourceMap
				if _, ok := oldSourceMap[key]; !ok {
					// If it's in newSource but not in oldSource, add it
					a.sources.AddSource(newSource)
				} else {
					// Remove the processed key-value pair from oldSource to avoid duplicate checking later
					delete(oldSourceMap, key)
				}
			}

			// Any remaining elements in oldSourceMap are not in newSourceMap
			for _, oldSource := range oldSourceMap {
				// If it's in oldSource but not in newSource, remove it
				a.sources.RemoveSource(oldSource)
			}

			curSourceMap = newSourceMap
			log.Printf("I! curSourceMap: : %+v\n", curSourceMap)
			log.Printf("I! GetSources: : %+v\n", a.sources.GetSources())
			log.Printf("I! GetSources len: : %d\n", len(a.sources.GetSources()))
		}
	}
}

// itemIdentify returns a string representing the identification of a logsconfig.LogsConfig item.
func (a *LogsAgent) itemIdentify(item *logsconfig.LogsConfig) string {
	str := item.Type + item.Path + item.Source + item.Service
	//hash := md5.Sum([]byte(str))
	//md5String := hex.EncodeToString(hash[:])
	return str
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
