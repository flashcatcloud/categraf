//go:build linux

package ethtool

import (
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/types"
)

const inputName = "ethtool"

const (
	tagInterface     = "interface"
	tagNamespace     = "namespace"
	tagDriverName    = "driver"
	fieldInterfaceUp = "interface_up"
)

var downInterfacesBehaviors = []string{"expose", "skip"}

type Ethtool struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Ethtool{}
	})
}

func (pt *Ethtool) Clone() inputs.Input {
	return &Ethtool{}
}

func (pt *Ethtool) Name() string {
	return inputName
}

func (pt *Ethtool) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	// This is the list of interface names to include
	InterfaceInclude []string `toml:"interface_include"`

	// This is the list of interface names to ignore
	InterfaceExclude []string `toml:"interface_exclude"`

	// Behavior regarding metrics for downed interfaces
	DownInterfaces string `toml:" down_interfaces"`

	// This is the list of namespace names to include
	NamespaceInclude []string `toml:"namespace_include"`

	// This is the list of namespace names to ignore
	NamespaceExclude []string `toml:"namespace_exclude"`

	// Normalization on the key names
	NormalizeKeys []string `toml:"normalize_keys"`

	interfaceFilter   filter.Filter
	namespaceFilter   filter.Filter
	includeNamespaces bool

	// the ethtool command
	command Command
}

func (ins *Instance) Init() error {
	var err error
	ins.interfaceFilter, err = filter.NewIncludeExcludeFilter(ins.InterfaceInclude, ins.InterfaceExclude)
	if err != nil {
		return err
	}

	if ins.DownInterfaces == "" {
		ins.DownInterfaces = "expose"
	}

	if err = choice.Check(ins.DownInterfaces, downInterfacesBehaviors); err != nil {
		return fmt.Errorf("down_interfaces: %w", err)
	}

	// If no namespace include or exclude filters were provided, then default
	// to just the initial namespace.
	ins.includeNamespaces = len(ins.NamespaceInclude) > 0 || len(ins.NamespaceExclude) > 0
	if len(ins.NamespaceInclude) == 0 && len(ins.NamespaceExclude) == 0 {
		ins.NamespaceInclude = []string{""}
	} else if len(ins.NamespaceInclude) == 0 {
		ins.NamespaceInclude = []string{"*"}
	}
	ins.namespaceFilter, err = filter.NewIncludeExcludeFilter(ins.NamespaceInclude, ins.NamespaceExclude)
	if err != nil {
		return err
	}

	ins.command = NewCommandEthtool()
	if _, ok := ins.command.(*CommandEthtool); !ok {
		errMsg := "Conversion failed"
		log.Println("E! ", errMsg)
		return fmt.Errorf("%v", errMsg)
	}

	return ins.command.Init()
}

func (ins *Instance) Gather(slist *types.SampleList) {
	// Get the list of interfaces
	interfaces, err := ins.command.Interfaces(ins.includeNamespaces)
	if err != nil {
		log.Printf("E! gather interfaces:[%q] error:[%s]", interfaces, err)
		return
	}

	// parallelize the ethtool call in event of many interfaces
	var wg sync.WaitGroup

	for _, iface := range interfaces {
		// Check this isn't a loop back and that its matched by the filter(s)
		if ins.interfaceEligibleForGather(iface) {
			wg.Add(1)

			go func(i NamespacedInterface) {
				ins.gatherEthtoolStats(i, slist)
				wg.Done()
			}(iface)
		}
	}

	// Waiting for all the interfaces
	wg.Wait()
	return
}

func (ins *Instance) interfaceEligibleForGather(iface NamespacedInterface) bool {
	// Don't gather if it is a loop back, or it isn't matched by the filter
	if isLoopback(iface) || !ins.interfaceFilter.Match(iface.Name) {
		return false
	}

	// Don't gather if it's not in a namespace matched by the filter
	if !ins.namespaceFilter.Match(iface.Namespace.Name()) {
		return false
	}

	// For downed interfaces, gather only for "expose"
	if !interfaceUp(iface) {
		return ins.DownInterfaces == "expose"
	}

	return true
}

// Gather the stats for the interface.
func (ins *Instance) gatherEthtoolStats(iface NamespacedInterface, slist *types.SampleList) {
	tags := make(map[string]string)
	tags[tagInterface] = iface.Name
	tags[tagNamespace] = iface.Namespace.Name()

	driverName, err := ins.command.DriverName(iface)
	if err != nil {
		log.Printf("E! [%q] driver: [%s]", iface.Name, err)
		return
	}

	tags[tagDriverName] = driverName

	fields := make(map[string]interface{})
	stats, err := ins.command.Stats(iface)
	if err != nil {
		log.Printf("E! [%q] stats: [%s]", iface.Name, err)
		return
	}

	fields[fieldInterfaceUp] = interfaceUp(iface)
	for k, v := range stats {
		fields[ins.normalizeKey(k)] = v
	}

	cmdget, err := ins.command.Get(iface)
	// error text is directly from running ethtool and syscalls
	if err != nil && err.Error() != "operation not supported" {
		log.Printf("E! [%q] get: [%s]", iface.Name, err)
		return
	}
	for k, v := range cmdget {
		fields[ins.normalizeKey(k)] = v
	}

	slist.PushSamples(inputName, fields, tags)
}

// normalize key string; order matters to avoid replacing whitespace with
// underscores, then trying to trim those same underscores. Likewise with
// camelcase before trying to lower case things.
func (ins *Instance) normalizeKey(key string) string {
	// must trim whitespace or this will have a leading _
	if inStringSlice(ins.NormalizeKeys, "snakecase") {
		key = camelCase2SnakeCase(strings.TrimSpace(key))
	}
	// must occur before underscore, otherwise nothing to trim
	if inStringSlice(ins.NormalizeKeys, "trim") {
		key = strings.TrimSpace(key)
	}
	if inStringSlice(ins.NormalizeKeys, "lower") {
		key = strings.ToLower(key)
	}
	if inStringSlice(ins.NormalizeKeys, "underscore") {
		key = strings.ReplaceAll(key, " ", "_")
	}
	// aws has a conflicting name that needs to be renamed
	if key == "interface_up" {
		key = "interface_up_counter"
	}

	return key
}

func camelCase2SnakeCase(value string) string {
	matchFirstCap := regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap := regexp.MustCompile("([a-z0-9])([A-Z])")

	snake := matchFirstCap.ReplaceAllString(value, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

func inStringSlice(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}

	return false
}

func isLoopback(iface NamespacedInterface) bool {
	return (iface.Flags & net.FlagLoopback) != 0
}

func interfaceUp(iface NamespacedInterface) bool {
	return (iface.Flags & net.FlagUp) != 0
}
