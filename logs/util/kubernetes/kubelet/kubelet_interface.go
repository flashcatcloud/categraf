//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubelet

import (
	"context"

	"flashcat.cloud/categraf/logs/util/containers"
	"flashcat.cloud/categraf/pkg/kubernetes"
)

// KubeUtilInterface defines the interface for kubelet api
// see `kubelet_orchestrator` for orchestrator-only interface
type KubeUtilInterface interface {
	GetNodeInfo(ctx context.Context) (string, string, error)
	GetNodename(ctx context.Context) (string, error)
	GetLocalPodList(ctx context.Context) ([]*kubernetes.Pod, error)
	ForceGetLocalPodList(ctx context.Context) ([]*kubernetes.Pod, error)
	GetPodForContainerID(ctx context.Context, containerID string) (*kubernetes.Pod, error)
	GetStatusForContainerID(pod *kubernetes.Pod, containerID string) (kubernetes.ContainerStatus, error)
	GetSpecForContainerName(pod *kubernetes.Pod, containerName string) (kubernetes.ContainerSpec, error)
	GetPodFromUID(ctx context.Context, podUID string) (*kubernetes.Pod, error)
	GetPodForEntityID(ctx context.Context, entityID string) (*kubernetes.Pod, error)
	QueryKubelet(ctx context.Context, path string) ([]byte, int, error)
	GetKubeletAPIEndpoint() string
	GetRawConnectionInfo() map[string]string
	GetRawMetrics(ctx context.Context) ([]byte, error)
	IsAgentHostNetwork(ctx context.Context) (bool, error)
	ListContainers(ctx context.Context) ([]*containers.Container, error)
	UpdateContainerMetrics(ctrList []*containers.Container) error
}
