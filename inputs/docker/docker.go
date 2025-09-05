package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/pkg/dock"
	"flashcat.cloud/categraf/pkg/filter"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"

	tlsx "flashcat.cloud/categraf/pkg/tls"
	itypes "flashcat.cloud/categraf/types"
)

const inputName = "docker"

// KB, MB, GB, TB, PB...human friendly
const (
	KB = 1000
	MB = 1000 * KB
	GB = 1000 * MB
	TB = 1000 * GB
	PB = 1000 * TB
)

var (
	// sizeRegex              = regexp.MustCompile(`^(\d+(\.\d+)*) ?([kKmMgGtTpP])?[bB]?$`)
	containerStates        = []string{"created", "restarting", "running", "removing", "paused", "exited", "dead"}
	containerMetricClasses = []string{"cpu", "network", "blkio"}
)

type Docker struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Docker{}
	})
}

func (d *Docker) Clone() inputs.Input {
	return &Docker{}
}

func (c Docker) Name() string {
	return inputName
}

func (d *Docker) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(d.Instances))
	for i := 0; i < len(d.Instances); i++ {
		ret[i] = d.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Endpoint                   string   `toml:"endpoint"`
	GatherServices             bool     `toml:"gather_services"`
	GatherExtendMemstats       bool     `toml:"gather_extend_memstats"`
	ContainerIDLabelEnable     bool     `toml:"container_id_label_enable"`
	ContainerIDLabelShortStyle bool     `toml:"container_id_label_short_style"`
	PerDeviceInclude           []string `toml:"perdevice_include"`
	TotalInclude               []string `toml:"total_include"`
	TagEnvironment             []string `toml:"tag_env"`
	LabelInclude               []string `toml:"docker_label_include"`
	LabelExclude               []string `toml:"docker_label_exclude"`
	ContainerInclude           []string `toml:"container_name_include"`
	ContainerExclude           []string `toml:"container_name_exclude"`
	ContainerStateInclude      []string `toml:"container_state_include"`
	ContainerStateExclude      []string `toml:"container_state_exclude"`

	Timeout config.Duration
	tlsx.ClientConfig

	client          Client
	labelFilter     filter.Filter
	containerFilter filter.Filter
	stateFilter     filter.Filter

	engineHost    string
	serverVersion string
}

func (ins *Instance) Init() error {
	if len(ins.Endpoint) == 0 {
		return itypes.ErrInstancesEmpty
	}

	c, err := ins.getNewClient()
	if err != nil {
		return err
	}
	ins.client = c

	err = choice.CheckSlice(ins.PerDeviceInclude, containerMetricClasses)
	if err != nil {
		return fmt.Errorf("error validating 'perdevice_include' setting : %v", err)
	}

	err = choice.CheckSlice(ins.TotalInclude, containerMetricClasses)
	if err != nil {
		return fmt.Errorf("error validating 'total_include' setting : %v", err)
	}

	if err = ins.createLabelFilters(); err != nil {
		return err
	}

	if err = ins.createContainerFilters(); err != nil {
		return err
	}

	if err = ins.createContainerStateFilters(); err != nil {
		return err
	}

	return nil
}

func (ins *Instance) Gather(slist *itypes.SampleList) {
	if ins.Endpoint == "" {
		return
	}

	if ins.client == nil {
		c, err := ins.getNewClient()
		if err != nil {
			slist.PushSample("docker", "up", 0)
			log.Println("E! failed to new docker client:", err)
			return
		}
		ins.client = c
	}

	defer ins.client.Close()

	if err := ins.gatherInfo(slist); err != nil {
		slist.PushSample("docker", "up", 0)
		log.Println("E! failed to gather docker info:", err)
		return
	}

	slist.PushSample("docker", "up", 1)

	if ins.GatherServices {
		ins.gatherSwarmInfo(slist)
	}

	filterArgs := filters.NewArgs()
	for _, state := range containerStates {
		if ins.stateFilter.Match(state) {
			filterArgs.Add("status", state)
		}
	}

	// All container states were excluded
	if filterArgs.Len() == 0 {
		return
	}

	// List containers
	opts := types.ContainerListOptions{
		Filters: filterArgs,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	containers, err := ins.client.ContainerList(ctx, opts)
	if err == context.DeadlineExceeded {
		log.Println("E! failed to gather container list: timeout")
		return
	}
	if err != nil {
		log.Println("E! failed to gather container list:", err)
		return
	}

	// Get container data
	var wg sync.WaitGroup
	wg.Add(len(containers))
	for _, container := range containers {
		go func(c types.Container) {
			defer wg.Done()
			ins.gatherContainer(c, slist)
		}(container)
	}
	wg.Wait()
}

func (ins *Instance) gatherContainer(container types.Container, slist *itypes.SampleList) {
	// Parse container name
	var cname string
	for _, name := range container.Names {
		trimmedName := strings.TrimPrefix(name, "/")
		if !strings.Contains(trimmedName, "/") {
			cname = trimmedName
			break
		}
	}

	if cname == "" {
		return
	}

	if !ins.containerFilter.Match(cname) {
		return
	}

	imageName, _ := dock.ParseImage(container.Image)

	tags := map[string]string{
		"container_name":  cname,
		"container_image": imageName,
		// "container_version": imageVersion,
		"engine_host":    ins.engineHost,
		"server_version": ins.serverVersion,
	}

	if ins.ContainerIDLabelEnable {
		tags["container_id"] = container.ID
		if ins.ContainerIDLabelShortStyle {
			tags["container_id"] = hostnameFromID(container.ID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	r, err := ins.client.ContainerStats(ctx, container.ID, false)
	if err == context.DeadlineExceeded {
		log.Println("E! failed to get container stats: timeout")
		return
	}
	if err != nil {
		log.Println("E! failed to get container stats:", err)
		return
	}

	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)

	var v *types.StatsJSON
	if err = dec.Decode(&v); err != nil {
		if err != io.EOF {
			log.Println("E! failed to decode output of container stats:", err)
		}
		return
	}

	// Add labels to tags
	for k, label := range container.Labels {
		if ins.labelFilter.Match(k) {
			tags[k] = label
		}
	}

	err = ins.gatherContainerInspect(container, slist, tags, r.OSType, v)
	if err != nil {
		log.Println("E! failed to gather container inspect:", err)
	}
}

func (ins *Instance) gatherContainerInspect(container types.Container, slist *itypes.SampleList, tags map[string]string, daemonOSType string, v *types.StatsJSON) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	info, err := ins.client.ContainerInspect(ctx, container.ID)
	if err == context.DeadlineExceeded {
		return errInspectTimeout
	}
	if err != nil {
		return fmt.Errorf("error inspecting docker container: %v", err)
	}

	// Add whitelisted environment variables to tags
	if len(ins.TagEnvironment) > 0 {
		for _, envvar := range info.Config.Env {
			for _, configvar := range ins.TagEnvironment {
				dockEnv := strings.SplitN(envvar, "=", 2)
				// check for presence of tag in whitelist
				if len(dockEnv) == 2 && len(strings.TrimSpace(dockEnv[1])) != 0 && configvar == dockEnv[0] {
					tags[dockEnv[0]] = dockEnv[1]
				}
			}
		}
	}

	if info.State != nil {
		tags["container_status"] = info.State.Status
		oomKilled := func() int {
			if info.State.OOMKilled == true {
				return 1
			}
			return 0
		}
		statefields := map[string]interface{}{
			"status_oomkilled":     oomKilled(),
			"status_pid":           info.State.Pid,
			"status_exitcode":      info.State.ExitCode,
			"status_restart_count": info.RestartCount,
			//"container_id":  container.ID,
		}
		finished, err := time.Parse(time.RFC3339, info.State.FinishedAt)
		if err == nil && !finished.IsZero() {
			statefields["status_finished_at"] = finished.Unix()
		} else {
			// set finished to now for use in uptime
			finished = time.Now()
		}

		started, err := time.Parse(time.RFC3339, info.State.StartedAt)
		if err == nil && !started.IsZero() {
			statefields["status_started_at"] = started.Unix()

			uptime := finished.Sub(started)
			if finished.Before(started) {
				uptime = time.Since(started)
			}
			statefields["status_uptime"] = uptime.Seconds()
		}

		slist.PushSamples("docker_container", statefields, tags)

		if info.State.Health != nil {
			slist.PushSample("docker_container", "health_failing_streak", info.ContainerJSONBase.State.Health.FailingStreak, tags)
			var containerHealthStatesNum int
			switch info.ContainerJSONBase.State.Health.Status {
			case "healthy":
				containerHealthStatesNum = 0
			case "starting":
				containerHealthStatesNum = 1
			default:
				containerHealthStatesNum = 2
			}
			slist.PushSample("docker_container", "health_status", containerHealthStatesNum, tags)
		}
	}

	ins.parseContainerStats(v, slist, tags, daemonOSType)

	return nil
}

func (ins *Instance) parseContainerStats(stat *types.StatsJSON, slist *itypes.SampleList, tags map[string]string, ostype string) {
	// memory

	basicMemstats := []string{
		"cache",
		"rss",
		"total_cache",
		"total_rss",
	}

	extendMemstats := []string{
		"active_anon",
		"active_file",
		"hierarchical_memory_limit",
		"inactive_anon",
		"inactive_file",
		"mapped_file",
		"pgfault",
		"pgmajfault",
		"pgpgin",
		"pgpgout",
		"rss_huge",
		"total_active_anon",
		"total_active_file",
		"total_inactive_anon",
		"total_inactive_file",
		"total_mapped_file",
		"total_pgfault",
		"total_pgmajfault",
		"total_pgpgin",
		"total_pgpgout",
		"total_rss_huge",
		"total_unevictable",
		"total_writeback",
		"unevictable",
		"writeback",
	}

	memfields := map[string]interface{}{}

	for _, field := range basicMemstats {
		if value, ok := stat.MemoryStats.Stats[field]; ok {
			memfields[field] = value
		}
	}

	if ins.GatherExtendMemstats {
		for _, field := range extendMemstats {
			if value, ok := stat.MemoryStats.Stats[field]; ok {
				memfields[field] = value
			}
		}
	}

	if stat.MemoryStats.Failcnt != 0 {
		memfields["fail_count"] = stat.MemoryStats.Failcnt
	}

	if ostype != "windows" {
		memfields["limit"] = stat.MemoryStats.Limit
		memfields["max_usage"] = stat.MemoryStats.MaxUsage

		mem := CalculateMemUsageUnixNoCache(stat.MemoryStats)
		memLimit := float64(stat.MemoryStats.Limit)
		memfields["usage"] = uint64(mem)
		memfields["usage_percent"] = CalculateMemPercentUnixNoCache(memLimit, mem)
	} else {
		memfields["commit_bytes"] = stat.MemoryStats.Commit
		memfields["commit_peak_bytes"] = stat.MemoryStats.CommitPeak
		memfields["private_working_set"] = stat.MemoryStats.PrivateWorkingSet
	}

	slist.PushSamples("docker_container_mem", memfields, tags)

	// cpu

	if choice.Contains("cpu", ins.TotalInclude) {
		cpufields := map[string]interface{}{
			"usage_total":                  stat.CPUStats.CPUUsage.TotalUsage,
			"usage_in_usermode":            stat.CPUStats.CPUUsage.UsageInUsermode,
			"usage_in_kernelmode":          stat.CPUStats.CPUUsage.UsageInKernelmode,
			"usage_system":                 stat.CPUStats.SystemUsage,
			"throttling_periods":           stat.CPUStats.ThrottlingData.Periods,
			"throttling_throttled_periods": stat.CPUStats.ThrottlingData.ThrottledPeriods,
			"throttling_throttled_time":    stat.CPUStats.ThrottlingData.ThrottledTime,
		}

		if ostype != "windows" {
			previousCPU := stat.PreCPUStats.CPUUsage.TotalUsage
			previousSystem := stat.PreCPUStats.SystemUsage
			cpuPercent := CalculateCPUPercentUnix(previousCPU, previousSystem, stat)
			cpufields["usage_percent"] = cpuPercent
		} else {
			cpuPercent := calculateCPUPercentWindows(stat)
			cpufields["usage_percent"] = cpuPercent
		}

		slist.PushSamples("docker_container_cpu", cpufields, map[string]string{"cpu": "cpu-total"}, tags)
	}

	if choice.Contains("cpu", ins.PerDeviceInclude) && len(stat.CPUStats.CPUUsage.PercpuUsage) > 0 {
		var percpuusage []uint64
		if stat.CPUStats.OnlineCPUs > 0 {
			percpuusage = stat.CPUStats.CPUUsage.PercpuUsage[:stat.CPUStats.OnlineCPUs]
		} else {
			percpuusage = stat.CPUStats.CPUUsage.PercpuUsage
		}

		for i, percpu := range percpuusage {
			slist.PushSample("", "docker_container_cpu_usage_total", percpu, map[string]string{"cpu": fmt.Sprintf("cpu%d", i)}, tags)
		}
	}

	// network

	totalNetworkStatMap := make(map[string]interface{})
	for network, netstats := range stat.Networks {
		netfields := map[string]interface{}{
			"rx_dropped": netstats.RxDropped,
			"rx_bytes":   netstats.RxBytes,
			"rx_errors":  netstats.RxErrors,
			"tx_packets": netstats.TxPackets,
			"tx_dropped": netstats.TxDropped,
			"rx_packets": netstats.RxPackets,
			"tx_errors":  netstats.TxErrors,
			"tx_bytes":   netstats.TxBytes,
		}

		if choice.Contains("network", ins.PerDeviceInclude) {
			slist.PushSamples("docker_container_net", netfields, map[string]string{"network": network}, tags)
		}

		if choice.Contains("network", ins.TotalInclude) {
			for field, value := range netfields {
				var uintV uint64
				switch v := value.(type) {
				case uint64:
					uintV = v
				case int64:
					uintV = uint64(v)
				default:
					continue
				}

				_, ok := totalNetworkStatMap[field]
				if ok {
					totalNetworkStatMap[field] = totalNetworkStatMap[field].(uint64) + uintV
				} else {
					totalNetworkStatMap[field] = uintV
				}
			}
		}
	}

	// totalNetworkStatMap could be empty if container is running with --net=host.
	if choice.Contains("network", ins.TotalInclude) && len(totalNetworkStatMap) != 0 {
		slist.PushSamples("docker_container_net", totalNetworkStatMap, map[string]string{"network": "total"}, tags)
	}

	ins.gatherBlockIOMetrics(slist, stat, tags)
}

func (ins *Instance) gatherBlockIOMetrics(slist *itypes.SampleList, stat *types.StatsJSON, tags map[string]string) {
	perDeviceBlkio := choice.Contains("blkio", ins.PerDeviceInclude)
	totalBlkio := choice.Contains("blkio", ins.TotalInclude)

	blkioStats := stat.BlkioStats
	deviceStatMap := getDeviceStatMap(blkioStats)

	totalStatMap := make(map[string]interface{})
	for device, fields := range deviceStatMap {
		if perDeviceBlkio {
			slist.PushSamples("", fields, map[string]string{"device": device}, tags)
		}
		if totalBlkio {
			for field, value := range fields {
				var uintV uint64
				switch v := value.(type) {
				case uint64:
					uintV = v
				case int64:
					uintV = uint64(v)
				default:
					continue
				}

				_, ok := totalStatMap[field]
				if ok {
					totalStatMap[field] = totalStatMap[field].(uint64) + uintV
				} else {
					totalStatMap[field] = uintV
				}
			}
		}
	}

	if totalBlkio {
		slist.PushSamples("", totalStatMap, map[string]string{"device": "total"}, tags)
	}
}

func getDeviceStatMap(blkioStats types.BlkioStats) map[string]map[string]interface{} {
	deviceStatMap := make(map[string]map[string]interface{})

	for _, metric := range blkioStats.IoServiceBytesRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		_, ok := deviceStatMap[device]
		if !ok {
			deviceStatMap[device] = make(map[string]interface{})
		}

		field := fmt.Sprintf("docker_container_blkio_io_service_bytes_recursive_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoServicedRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		_, ok := deviceStatMap[device]
		if !ok {
			deviceStatMap[device] = make(map[string]interface{})
		}

		field := fmt.Sprintf("docker_container_blkio_io_serviced_recursive_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoQueuedRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		field := fmt.Sprintf("docker_container_blkio_io_queue_recursive_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoServiceTimeRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		field := fmt.Sprintf("docker_container_blkio_io_service_time_recursive_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoWaitTimeRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		field := fmt.Sprintf("docker_container_blkio_io_wait_time_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoMergedRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		field := fmt.Sprintf("docker_container_blkio_io_merged_recursive_%s", strings.ToLower(metric.Op))
		deviceStatMap[device][field] = metric.Value
	}

	for _, metric := range blkioStats.IoTimeRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		deviceStatMap[device]["docker_container_blkio_io_time_recursive"] = metric.Value
	}

	for _, metric := range blkioStats.SectorsRecursive {
		device := fmt.Sprintf("%d:%d", metric.Major, metric.Minor)
		deviceStatMap[device]["docker_container_blkio_sectors_recursive"] = metric.Value
	}
	return deviceStatMap
}

func (ins *Instance) gatherSwarmInfo(slist *itypes.SampleList) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	services, err := ins.client.ServiceList(ctx, types.ServiceListOptions{})
	if err == context.DeadlineExceeded {
		log.Println("E! failed to gather swarm info: timeout")
		return
	}
	if err != nil {
		log.Println("E! failed to gather swarm info:", err)
		return
	}

	if len(services) == 0 {
		return
	}

	tasks, err := ins.client.TaskList(ctx, types.TaskListOptions{})
	if err != nil {
		log.Println("E! failed to gather swarm info:", err)
		return
	}

	nodes, err := ins.client.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		log.Println("E! failed to gather swarm info:", err)
		return
	}

	activeNodes := make(map[string]struct{})
	for _, n := range nodes {
		if n.Status.State != swarm.NodeStateDown {
			activeNodes[n.ID] = struct{}{}
		}
	}

	running := map[string]int{}
	tasksNoShutdown := map[string]uint64{}
	for _, task := range tasks {
		if task.DesiredState != swarm.TaskStateShutdown {
			tasksNoShutdown[task.ServiceID]++
		}

		if task.Status.State == swarm.TaskStateRunning {
			running[task.ServiceID]++
		}
	}

	for _, service := range services {
		tags := map[string]string{
			"engine_host":    ins.engineHost,
			"server_version": ins.serverVersion,
		}
		fields := make(map[string]interface{})
		tags["service_id"] = service.ID
		tags["service_name"] = service.Spec.Name
		if service.Spec.Mode.Replicated != nil && service.Spec.Mode.Replicated.Replicas != nil {
			tags["service_mode"] = "replicated"
			fields["tasks_running"] = running[service.ID]
			fields["tasks_desired"] = *service.Spec.Mode.Replicated.Replicas
		} else if service.Spec.Mode.Global != nil {
			tags["service_mode"] = "global"
			fields["tasks_running"] = running[service.ID]
			fields["tasks_desired"] = tasksNoShutdown[service.ID]
		} else {
			log.Println("E! Unknown replica mode")
		}

		slist.PushSamples("docker_swarm", fields, tags)
	}
}

func (ins *Instance) gatherInfo(slist *itypes.SampleList) error {
	// Get info from docker daemon
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	info, err := ins.client.Info(ctx)
	if err == context.DeadlineExceeded {
		return errors.New("timeout")
	}
	if err != nil {
		return err
	}

	fields := map[string]interface{}{
		"docker_n_cpus":                  info.NCPU,
		"docker_n_used_file_descriptors": info.NFd,
		"docker_n_containers":            info.Containers,
		"docker_n_containers_running":    info.ContainersRunning,
		"docker_n_containers_stopped":    info.ContainersStopped,
		"docker_n_containers_paused":     info.ContainersPaused,
		"docker_n_images":                info.Images,
		"docker_memory_total":            info.MemTotal,
	}
	ins.engineHost = info.Name
	ins.serverVersion = info.ServerVersion

	slist.PushSamples("", fields, map[string]string{
		"engine_host":    ins.engineHost,
		"server_version": ins.serverVersion,
	})
	return nil
}

func (ins *Instance) getNewClient() (Client, error) {
	if ins.Endpoint == "ENV" {
		return NewEnvClient()
	}

	tlsConfig, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	c, err := NewClient(ins.Endpoint, tlsConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()
	if _, err := c.Ping(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (ins *Instance) createContainerFilters() error {
	containerFilter, err := filter.NewIncludeExcludeFilter(ins.ContainerInclude, ins.ContainerExclude)
	if err != nil {
		return err
	}
	ins.containerFilter = containerFilter
	return nil
}

func (ins *Instance) createLabelFilters() error {
	labelFilter, err := filter.NewIncludeExcludeFilter(ins.LabelInclude, ins.LabelExclude)
	if err != nil {
		return err
	}
	ins.labelFilter = labelFilter
	return nil
}

func (ins *Instance) createContainerStateFilters() error {
	if len(ins.ContainerStateInclude) == 0 && len(ins.ContainerStateExclude) == 0 {
		ins.ContainerStateInclude = []string{"running"}
	}
	stateFilter, err := filter.NewIncludeExcludeFilter(ins.ContainerStateInclude, ins.ContainerStateExclude)
	if err != nil {
		return err
	}
	ins.stateFilter = stateFilter
	return nil
}

func hostnameFromID(id string) string {
	if len(id) > 12 {
		return id[0:12]
	}
	return id
}

// Parses the human-readable size string into the amount it represents.
// func parseSize(sizeStr string) (int64, error) {
// 	matches := sizeRegex.FindStringSubmatch(sizeStr)
// 	if len(matches) != 4 {
// 		return -1, fmt.Errorf("invalid size: %s", sizeStr)
// 	}

// 	size, err := strconv.ParseFloat(matches[1], 64)
// 	if err != nil {
// 		return -1, err
// 	}

// 	uMap := map[string]int64{"k": KB, "m": MB, "g": GB, "t": TB, "p": PB}
// 	unitPrefix := strings.ToLower(matches[3])
// 	if mul, ok := uMap[unitPrefix]; ok {
// 		size *= float64(mul)
// 	}

// 	return int64(size), nil
// }
