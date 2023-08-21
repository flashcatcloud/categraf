package config

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type HostInfoCache struct {
	name string
	ip   string
	sync.RWMutex
}

var HostInfo *HostInfoCache

func (c *HostInfoCache) GetHostname() string {
	c.RLock()
	n := c.name
	c.RUnlock()
	return n
}

func (c *HostInfoCache) GetIP() string {
	c.RLock()
	defer c.RUnlock()
	ip := c.ip
	return ip
}

func (c *HostInfoCache) SetHostname(name string) {
	if name == c.GetHostname() {
		return
	}

	c.Lock()
	c.name = name
	c.Unlock()
}

func (c *HostInfoCache) SetIP(ip string) {
	if ip == c.GetIP() {
		return
	}

	c.Lock()
	c.ip = ip
	c.Unlock()
}

func InitHostInfo() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	ip, err := GetOutboundIP()
	if err != nil {
		return err
	}

	HostInfo = &HostInfoCache{
		name: hostname,
		ip:   fmt.Sprint(ip),
	}

	go HostInfo.update()

	return nil
}

func (c *HostInfoCache) update() {
	for {
		time.Sleep(time.Minute)
		name, err := os.Hostname()
		if err != nil {
			log.Println("E! failed to get hostname:", err)
		} else {
			HostInfo.SetHostname(name)
		}
		ip, err := GetOutboundIP()
		if err != nil {
			log.Println("E! failed to get ip:", err)
		} else {
			HostInfo.SetIP(fmt.Sprint(ip))
		}
	}
}
