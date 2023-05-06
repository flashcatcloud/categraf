//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/errors"
	"flashcat.cloud/categraf/logs/service"
	"flashcat.cloud/categraf/logs/util/containers"
	"flashcat.cloud/categraf/logs/util/kubernetes/kubelet"
	"flashcat.cloud/categraf/pkg/kubernetes"
	"flashcat.cloud/categraf/pkg/retry"
)

const (
	anyLogFile             = "*.log"
	anyV19LogFile          = "%s_*.log"
	AnnotationRuleKey      = "categraf/logs.stdout.processing_rules"
	AnnotationTopicKey     = "categraf/logs.stdout.topic"
	AnnotationTagPrefixKey = "categraf/tags.prefix"
	AnnotationCollectKey   = "categraf/logs.stdout.collect"
)

var (
	basePath = "/var/log/pods"
)

var errCollectAllDisabled = fmt.Errorf("%s disabled", logsconfig.ContainerCollectAll)

type retryOps struct {
	service          *service.Service
	backoff          backoff.BackOff
	removalScheduled bool
}

// Launcher looks for new and deleted pods to create or delete one logs-source per container.
type Launcher struct {
	sources            *logsconfig.LogSources
	sourcesByContainer map[string]*logsconfig.LogSource
	stopped            chan struct{}
	kubeutil           kubelet.KubeUtilInterface
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	retryOperations    chan *retryOps
	collectAll         bool
	pendingRetries     map[string]*retryOps
	serviceNameFunc    func(string, string) string // serviceNameFunc gets the service name from the tagger, it is in a separate field for testing purpose
}

// IsAvailable retrues true if the launcher is available and a retrier otherwise
func IsAvailable() (bool, *retry.Retrier) {

	util, retrier := kubelet.GetKubeUtilWithRetrier()
	if util != nil {
		log.Println("Kubernetes launcher is available")
		return true, nil
	}
	log.Printf("Kubernetes launcher is not available: %v", retrier.LastError())
	return false, retrier
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *logsconfig.LogSources, services *service.Services, collectAll bool) *Launcher {
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		log.Printf("KubeUtil not available, failed to create launcher", err)
		return nil
	}
	launcher := &Launcher{
		sources:            sources,
		sourcesByContainer: make(map[string]*logsconfig.LogSource),
		stopped:            make(chan struct{}),
		kubeutil:           kubeutil,
		collectAll:         collectAll,
		pendingRetries:     make(map[string]*retryOps),
		retryOperations:    make(chan *retryOps),
		serviceNameFunc:    ServiceNameFromTags,
	}
	launcher.addedServices = services.GetAllAddedServices()
	launcher.removedServices = services.GetAllRemovedServices()
	return launcher
}

// Start starts the launcher
func (l *Launcher) Start() {
	log.Println("Starting Kubernetes launcher")
	go l.run()
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	log.Println("Stopping Kubernetes launcher")
	l.stopped <- struct{}{}
}

// run handles new and deleted pods,
// the kubernetes launcher consumes new and deleted services pushed by the autodiscovery
func (l *Launcher) run() {
	for {
		select {
		case service := <-l.addedServices:
			l.addSource(service)
		case service := <-l.removedServices:
			l.removeSource(service)
		case ops := <-l.retryOperations:
			l.addSource(ops.service)
		case <-l.stopped:
			log.Println("Kubernetes launcher stopped")
			return
		}
	}
}

func (l *Launcher) scheduleServiceForRetry(svc *service.Service) {
	containerID := svc.GetEntityID()
	ops, exists := l.pendingRetries[containerID]
	if !exists {
		b := &backoff.ExponentialBackOff{
			InitialInterval:     500 * time.Millisecond,
			RandomizationFactor: 0,
			Multiplier:          2,
			MaxInterval:         5 * time.Second,
			MaxElapsedTime:      30 * time.Second,
			Clock:               backoff.SystemClock,
		}
		b.Reset()
		ops = &retryOps{
			service:          svc,
			backoff:          b,
			removalScheduled: false,
		}
		l.pendingRetries[containerID] = ops
	}
	l.delayRetry(ops)
}

func (l *Launcher) delayRetry(ops *retryOps) {
	delay := ops.backoff.NextBackOff()
	if delay == backoff.Stop {
		log.Println("Unable to add source for container %v", ops.service.GetEntityID())
		delete(l.pendingRetries, ops.service.GetEntityID())
		return
	}
	go func() {
		<-time.After(delay)
		l.retryOperations <- ops
	}()
}

// addSource creates a new log-source from a service by resolving the
// pod linked to the entityID of the service
func (l *Launcher) addSource(svc *service.Service) {
	// If the container is already tailed, we don't do anything
	// That shouldn't happen
	if _, exists := l.sourcesByContainer[svc.GetEntityID()]; exists {
		log.Printf("A source already exist for container %v", svc.GetEntityID())
		return
	}

	pod, err := l.kubeutil.GetPodForContainerID(context.TODO(), svc.GetEntityID())
	if err != nil {
		if errors.IsRetriable(err) {
			// Attempt to reschedule the source later
			log.Println("Failed to fetch pod info for container %v, will retry: %v", svc.Identifier, err)
			l.scheduleServiceForRetry(svc)
			return
		}
		log.Printf("Could not add source for container %v: %v", svc.Identifier, err)
		return
	}
	container, err := l.kubeutil.GetStatusForContainerID(pod, svc.GetEntityID())
	if err != nil {
		log.Println(err)
		return
	}
	source, err := l.getSource(pod, container)
	if err != nil {
		if err != errCollectAllDisabled {
			log.Println("Invalid configuration for pod %v, container %v: %v", pod.Metadata.Name, container.Name, err)
		}
		return
	}

	// force setting source type to kubernetes
	source.SetSourceType(logsconfig.KubernetesSourceType)

	l.sourcesByContainer[svc.GetEntityID()] = source
	l.sources.AddSource(source)

	// Clean-up retry logic
	if ops, exists := l.pendingRetries[svc.GetEntityID()]; exists {
		if ops.removalScheduled {
			// A removal was emitted while addSource was being retried
			l.removeSource(ops.service)
		}
		delete(l.pendingRetries, svc.GetEntityID())
	}
}

// removeSource removes a new log-source from a service
func (l *Launcher) removeSource(service *service.Service) {
	containerID := service.GetEntityID()
	if ops, exists := l.pendingRetries[containerID]; exists {
		// Service was added unsuccessfully and is being retried
		ops.removalScheduled = true
		return
	}
	if source, exists := l.sourcesByContainer[containerID]; exists {
		delete(l.sourcesByContainer, containerID)
		l.sources.RemoveSource(source)
	}
}

// kubernetesIntegration represents the name of the integration.
const kubernetesIntegration = "kubernetes"

// getSource returns a new source for the container in pod.
func (l *Launcher) getSource(pod *kubernetes.Pod, container kubernetes.ContainerStatus) (*logsconfig.LogSource, error) {
	var cfg *logsconfig.LogsConfig
	standardService := l.serviceNameFunc(container.Name, getTaggerEntityID(container.ID))
	// if annotation := l.getAnnotation(pod, container); annotation != "" {
	// 	configs, err := logsconfig.ParseJSON([]byte(annotation))
	// 	if err != nil || len(configs) == 0 {
	// 		return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
	// 	}
	// 	// We may have more than one log configuration in the annotation, ignore those
	// 	// unrelated to containers
	// 	containerType, _ := containers.SplitEntityName(container.ID)
	// 	for _, c := range configs {
	// 		if c.Type == "" || c.Type == containerType {
	// 			cfg = c
	// 			break
	// 		}
	// 	}
	// 	if cfg == nil {
	// 		log.Printf("annotation found: %v, for pod %v, container %v, but no config was usable for container log collection", annotation, pod.Metadata.Name, container.Name)
	// 	}
	// }

	if cfg == nil {
		if !l.collectAll {
			return nil, errCollectAllDisabled
		}
		if !(pod.Metadata.Annotations[AnnotationCollectKey] == "" ||
			pod.Metadata.Annotations[AnnotationCollectKey] == "true") {
			log.Printf("pod %s disable stdout collecting", pod.Metadata.Name)
			return nil, errCollectAllDisabled
		}
		// The logs source is the short image name
		logsSource := ""
		shortImageName, err := l.getShortImageName(pod, container.Name)
		if err != nil {
			log.Printf("Couldn't get short image for container '%s': %v", container.Name, err)
			// Fallback and use `kubernetes` as source name
			logsSource = kubernetesIntegration
		} else {
			logsSource = shortImageName
		}
		rules := make([]*logsconfig.ProcessingRule, 0)
		ruleStr, ok := pod.Metadata.Annotations[AnnotationRuleKey]
		if ok {
			err = json.Unmarshal([]byte(ruleStr), &rules)
			if err != nil {
				log.Printf("pod rule %s unmarshal error %s", ruleStr, err)
			}
		}
		topic := pod.Metadata.Annotations[AnnotationTopicKey]
		if standardService != "" {
			cfg = &logsconfig.LogsConfig{
				Source:          logsSource,
				Service:         standardService,
				Tags:            buildTags(pod, container),
				Topic:           topic,
				ProcessingRules: rules,
			}
		} else {
			cfg = &logsconfig.LogsConfig{
				Source:          logsSource,
				Service:         logsSource,
				Tags:            buildTags(pod, container),
				Topic:           topic,
				ProcessingRules: rules,
			}
		}
	}
	if cfg.Service == "" && standardService != "" {
		cfg.Service = standardService
	}
	cfg.Type = logsconfig.FileType
	if v := os.Getenv("HOST_MOUNT_PREFIX"); v != "" && !strings.HasPrefix(basePath, v) {
		basePath = filepath.Join(v, basePath)
	}
	cfg.Path = l.getPath(basePath, pod, container)
	cfg.Identifier = kubelet.TrimRuntimeFromCID(container.ID)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}

	return logsconfig.NewLogSource(l.getSourceName(pod, container), cfg), nil
}

func buildTags(pod *kubernetes.Pod, container kubernetes.ContainerStatus) []string {
	tags := []string{
		fmt.Sprintf("kubernetes.namespace_name=%s", pod.Metadata.Namespace),
		fmt.Sprintf("kubernetes.pod_id=%s", pod.Metadata.UID),
		fmt.Sprintf("kubernetes.pod_name=%s", pod.Metadata.Name),
		fmt.Sprintf("kubernetes.host=%s", pod.Spec.NodeName),
		fmt.Sprintf("kubernetes.container_id=%s", container.ID),
		fmt.Sprintf("kubernetes.container_name=%s", container.Name),
		fmt.Sprintf("kubernetes.container_image=%s", container.Image),
		fmt.Sprintf("kubernetes.container_hash=%s", container.ImageID),
	}
	prefixStr, ok := pod.Metadata.Annotations[AnnotationTagPrefixKey]
	if !ok {
		return tags
	}
	prefixes := strings.Split(prefixStr, ",")
	for _, prefix := range prefixes {
		for k, v := range pod.Metadata.Labels {
			if strings.HasPrefix(k, prefix) {
				tags = append(tags, fmt.Sprintf("kubernetes.labels.%s:%s", k, v))
			}
		}
		for k, v := range pod.Metadata.Annotations {
			if strings.HasPrefix(k, prefix) {
				tags = append(tags, fmt.Sprintf("kubernetes.annotations.%s:%s", k, v))
			}
		}
	}
	return tags
}

// getTaggerEntityID builds an entity ID from a kubernetes container ID
// Transforms the <runtime>:// prefix into container_id://
// Returns the original container ID if an error occurred
func getTaggerEntityID(ctrID string) string {
	taggerEntityID, err := kubelet.KubeContainerIDToTaggerEntityID(ctrID)
	if err != nil {
		log.Printf("Could not get tagger entity ID: %v", err)
		return ctrID
	}
	return taggerEntityID
}

// configPath refers to the configuration that can be passed over a pod annotation,
// this feature is commonly named 'ad' or 'autodiscovery'.
// The pod annotation must respect the format: ad.datadoghq.com/<container_name>.logs: '[{...}]'.
const (
	configPathPrefix = "ad.flashcat.cloud"
	configPathSuffix = "logs"
)

// getConfigPath returns the path of the logs-config annotation for container.
func (l *Launcher) getConfigPath(container kubernetes.ContainerStatus) string {
	return fmt.Sprintf("%s/%s.%s", configPathPrefix, container.Name, configPathSuffix)
}

// getAnnotation returns the logs-config annotation for container if present.
// FIXME: Reuse the annotation logic from AD
func (l *Launcher) getAnnotation(pod *kubernetes.Pod, container kubernetes.ContainerStatus) string {
	configPath := l.getConfigPath(container)
	if annotation, exists := pod.Metadata.Annotations[configPath]; exists {
		return annotation
	}
	return ""
}

// getSourceName returns the source name of the container to tail.
func (l *Launcher) getSourceName(pod *kubernetes.Pod, container kubernetes.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
}

// getPath returns a wildcard matching with any logs file of container in pod.
func (l *Launcher) getPath(basePath string, pod *kubernetes.Pod, container kubernetes.ContainerStatus) string {
	// the pattern for container logs is different depending on the version of Kubernetes
	// so we need to try three possbile formats
	// until v1.9 it was `/var/log/pods/{pod_uid}/{container_name_n}.log`,
	// v.1.10 to v1.13 it was `/var/log/pods/{pod_uid}/{container_name}/{n}.log`,
	// since v1.14 it is `/var/log/pods/{pod_namespace}_{pod_name}_{pod_uid}/{container_name}/{n}.log`.
	// see: https://github.com/kubernetes/kubernetes/pull/74441 for more information.
	oldDirectory := filepath.Join(basePath, l.getPodDirectoryUntil1_13(pod))
	if _, err := os.Stat(oldDirectory); err == nil {
		v110Dir := filepath.Join(oldDirectory, container.Name)
		_, err := os.Stat(v110Dir)
		if err == nil {
			log.Printf("Logs path found for container %s, v1.13 >= kubernetes version >= v1.10", container.Name)
			return filepath.Join(v110Dir, anyLogFile)
		}
		if !os.IsNotExist(err) {
			log.Printf("Cannot get file info for %s: %v", v110Dir, err)
		}

		v19Files := filepath.Join(oldDirectory, fmt.Sprintf(anyV19LogFile, container.Name))
		files, err := filepath.Glob(v19Files)
		if err == nil && len(files) > 0 {
			log.Printf("Logs path found for container %s, kubernetes version <= v1.9", container.Name)
			return v19Files
		}
		if err != nil {
			log.Printf("Cannot get file info for %s: %v", v19Files, err)
		}
		if len(files) == 0 {
			log.Printf("Files matching %s not found", v19Files)
		}
	}

	log.Printf("Using the latest kubernetes logs path for container %s", container.Name)
	return filepath.Join(basePath, l.getPodDirectorySince1_14(pod), container.Name, anyLogFile)
}

// getPodDirectoryUntil1_13 returns the name of the directory of pod containers until Kubernetes v1.13.
func (l *Launcher) getPodDirectoryUntil1_13(pod *kubernetes.Pod) string {
	return pod.Metadata.UID
}

// getPodDirectorySince1_14 returns the name of the directory of pod containers since Kubernetes v1.14.
func (l *Launcher) getPodDirectorySince1_14(pod *kubernetes.Pod) string {
	return fmt.Sprintf("%s_%s_%s", pod.Metadata.Namespace, pod.Metadata.Name, pod.Metadata.UID)
}

// getShortImageName returns the short image name of a container
func (l *Launcher) getShortImageName(pod *kubernetes.Pod, containerName string) (string, error) {
	containerSpec, err := l.kubeutil.GetSpecForContainerName(pod, containerName)
	if err != nil {
		return "", err
	}
	_, shortName, _, err := containers.SplitImageName(containerSpec.Image)
	if err != nil {
		log.Printf("Cannot parse image name: %v", err)
	}
	return shortName, err
}

func ServiceNameFromTags(ctrName, taggerEntity string) string {
	return ""
}
