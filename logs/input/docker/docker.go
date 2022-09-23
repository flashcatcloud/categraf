// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	coreConfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/auditor"
	dockerutil "flashcat.cloud/categraf/logs/input/docker/util"
	"flashcat.cloud/categraf/logs/pipeline"
	"flashcat.cloud/categraf/logs/restart"
	"flashcat.cloud/categraf/logs/service"
	"flashcat.cloud/categraf/pkg/retry"
)

const (
	backoffInitialDuration = 1 * time.Second
	backoffMaxDuration     = 60 * time.Second
)

type sourceInfoPair struct {
	source *logsconfig.LogSource
	info   *logsconfig.MappedInfo
}

// A DockerLauncher starts and stops new tailers for every new containers discovered by autodiscovery.
type DockerLauncher struct {
	pipelineProvider   pipeline.Provider
	addedSources       chan *logsconfig.LogSource
	removedSources     chan *logsconfig.LogSource
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	activeSources      []*logsconfig.LogSource
	pendingContainers  map[string]*Container
	tailers            map[string]*Tailer
	registry           auditor.Registry
	stop               chan struct{}
	erroredContainerID chan string
	lock               *sync.Mutex
	collectAllSource   *logsconfig.LogSource
	collectAllInfo     *logsconfig.MappedInfo
	readTimeout        time.Duration               // client read timeout to set on the created tailer
	serviceNameFunc    func(string, string) string // serviceNameFunc gets the service name from the tagger, it is in a separate field for testing purpose

	forceTailingFromFile   bool                      // will ignore known offset and always tail from file
	tailFromFile           bool                      // If true docker will be tailed from the corresponding log file
	fileSourcesByContainer map[string]sourceInfoPair // Keep track of locally generated sources
	sources                *logsconfig.LogSources    // To schedule file source when taileing container from file
}

// IsAvailable retrues true if the launcher is available and a retrier otherwise
func IsAvailable() (bool, *retry.Retrier) {
	if !coreConfig.IsFeaturePresent(coreConfig.Docker) {
		return false, nil
	}

	util, retrier := dockerutil.GetDockerUtilWithRetrier()
	if util != nil {
		log.Println("Docker launcher is available")
		return true, nil
	}

	return false, retrier
}

// NewLauncher returns a new launcher
func NewDockerLauncher(readTimeout time.Duration, sources *logsconfig.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry, tailFromFile, forceTailingFromFile bool) *DockerLauncher {
	if _, err := dockerutil.GetDockerUtil(); err != nil {
		log.Println("DockerUtil not available, failed to create launcher", err)
		return nil
	}

	launcher := &DockerLauncher{
		pipelineProvider:       pipelineProvider,
		tailers:                make(map[string]*Tailer),
		pendingContainers:      make(map[string]*Container),
		registry:               registry,
		stop:                   make(chan struct{}),
		erroredContainerID:     make(chan string),
		lock:                   &sync.Mutex{},
		readTimeout:            readTimeout,
		serviceNameFunc:        ServiceNameFromTags,
		sources:                sources,
		forceTailingFromFile:   forceTailingFromFile,
		tailFromFile:           tailFromFile,
		fileSourcesByContainer: make(map[string]sourceInfoPair),
		collectAllInfo:         logsconfig.NewMappedInfo("Container Info"),
	}

	if tailFromFile {
		if err := checkReadAccess(); err != nil {
			log.Println("Error accessing %s, %v, falling back on tailing from Docker socket", basePath, err)
			launcher.tailFromFile = false
		}
	}

	// FIXME(achntrl): Find a better way of choosing the right launcher
	// between Docker and Kubernetes
	launcher.addedSources = sources.GetAddedForType(logsconfig.DockerType)
	launcher.removedSources = sources.GetRemovedForType(logsconfig.DockerType)
	launcher.addedServices = services.GetAddedServicesForType(logsconfig.DockerType)
	launcher.removedServices = services.GetRemovedServicesForType(logsconfig.DockerType)
	return launcher
}

// Start starts the DockerLauncher
func (l *DockerLauncher) Start() {
	log.Println("Starting Docker launcher")
	go l.run()
}

// Stop stops the DockerLauncher and its tailers in parallel,
// this call returns only when all the tailers are stopped.
func (l *DockerLauncher) Stop() {
	log.Println("Stopping Docker launcher")
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	l.lock.Lock()
	var containerIDs []string
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		containerIDs = append(containerIDs, tailer.ContainerID)
	}
	l.lock.Unlock()
	for _, containerID := range containerIDs {
		l.removeTailer(containerID)
	}
	stopper.Stop()
}

// run starts and stops new tailers when it receives a new source
// or a new service which is mapped to a container.
func (l *DockerLauncher) run() {
	scanTicker := time.NewTicker(10 * time.Second)
	for {
		select {
		case service := <-l.addedServices:
			log.Printf("DEBUG, docker service add %v", service)
			// detected a new container running on the host,
			dockerutil, err := dockerutil.GetDockerUtil()
			if err != nil {
				log.Println("Could not use docker client, logs for container %s won’t be collected: %v", service.Identifier, err)
				continue
			}
			dockerContainer, err := dockerutil.Inspect(context.TODO(), service.Identifier, false)
			if err != nil {
				log.Println("Could not find container with id: %v", err)
				continue
			}
			log.Printf("DEBUG! dockerutil %v", dockerContainer)
			container := NewContainer(dockerContainer, service)
			source := container.FindSource(l.activeSources)
			switch {
			case source != nil:
				// a source matches with the container, start a new tailer
				l.startTailer(container, source)
			default:
				// no source matches with the container but a matching source may not have been
				// emitted yet or the container may contain an autodiscovery identifier
				// so it's put in a cache until a matching source is found.
				l.pendingContainers[service.Identifier] = container
			}
		case source := <-l.addedSources:
			// detected a new source that has been created either from a configuration file,
			// a docker label or a pod annotation.
			log.Printf("DEBUG! docker source add %v", source)
			l.activeSources = append(l.activeSources, source)
			pendingContainers := make(map[string]*Container)
			for _, container := range l.pendingContainers {
				if container.IsMatch(source) {
					// found a container matching the new source, start a new tailer
					l.startTailer(container, source)
				} else {
					// keep the container in cache until
					pendingContainers[container.service.Identifier] = container
				}
			}
			// keep the containers that have not found any source yet for next iterations
			l.pendingContainers = pendingContainers
		case source := <-l.removedSources:
			log.Printf("DEBUG docker source remove %v", source)
			for i, src := range l.activeSources {
				if src == source {
					// no need to stop any tailer here, it will be stopped after receiving a
					// "remove service" event.
					l.activeSources = append(l.activeSources[:i], l.activeSources[i+1:]...)
					break
				}
			}
		case service := <-l.removedServices:
			log.Printf("DEBUG docker service remove", service)
			// detected that a container has been stopped.
			containerID := service.Identifier
			l.stopTailer(containerID)
			delete(l.pendingContainers, containerID)
		case containerID := <-l.erroredContainerID:
			log.Printf("DEBUG, error container %v ", containerID)
			go l.restartTailer(containerID)

		case <-scanTicker.C:
			// check if there are new files to tail, tailers to stop and tailer to restart because of file rotation
			l.scan()
		case <-l.stop:
			log.Printf("DEBUG, docker launcher stop")
			// no docker container should be tailed anymore
			return
		}
	}
}
func (l *DockerLauncher) scan() {

}

// overrideSource create a new source with the image short name if the source is ContainerCollectAll
func (l *DockerLauncher) overrideSource(container *Container, source *logsconfig.LogSource) *logsconfig.LogSource {
	standardService := l.serviceNameFunc(container.container.Name, dockerutil.ContainerIDToTaggerEntityName(container.container.ID))
	if source.Name != logsconfig.ContainerCollectAll {
		if source.Config.Service == "" && standardService != "" {
			source.Config.Service = standardService
		}
		return source
	}

	if l.collectAllSource == nil {
		l.collectAllSource = source
		l.collectAllSource.RegisterInfo(l.collectAllInfo)
	}

	shortName, err := container.getShortImageName(context.TODO())
	containerID := container.service.Identifier
	if err != nil {
		log.Println("Could not get short image name for container %v: %v", ShortContainerID(containerID), err)
		return source
	}

	l.collectAllInfo.SetMessage(containerID, fmt.Sprintf("Container ID: %s, Image: %s, Created: %s, Tailing from the Docker socket", ShortContainerID(containerID), shortName, container.container.Created))

	newSource := newOverridenSource(standardService, shortName, source.Status)
	newSource.ParentSource = source
	return newSource
}

// getFileSource create a new file source with the image short name if the source is ContainerCollectAll
func (l *DockerLauncher) getFileSource(container *Container, source *logsconfig.LogSource) sourceInfoPair {
	containerID := container.service.Identifier

	// If containerCollectAll is set - we use the global collectAllInfo, otherwise we create a new info for this source
	var sourceInfo *logsconfig.MappedInfo

	// Populate the collectAllSource if we don't have it yet
	if source.Name == logsconfig.ContainerCollectAll && l.collectAllSource == nil {
		l.collectAllSource = source
		l.collectAllSource.RegisterInfo(l.collectAllInfo)
		sourceInfo = l.collectAllInfo
	} else {
		sourceInfo = logsconfig.NewMappedInfo("Container Info")
		source.RegisterInfo(sourceInfo)
	}

	standardService := l.serviceNameFunc(container.container.Name, dockerutil.ContainerIDToTaggerEntityName(containerID))
	shortName, err := container.getShortImageName(context.TODO())
	if err != nil {
		log.Println("Could not get short image name for container %v: %v", ShortContainerID(containerID), err)
	}

	// Update parent source with additional information
	sourceInfo.SetMessage(containerID, fmt.Sprintf("Container ID: %s, Image: %s, Created: %s, Tailing from file: %s", ShortContainerID(containerID), shortName, container.container.Created, l.getPath(containerID)))

	// When ContainerCollectAll is not enabled, we try to derive the service and source names from container labels
	// provided by AD (in this case, the parent source config). Otherwise we use the standard service or short image
	// name for the service name and always use the short image name for the source name.
	var serviceName string
	if source.Name != logsconfig.ContainerCollectAll && source.Config.Service != "" {
		serviceName = source.Config.Service
	} else if standardService != "" {
		serviceName = standardService
	} else {
		serviceName = shortName
	}

	sourceName := shortName
	if source.Name != logsconfig.ContainerCollectAll && source.Config.Source != "" {
		sourceName = source.Config.Source
	}

	// New file source that inherit most of its parent properties
	fileSource := logsconfig.NewLogSource(source.Name, &logsconfig.LogsConfig{
		Type:            logsconfig.FileType,
		Identifier:      containerID,
		Path:            l.getPath(containerID),
		Service:         serviceName,
		Source:          sourceName,
		Tags:            source.Config.Tags,
		ProcessingRules: source.Config.ProcessingRules,
	})
	fileSource.SetSourceType(logsconfig.DockerSourceType)
	fileSource.Status = source.Status
	fileSource.ParentSource = source
	return sourceInfoPair{source: fileSource, info: sourceInfo}
}

// getPath returns the file path of the container log to tail.
// The pattern looks like /var/lib/docker/containers/{container-id}/{container-id}-json.log
func (l *DockerLauncher) getPath(id string) string {
	filename := fmt.Sprintf("%s-json.log", id)
	return filepath.Join(basePath, id, filename)
}

// newOverridenSource is separated from overrideSource for testing purpose
func newOverridenSource(standardService, shortName string, status *logsconfig.LogStatus) *logsconfig.LogSource {
	var serviceName string
	if standardService != "" {
		serviceName = standardService
	} else {
		serviceName = shortName
	}

	overridenSource := logsconfig.NewLogSource(logsconfig.ContainerCollectAll, &logsconfig.LogsConfig{
		Type:    logsconfig.DockerType,
		Service: serviceName,
		Source:  shortName,
	})
	overridenSource.Status = status
	return overridenSource
}

// startTailer starts a new tailer for the container matching with the source.
func (l *DockerLauncher) startTailer(container *Container, source *logsconfig.LogSource) {
	if l.shouldTailFromFile(container) {
		l.scheduleFileSource(container, source)
	} else {
		l.startSocketTailer(container, source)
	}
}

func (l *DockerLauncher) shouldTailFromFile(container *Container) bool {
	if !l.tailFromFile {
		return false
	}
	// Unsure this one is really useful, user could be instructed to clean up the registry
	if l.forceTailingFromFile {
		return true
	}
	// Check if there is a known offset for that container, if so keep tailing
	// the container from the docker socket
	registryID := fmt.Sprintf("docker:%s", container.service.Identifier)
	offset := l.registry.GetOffset(registryID)
	return offset == ""
}

func (l *DockerLauncher) scheduleFileSource(container *Container, source *logsconfig.LogSource) {
	containerID := container.service.Identifier
	if _, isTailed := l.fileSourcesByContainer[containerID]; isTailed {
		log.Println("Can't tail twice the same container: %v", ShortContainerID(containerID))
		return
	}
	// fileSource is a new source using the original source as its parent
	fileSource := l.getFileSource(container, source)
	// Keep source for later unscheduling
	l.fileSourcesByContainer[containerID] = fileSource
	l.sources.AddSource(fileSource.source)
}

func (l *DockerLauncher) unscheduleFileSource(containerID string) {
	if sourcePair, exists := l.fileSourcesByContainer[containerID]; exists {
		sourcePair.info.RemoveMessage(containerID)
		delete(l.fileSourcesByContainer, containerID)
		l.sources.RemoveSource(sourcePair.source)
	}
}

func (l *DockerLauncher) startSocketTailer(container *Container, source *logsconfig.LogSource) {
	containerID := container.service.Identifier
	if _, isTailed := l.getTailer(containerID); isTailed {
		log.Println("Can't tail twice the same container: %v", ShortContainerID(containerID))
		return
	}
	log.Println("DEBUG tail container %v from socket", ShortContainerID(containerID))
	dockerutil, err := dockerutil.GetDockerUtil()
	if err != nil {
		log.Println("Could not use docker client, logs for container %s won’t be collected: %v", containerID, err)
		return
	}
	// overridenSource == source if the containerCollectAll option is not activated or the container has AD labels
	overridenSource := l.overrideSource(container, source)
	tailer := NewTailer(dockerutil, containerID, overridenSource, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID, l.readTimeout)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier(), container.service.CreationTime)
	if err != nil {
		log.Println("Could not recover tailing from last committed offset %v: %v", ShortContainerID(containerID), err)
	}

	// start the tailer
	err = tailer.Start(since)
	if err != nil {
		log.Println("Could not start tailer %s: %v", containerID, err)
		return
	}
	source.AddInput(containerID)

	// keep the tailer in track to stop it later on
	l.addTailer(containerID, tailer)
}

// stopTailer stops the tailer matching the containerID.
func (l *DockerLauncher) stopTailer(containerID string) {
	if l.tailFromFile {
		l.unscheduleFileSource(containerID)
	} else {
		l.stopSocketTailer(containerID)
	}
}

func (l *DockerLauncher) stopSocketTailer(containerID string) {
	if tailer, isTailed := l.getTailer(containerID); isTailed {
		// No-op if the tailer source came from AD
		if l.collectAllSource != nil {
			l.collectAllSource.RemoveInput(containerID)
			l.collectAllInfo.RemoveMessage(containerID)
		}
		go tailer.Stop()
		l.removeTailer(containerID)
	}
}

func (l *DockerLauncher) restartTailer(containerID string) {
	// It should never happen
	if l.tailFromFile {
		return
	}
	backoffDuration := backoffInitialDuration
	cumulatedBackoff := 0 * time.Second
	var source *logsconfig.LogSource

	if oldTailer, exists := l.getTailer(containerID); exists {
		source = oldTailer.source
		if l.collectAllSource != nil {
			l.collectAllSource.RemoveInput(containerID)
			l.collectAllInfo.RemoveMessage(containerID)
		}
		oldTailer.Stop()
		l.removeTailer(containerID)
	} else {
		log.Println("Unable to restart tailer, old source not found, keeping previous one, container: %s", containerID)
		return
	}

	dockerutil, err := dockerutil.GetDockerUtil()
	if err != nil {
		// This cannot happen since, if we have a tailer to restart, it means that we created
		// it earlier and we couldn't have created it if the docker client wasn't initialized.
		log.Println("Could not use docker client, logs for container %s won’t be collected: %v", containerID, err)
		return
	}
	tailer := NewTailer(dockerutil, containerID, source, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID, l.readTimeout)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier(), service.Before)
	if err != nil {
		log.Println("Could not recover last committed offset for container %v: %v", ShortContainerID(containerID), err)
	}

	for {
		if backoffDuration > backoffMaxDuration {
			log.Println("Could not resume tailing container %v", ShortContainerID(containerID))
			return
		}

		// start the tailer
		err = tailer.Start(since)
		if err != nil {
			log.Println("Could not start tailer for container %v: %v", ShortContainerID(containerID), err)
			time.Sleep(backoffDuration)
			cumulatedBackoff += backoffDuration
			backoffDuration *= 2
			continue
		}
		// keep the tailer in track to stop it later on
		l.addTailer(containerID, tailer)
		source.AddInput(containerID)
		return
	}
}

func (l *DockerLauncher) addTailer(containerID string, tailer *Tailer) {
	l.lock.Lock()
	l.tailers[containerID] = tailer
	l.lock.Unlock()
}

func (l *DockerLauncher) removeTailer(containerID string) {
	l.lock.Lock()
	delete(l.tailers, containerID)
	l.lock.Unlock()
}

func (l *DockerLauncher) getTailer(containerID string) (*Tailer, bool) {
	l.lock.Lock()
	defer l.lock.Unlock()
	tailer, exist := l.tailers[containerID]
	return tailer, exist
}
