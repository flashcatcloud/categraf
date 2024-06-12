//go:build linux

package ethtool

import (
	"log"
	"math"
	"net"
	"runtime"

	ethtoolLib "github.com/safchain/ethtool"
	"github.com/vishvananda/netns"
)

type Namespace interface {
	Name() string
	Interfaces() ([]NamespacedInterface, error)
	DriverName(intf NamespacedInterface) (string, error)
	Stats(intf NamespacedInterface) (map[string]uint64, error)
	Get(intf NamespacedInterface) (map[string]uint64, error)
}

type NamespacedInterface struct {
	net.Interface
	Namespace Namespace
}

type NamespacedAction struct {
	result chan<- NamespacedResult
	f      func(*NamespaceGoroutine) (interface{}, error)
}

type NamespacedResult struct {
	Result interface{}
	Error  error
}

type NamespaceGoroutine struct {
	name          string
	handle        netns.NsHandle
	ethtoolClient *ethtoolLib.Ethtool
	c             chan NamespacedAction
}

func (n *NamespaceGoroutine) Name() string {
	return n.name
}

// Interfaces get a list of the system's network interfaces.
func (n *NamespaceGoroutine) Interfaces() ([]NamespacedInterface, error) {
	interfaces, err := n.Do(func(n *NamespaceGoroutine) (interface{}, error) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return nil, err
		}
		namespacedInterfaces := make([]NamespacedInterface, 0, len(interfaces))
		for _, iface := range interfaces {
			namespacedInterfaces = append(
				namespacedInterfaces,
				NamespacedInterface{
					Interface: iface,
					Namespace: n,
				},
			)
		}
		return namespacedInterfaces, nil
	})

	return interfaces.([]NamespacedInterface), err
}

// DriverName get the driver name of the given interface name.
func (n *NamespaceGoroutine) DriverName(intf NamespacedInterface) (string, error) {
	driver, err := n.Do(func(n *NamespaceGoroutine) (interface{}, error) {
		return n.ethtoolClient.DriverName(intf.Name)
	})
	return driver.(string), err
}

// Stats get stats of the given interface name.
func (n *NamespaceGoroutine) Stats(intf NamespacedInterface) (map[string]uint64, error) {
	driver, err := n.Do(func(n *NamespaceGoroutine) (interface{}, error) {
		return n.ethtoolClient.Stats(intf.Name)
	})
	return driver.(map[string]uint64), err
}

// Get return information about a network interface's speed, duplex mode, autonegotiation status, and link state
func (n *NamespaceGoroutine) Get(intf NamespacedInterface) (map[string]uint64, error) {
	result, err := n.Do(func(n *NamespaceGoroutine) (interface{}, error) {
		ecmd := ethtoolLib.EthtoolCmd{}
		speed32, err := n.ethtoolClient.CmdGet(&ecmd, intf.Name)
		if err != nil {
			return nil, err
		}

		var speed = uint64(speed32)
		if speed == math.MaxUint32 {
			speed = math.MaxUint64
		}

		var link32 uint32
		link32, err = n.ethtoolClient.LinkState(intf.Name)
		if err != nil {
			return nil, err
		}

		return map[string]uint64{
			"speed":   speed,
			"duplex":  uint64(ecmd.Duplex),
			"autoneg": uint64(ecmd.Autoneg),
			"link":    uint64(link32),
		}, nil
	})

	if result != nil {
		return result.(map[string]uint64), err
	}
	return nil, err
}

// Start locks a goroutine to an OS thread and ties it to the namespace, then
// loops for actions to run in the namespace.
func (n *NamespaceGoroutine) Start() error {
	n.c = make(chan NamespacedAction)
	started := make(chan error)
	go func() {
		// We're going to hold this thread locked permanently. We're going to
		// do this for every namespace. This makes it very likely that the Go
		// runtime will spin up new threads to replace it. To avoid thread
		// leaks, we don't unlock when we're done and instead let this thread
		// die.
		runtime.LockOSThread()

		// If this goroutine is for the initial namespace, we are already in
		// the correct namespace. Switching would require CAP_SYS_ADMIN, which
		// we may not have. Don't switch if the desired namespace matches the
		// current one.
		initialNamespace, err := netns.Get()
		if err != nil {
			log.Println("E! Could not get initial namespace: ", err)
			started <- err
			return
		}
		if !initialNamespace.Equal(n.handle) {
			if err := netns.Set(n.handle); err != nil {
				log.Printf("E! Could not switch to namespace [%q]: [%s]", n.name, err.Error())
				started <- err
				return
			}
		}

		// Every namespace needs its own connection to ethtool
		e, err := ethtoolLib.NewEthtool()
		if err != nil {
			log.Printf("E! Could not create ethtool client for namespace [%q]: [%s]", n.name, err.Error())
			started <- err
			return
		}
		n.ethtoolClient = e
		started <- nil
		for command := range n.c {
			result, err := command.f(n)
			command.result <- NamespacedResult{
				Result: result,
				Error:  err,
			}
			close(command.result)
		}
	}()
	return <-started
}

// Do runs a function inside the OS thread tied to the namespace.
func (n *NamespaceGoroutine) Do(f func(*NamespaceGoroutine) (interface{}, error)) (interface{}, error) {
	result := make(chan NamespacedResult)
	n.c <- NamespacedAction{
		result: result,
		f:      f,
	}
	r := <-result
	return r.Result, r.Error
}
