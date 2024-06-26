//go:build linux

package ethtool

import (
	"log"
	"os"
	"path/filepath"

	"github.com/vishvananda/netns"
)

type Command interface {
	Init() error
	DriverName(intf NamespacedInterface) (string, error)
	Interfaces(includeNamespaces bool) ([]NamespacedInterface, error)
	Stats(intf NamespacedInterface) (map[string]uint64, error)
	Get(intf NamespacedInterface) (map[string]uint64, error)
}

type CommandEthtool struct {
	namespaceGoroutines map[string]*NamespaceGoroutine
}

func NewCommandEthtool() *CommandEthtool {
	return &CommandEthtool{}
}

func (c *CommandEthtool) Init() error {
	// Create the goroutine for the initial namespace
	initialNamespace, err := netns.Get()
	if err != nil {
		return err
	}
	namespaceGoroutine := &NamespaceGoroutine{
		name:   "",
		handle: initialNamespace,
	}
	if err := namespaceGoroutine.Start(); err != nil {
		log.Println("E! Failed to start goroutine for the initial namespace: ", err)
		return err
	}
	c.namespaceGoroutines = map[string]*NamespaceGoroutine{
		"": namespaceGoroutine,
	}
	return nil
}

func (c *CommandEthtool) DriverName(intf NamespacedInterface) (driver string, err error) {
	return intf.Namespace.DriverName(intf)
}

func (c *CommandEthtool) Stats(intf NamespacedInterface) (stats map[string]uint64, err error) {
	return intf.Namespace.Stats(intf)
}

func (c *CommandEthtool) Get(intf NamespacedInterface) (stats map[string]uint64, err error) {
	return intf.Namespace.Get(intf)
}

func (c *CommandEthtool) Interfaces(includeNamespaces bool) ([]NamespacedInterface, error) {
	const namespaceDirectory = "/var/run/netns"

	initialNamespace, err := netns.Get()
	if err != nil {
		log.Println("E! Could not get initial namespace: ", err)
		return nil, err
	}
	defer initialNamespace.Close()

	// Gather the list of namespace names to from which to retrieve interfaces.
	initialNamespaceIsNamed := false
	var namespaceNames []string
	// Handles are only used to create namespaced goroutines. We don't prefill
	// with the handle for the initial namespace because we've already created
	// its goroutine in Init().
	handles := map[string]netns.NsHandle{}

	if includeNamespaces {
		namespaces, err := os.ReadDir(namespaceDirectory)
		if err != nil {
			log.Println("W! Could not find namespace directory: ", err)
		}

		// We'll always have at least the initial namespace, so add one to ensure
		// we have capacity for it.
		namespaceNames = make([]string, 0, len(namespaces)+1)
		for _, namespace := range namespaces {
			name := namespace.Name()
			namespaceNames = append(namespaceNames, name)

			handle, err := netns.GetFromPath(filepath.Join(namespaceDirectory, name))
			if err != nil {
				log.Printf("W! Could not get handle for namespace [%q]: [%s]", name, err.Error())
				continue
			}
			handles[name] = handle
			if handle.Equal(initialNamespace) {
				initialNamespaceIsNamed = true
			}
		}
	}

	// We don't want to gather interfaces from the same namespace twice, and
	// it's possible, though unlikely, that the initial namespace is also a
	// named interface.
	if !initialNamespaceIsNamed {
		namespaceNames = append(namespaceNames, "")
	}

	allInterfaces := make([]NamespacedInterface, 0)
	for _, namespace := range namespaceNames {
		if _, ok := c.namespaceGoroutines[namespace]; !ok {
			c.namespaceGoroutines[namespace] = &NamespaceGoroutine{
				name:   namespace,
				handle: handles[namespace],
			}
			if err := c.namespaceGoroutines[namespace].Start(); err != nil {
				log.Printf("E! Failed to start goroutine for namespace [%q]: [%s]", namespace, err.Error())
				delete(c.namespaceGoroutines, namespace)
				continue
			}
		}

		interfaces, err := c.namespaceGoroutines[namespace].Interfaces()
		if err != nil {
			log.Printf("W! Could not get interfaces from namespace [%q]: [%s]", namespace, err.Error())
			continue
		}
		allInterfaces = append(allInterfaces, interfaces...)
	}

	return allInterfaces, nil
}
