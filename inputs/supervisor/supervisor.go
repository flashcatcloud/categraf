package supervisor

import (
	"fmt"
	"log"
	"net"
	"net/url"

	"github.com/kolo/xmlrpc"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const inputName = "supervisor"

type (
	Supervisor struct {
		config.PluginConfig

		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		Url            string   `toml:"url"`
		MetricsInclude []string `toml:"metrics_include"`
		MetricsExclude []string `toml:"metrics_exclude"`

		rpcClient   *xmlrpc.Client
		fieldFilter filter.Filter
	}

	processInfo struct {
		Name          string `xmlrpc:"name"`
		Group         string `xmlrpc:"group"`
		Description   string `xmlrpc:"description"`
		Start         int32  `xmlrpc:"start"`
		Stop          int32  `xmlrpc:"stop"`
		Now           int32  `xmlrpc:"now"`
		State         int16  `xmlrpc:"state"`
		Statename     string `xmlrpc:"statename"`
		StdoutLogfile string `xmlrpc:"stdout_logfile"`
		StderrLogfile string `xmlrpc:"stderr_logfile"`
		SpawnErr      string `xmlrpc:"spawnerr"`
		ExitStatus    int8   `xmlrpc:"exitstatus"`
		Pid           int32  `xmlrpc:"pid"`
	}

	supervisorInfo struct {
		StateCode int8   `xmlrpc:"statecode"`
		StateName string `xmlrpc:"statename"`
		Ident     string
	}
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Supervisor{}
	})
}

func (s *Supervisor) Clone() inputs.Input {
	return &Supervisor{}
}

func (s *Supervisor) Name() string {
	return inputName
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(Supervisor)
var _ inputs.InstancesGetter = new(Supervisor)

func (s *Supervisor) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if ins.Url == "" {
		return types.ErrInstancesEmpty
	}
	var err error
	// Initializing XML-RPC client
	ins.rpcClient, err = xmlrpc.NewClient(ins.Url, nil)
	if err != nil {
		return fmt.Errorf("XML-RPC client initialization failed: %w", err)
	}
	// Setting filter for additional metrics
	ins.fieldFilter, err = filter.NewIncludeExcludeFilter(ins.MetricsInclude, ins.MetricsExclude)
	if err != nil {
		return fmt.Errorf("metrics filter setup failed: %w", err)
	}
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	// API call to get information about all running processes
	var rawProcessData []processInfo
	err := ins.rpcClient.Call("supervisor.getAllProcessInfo", nil, &rawProcessData)
	if err != nil {
		log.Println("failed to get processes info: %w", err)
		return
	}

	// API call to get information about instance status
	var status supervisorInfo
	err = ins.rpcClient.Call("supervisor.getState", nil, &status)
	if err != nil {
		log.Println("failed to get processes info: %w", err)
		return
	}

	// API call to get identification string
	err = ins.rpcClient.Call("supervisor.getIdentification", nil, &status.Ident)
	if err != nil {
		log.Println("failed to get instance identification: %w", err)
		return
	}

	// Iterating through array of structs with processes info and adding fields to accumulator
	for _, process := range rawProcessData {
		processTags, processFields, err := ins.parseProcessData(process, status)
		if err != nil {
			log.Println("E! failed to parse process data: ", err)
			continue
		}
		slist.PushSamples("supervisor_processes", processFields, processTags)
	}

	// Adding instance info fields to accumulator
	instanceTags, instanceFields, err := ins.parseInstanceData(status)
	if err != nil {
		log.Println("failed to parse instance data: %w", err)
		return
	}
	slist.PushSamples("supervisor_instance", instanceFields, instanceTags)
	return
}

func (ins *Instance) parseProcessData(pInfo processInfo, status supervisorInfo) (map[string]string, map[string]interface{}, error) {
	source, port, err := parseServer(ins.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse server string: %w", err)
	}

	tags := map[string]string{
		"process": pInfo.Name,
		"group":   pInfo.Group,
		"id":      status.Ident,
		"source":  source,
		"port":    port,
	}
	fields := map[string]interface{}{
		"uptime": pInfo.Now - pInfo.Start,
		"state":  pInfo.State,
	}
	if ins.fieldFilter.Match("pid") {
		fields["pid"] = pInfo.Pid
	}
	if ins.fieldFilter.Match("exitCode") {
		fields["exitCode"] = pInfo.ExitStatus
	}

	return tags, fields, nil
}

func (ins *Instance) parseInstanceData(status supervisorInfo) (map[string]string, map[string]interface{}, error) {
	source, port, err := parseServer(ins.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse server string: %w", err)
	}

	tags := map[string]string{
		"id":     status.Ident,
		"source": source,
		"port":   port,
	}
	fields := map[string]interface{}{
		"state": status.StateCode,
	}

	return tags, fields, nil
}

// parseServer get only address and port from URL
func parseServer(rowUrl string) (source, port string, err error) {
	parsedURL, err := url.Parse(rowUrl)
	if err != nil {
		return "", "", err
	}
	host, port, splitErr := net.SplitHostPort(parsedURL.Host)
	if splitErr != nil {
		if parsedURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port, nil
}
