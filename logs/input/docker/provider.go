package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"flashcat.cloud/categraf/logs/autodiscovery/integration"
	dockerUtil "flashcat.cloud/categraf/logs/util/docker"
)

const (
	dockerADLabelPrefix = "cloud.flashcat.ad."
	delayDuration       = 5 * time.Second
)

// DockerConfigProvider implements the ConfigProvider interface for the docker labels.
type DockerConfigProvider struct {
	sync.RWMutex
	dockerUtil   *dockerUtil.DockerUtil
	upToDate     bool
	streaming    bool
	labelCache   map[string]map[string]string
	syncInterval int
	syncCounter  int
}

// NewDockerConfigProvider returns a new ConfigProvider connected to docker.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewDockerConfigProvider() (*DockerConfigProvider, error) {
	return &DockerConfigProvider{
		// periodically resync every 30 runs if we're missing events
		syncInterval: 30,
	}, nil
}

// String returns a string representation of the DockerConfigProvider
func (d *DockerConfigProvider) String() string {
	return "docker"
}

// Collect retrieves all running containers and extract AD templates from their labels.
func (d *DockerConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	var err error
	firstCollection := false

	d.Lock()
	if d.dockerUtil == nil {
		d.dockerUtil, err = dockerUtil.GetDockerUtil()
		if err != nil {
			d.Unlock()
			return []integration.Config{}, err
		}
		firstCollection = true
	}

	var containers map[string]map[string]string
	// on the first run we collect all labels, then rely on individual events to
	// avoid listing all containers too often
	if d.labelCache == nil || d.syncCounter == d.syncInterval {
		containers, err = d.dockerUtil.AllContainerLabels(ctx)
		if err != nil {
			d.Unlock()
			return []integration.Config{}, err
		}
		d.labelCache = containers
		d.syncCounter = 0
	} else {
		containers = d.labelCache
	}

	d.syncCounter++
	d.upToDate = true
	d.Unlock()

	// start listening after the first collection to avoid race in cache map init
	if firstCollection {
		go d.listen()
	}

	d.RLock()
	defer d.RUnlock()
	return parseDockerLabels(containers)
}

// We listen to docker events and invalidate our cache when we receive a start/die event
func (d *DockerConfigProvider) listen() {
	d.Lock()
	d.streaming = true
	d.Unlock()
	timer := time.NewTimer(15 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
CONNECT:
	for {
		eventChan, errChan, err := d.dockerUtil.SubscribeToContainerEvents(d.String())
		if err != nil {
			log.Printf("error subscribing to docker events: %s", err)
			break CONNECT // We disable streaming and revert to always-pull behaviour
		}

		for {
			select {
			case healthDeadline := <-timer.C:
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
			case ev := <-eventChan:
				// As our input is the docker `client.ContainerList`, which lists running containers,
				// only these two event types will change what containers appear.
				// Container labels cannot change once they are created, so we don't need to react on
				// other lifecycle events.
				if ev.Action == dockerUtil.ContainerEventActionStart {
					container, err := d.dockerUtil.Inspect(ctx, ev.ContainerID, false)
					if err != nil {
						log.Printf("W! Error inspecting container: %s", err)
					} else {
						d.Lock()
						_, containerSeen := d.labelCache[ev.ContainerID]
						d.Unlock()
						if containerSeen {
							// Container restarted with the same ID within 5 seconds.
							time.AfterFunc(delayDuration, func() {
								d.addLabels(ev.ContainerID, container.Config.Labels)
							})
						} else {
							d.addLabels(ev.ContainerID, container.Config.Labels)
						}
					}
				} else if ev.Action == dockerUtil.ContainerEventActionDie || ev.Action == dockerUtil.ContainerEventActionDied {
					// delay for short lived detection
					time.AfterFunc(delayDuration, func() {
						d.Lock()
						delete(d.labelCache, ev.ContainerID)
						d.upToDate = false
						d.Unlock()
					})
				}
			case err := <-errChan:
				log.Printf("Error getting docker events: %s", err)
				d.Lock()
				d.upToDate = false
				d.Unlock()
				continue CONNECT // Re-connect to dockerutils
			}
		}
	}

	d.Lock()
	d.streaming = false
	d.Unlock()
	cancel()
}

// IsUpToDate checks whether we have new containers to parse, based on events received by the listen goroutine.
// If listening fails, we fallback to Collecting everytime.
func (d *DockerConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	d.RLock()
	defer d.RUnlock()
	return (d.streaming && d.upToDate), nil
}

// addLabels updates the label cache for a given container
func (d *DockerConfigProvider) addLabels(containerID string, labels map[string]string) {
	d.Lock()
	defer d.Unlock()
	d.labelCache[containerID] = labels
	d.upToDate = false
}

func parseDockerLabels(containers map[string]map[string]string) ([]integration.Config, error) {
	var configs []integration.Config
	for cID, labels := range containers {
		dockerEntityName := dockerUtil.ContainerIDToEntityName(cID)
		c, errors := extractTemplatesFromMap(dockerEntityName, labels, dockerADLabelPrefix)

		for _, err := range errors {
			log.Printf("E! Can't parse template for container %s: %s", cID, err)
		}

		for idx := range c {
			c[idx].Source = "docker:" + dockerEntityName
		}

		configs = append(configs, c...)
	}
	return configs, nil
}

func init() {
	// initRegisterProvider("docker", NewDockerConfigProvider)
}

type ErrorMsgSet map[string]struct{}

// GetConfigErrors is not implemented for the DockerConfigProvider
func (d *DockerConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}

func extractTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, []error) {
	var configs []integration.Config
	var errors []error

	logsConfigs, err := extractLogsTemplatesFromMap(key, input, prefix)
	if err != nil {
		errors = append(errors, fmt.Errorf("could not extract logs config: %v", err))
	}
	configs = append(configs, logsConfigs...)

	return configs, errors
}

// extractLogsTemplatesFromMap returns the logs configuration from a given map,
// if none are found return an empty list.
func extractLogsTemplatesFromMap(key string, input map[string]string, prefix string) ([]integration.Config, error) {
	const (
		logsConfigPath = "logs"
	)
	value, found := input[prefix+logsConfigPath]
	if !found {
		return []integration.Config{}, nil
	}
	var data interface{}
	err := json.Unmarshal([]byte(value), &data)
	if err != nil {
		return []integration.Config{}, fmt.Errorf("in %s: %s", logsConfigPath, err)
	}
	switch data.(type) {
	case []interface{}:
		logsConfig, _ := json.Marshal(data)
		return []integration.Config{{LogsConfig: logsConfig, ADIdentifiers: []string{key}}}, nil
	default:
		return []integration.Config{}, fmt.Errorf("invalid format, expected an array, got: '%v'", data)
	}
}
