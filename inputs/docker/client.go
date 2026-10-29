package docker

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
	dockerClient "github.com/moby/moby/client"
)

var (
	defaultHeaders = map[string]string{"User-Agent": "engine-api-cli-1.0"}
)

type Client interface {
	Info(ctx context.Context) (system.Info, error)
	ContainerList(ctx context.Context, options dockerClient.ContainerListOptions) ([]container.Summary, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (dockerClient.ContainerStatsResult, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	Ping(ctx context.Context) (dockerClient.PingResult, error)
	ServiceList(ctx context.Context, options dockerClient.ServiceListOptions) ([]swarm.Service, error)
	TaskList(ctx context.Context, options dockerClient.TaskListOptions) ([]swarm.Task, error)
	NodeList(ctx context.Context, options dockerClient.NodeListOptions) ([]swarm.Node, error)
	Close() error
}

func NewEnvClient() (Client, error) {
	client, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		return nil, err
	}
	return &SocketClient{client}, nil
}

func NewClient(host string, tlsConfig *tls.Config) (Client, error) {
	transport := &http.Transport{}
	scheme := "http"

	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
		scheme = "https"
	}

	httpClient := &http.Client{Transport: transport}

	client, err := dockerClient.New(
		dockerClient.WithScheme(scheme),
		dockerClient.WithHTTPHeaders(defaultHeaders),
		dockerClient.WithHTTPClient(httpClient),
		dockerClient.WithHost(host))
	if err != nil {
		return nil, err
	}

	return &SocketClient{client}, nil
}

type SocketClient struct {
	client *dockerClient.Client
}

func (c *SocketClient) Info(ctx context.Context) (system.Info, error) {
	result, err := c.client.Info(ctx, dockerClient.InfoOptions{})
	return result.Info, err
}
func (c *SocketClient) Ping(ctx context.Context) (dockerClient.PingResult, error) {
	return c.client.Ping(ctx, dockerClient.PingOptions{NegotiateAPIVersion: true})
}
func (c *SocketClient) ContainerList(ctx context.Context, options dockerClient.ContainerListOptions) ([]container.Summary, error) {
	result, err := c.client.ContainerList(ctx, options)
	return result.Items, err
}
func (c *SocketClient) ContainerStats(ctx context.Context, containerID string, stream bool) (dockerClient.ContainerStatsResult, error) {
	// CPU usage is a delta between two samples, including in non-streaming mode.
	return c.client.ContainerStats(ctx, containerID, dockerClient.ContainerStatsOptions{Stream: stream, IncludePreviousSample: true})
}
func (c *SocketClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	result, err := c.client.ContainerInspect(ctx, containerID, dockerClient.ContainerInspectOptions{})
	return result.Container, err
}
func (c *SocketClient) ServiceList(ctx context.Context, options dockerClient.ServiceListOptions) ([]swarm.Service, error) {
	result, err := c.client.ServiceList(ctx, options)
	return result.Items, err
}
func (c *SocketClient) TaskList(ctx context.Context, options dockerClient.TaskListOptions) ([]swarm.Task, error) {
	result, err := c.client.TaskList(ctx, options)
	return result.Items, err
}
func (c *SocketClient) NodeList(ctx context.Context, options dockerClient.NodeListOptions) ([]swarm.Node, error) {
	result, err := c.client.NodeList(ctx, options)
	return result.Items, err
}
func (c *SocketClient) Close() error {
	return c.client.Close()
}
