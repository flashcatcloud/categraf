package switch_legacy

import (
	"sync"

	"github.com/gaochao1/sw"
)

type LastifMap struct {
	lock   *sync.RWMutex
	ifstat map[string][]sw.IfStats
}

func NewLastifMap() *LastifMap {
	return &LastifMap{
		lock:   new(sync.RWMutex),
		ifstat: make(map[string][]sw.IfStats),
	}
}

func (m *LastifMap) Get(k string) []sw.IfStats {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if val, ok := m.ifstat[k]; ok {
		return val
	}
	return nil
}

func (m *LastifMap) Set(k string, v []sw.IfStats) {
	m.lock.Lock()
	m.ifstat[k] = v
	m.lock.Unlock()
}

func (m *LastifMap) Check(k string) bool {
	m.lock.RLock()
	_, ok := m.ifstat[k]
	m.lock.RUnlock()
	return ok
}
