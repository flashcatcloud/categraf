package snmp_zabbix

import (
	"sync"

	"github.com/dop251/goja"
)

// JSCache 用于缓存已编译的goja程序
type JSCache struct {
	cache map[string]*goja.Program
	mu    sync.RWMutex
}

// NewJSCache 创建一个新的JavaScript编译缓存实例
func NewJSCache() *JSCache {
	return &JSCache{
		cache: make(map[string]*goja.Program),
	}
}

// Get 从缓存中获取一个已编译的程序。如果缓存未命中，它将编译脚本并存入缓存。
func (c *JSCache) Get(script string) (*goja.Program, error) {
	// 首先尝试读锁定，这是最高效的路径
	c.mu.RLock()
	program, exists := c.cache[script]
	c.mu.RUnlock()

	if exists {
		return program, nil
	}

	// 如果未命中，切换到写锁定
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查，防止在获取写锁的过程中其他goroutine已经完成了编译
	program, exists = c.cache[script]
	if exists {
		return program, nil
	}

	// 编译脚本
	compiled, err := goja.Compile("zabbix_script", script, false)
	if err != nil {
		return nil, err
	}

	// 存入缓存
	c.cache[script] = compiled
	return compiled, nil
}
