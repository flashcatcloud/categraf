package snmp_zabbix

import (
	"fmt"
	"sync"
)

// LabelCache 用于并发安全地存储和检索从CHAR/TEXT类型监控项采集到的标签
type LabelCache struct {
	mu    sync.RWMutex
	cache map[string]map[string]string // key: compositeKey, value: map[labelKey]labelValue
}

// NewLabelCache 创建一个新的LabelCache实例
func NewLabelCache() *LabelCache {
	return &LabelCache{
		cache: make(map[string]map[string]string),
	}
}

// buildCacheKey 根据agent, 规则和索引生成唯一的缓存键
func buildCacheKey(agent, ruleKey, index string) string {
	return fmt.Sprintf("%s|%s|%s", agent, ruleKey, index)
}

// Set 存储一个标签
func (lc *LabelCache) Set(agent, ruleKey, index, labelKey, labelValue string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	compositeKey := buildCacheKey(agent, ruleKey, index)
	if _, ok := lc.cache[compositeKey]; !ok {
		lc.cache[compositeKey] = make(map[string]string)
	}
	lc.cache[compositeKey][labelKey] = labelValue
}

// Get 获取与某个发现实例关联的所有标签
func (lc *LabelCache) Get(agent, ruleKey, index string) map[string]string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	compositeKey := buildCacheKey(agent, ruleKey, index)
	if labels, ok := lc.cache[compositeKey]; ok {
		// 返回副本以避免外部修改
		result := make(map[string]string, len(labels))
		for k, v := range labels {
			result[k] = v
		}
		return result
	}
	return nil
}

// DeleteLabel 移除一个特定的标签。如果这是该索引下最后一个标签，则整个条目被移除。
func (lc *LabelCache) DeleteLabel(agent, ruleKey, index, labelKey string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	compositeKey := buildCacheKey(agent, ruleKey, index)
	if labels, ok := lc.cache[compositeKey]; ok {
		delete(labels, labelKey)
		if len(labels) == 0 {
			delete(lc.cache, compositeKey)
		}
	}
}
