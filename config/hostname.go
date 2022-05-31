package config

import (
	"log"
	"os"
	"sync"
	"time"
)

type HostnameCache struct {
	name string
	sync.RWMutex
}

var Hostname *HostnameCache

func (c *HostnameCache) Get() string {
	c.RLock()
	n := c.name
	c.RUnlock()
	return n
}

func (c *HostnameCache) Set(name string) {
	if name == c.Get() {
		return
	}

	c.Lock()
	c.name = name
	c.Unlock()
}

func InitHostname() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	Hostname = &HostnameCache{
		name: hostname,
	}

	go Hostname.update()

	return nil
}

func (c *HostnameCache) update() {
	for {
		time.Sleep(time.Second)
		name, err := os.Hostname()
		if err != nil {
			log.Println("E! failed to get hostname:", err)
		} else {
			Hostname.Set(name)
		}
	}
}
