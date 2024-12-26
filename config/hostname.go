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
	sn   string
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

func (c *HostInfoCache) GetSN() string {
	c.RLock()
	defer c.RUnlock()
	sn := c.sn
	return sn
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

func (c *HostInfoCache) SetSN(sn string) {
	if sn == c.GetSN() {
		return
	}

	c.Lock()
	c.sn = sn
	c.Unlock()
}

func InitHostInfo() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	var ip string
	if ip = os.Getenv("HOSTIP"); ip == "" {
		nip, err := GetOutboundIP()
		if err != nil {
			return err
		}
		ip = fmt.Sprint(nip)
	}
	var sn string
	// allow sn empty
	sn, _ = GetBiosSn()
	HostInfo = &HostInfoCache{
		name: hostname,
		ip:   fmt.Sprint(ip),
		sn:   sn,
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
		sn, err := GetBiosSn()
		if err == nil {
			HostInfo.SetSN(sn)
		}
	}
}
