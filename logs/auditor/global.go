//go:build !no_logs

package auditor

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/logs/util"
	"github.com/bmatcuk/doublestar/v4"
)

// SourceMeta holds per-source runtime metadata for metrics labeling.
type SourceMeta struct {
	ConfigPath string
	SourceType string
	Source     string
	Service    string
	Tags       map[string]string
}

type registeredCollector struct {
	auditor Auditor
	metaFn  func() []SourceMeta
}

var (
	globalMu         sync.RWMutex
	globalCollectors []registeredCollector
)

// RegisterCollector registers an auditor with its dynamic source metadata loader callback.
func RegisterCollector(a Auditor, metaFn func() []SourceMeta) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalCollectors = append(globalCollectors, registeredCollector{
		auditor: a,
		metaFn:  metaFn,
	})
}

// UnregisterCollector removes an auditor from global registry.
func UnregisterCollector(a Auditor) {
	globalMu.Lock()
	defer globalMu.Unlock()
	for i, c := range globalCollectors {
		if c.auditor == a {
			globalCollectors = append(globalCollectors[:i], globalCollectors[i+1:]...)
			return
		}
	}
}

// LogFileEntry represents a single tracked log entry for metrics.
type LogFileEntry struct {
	Path        string
	SourceType  string
	TailingMode string
	Offset      string
	LastUpdated time.Time
	Source      string
	Service     string
	Tags        map[string]string
}

// CollectorStatus holds aggregated log collector status.
type CollectorStatus struct {
	Running      bool
	TrackedFiles int
	Entries      []LogFileEntry
}

// GetCollectorStatus returns all tracked entries from all registered auditors.
func GetCollectorStatus() CollectorStatus {
	globalMu.RLock()
	defer globalMu.RUnlock()

	if len(globalCollectors) == 0 {
		return CollectorStatus{}
	}

	stat := CollectorStatus{Running: true}
	for _, c := range globalCollectors {
		allEntries := c.auditor.GetAllEntries()
		if allEntries == nil {
			continue
		}
		stat.TrackedFiles += len(allEntries)

		var fnMeta []SourceMeta
		if c.metaFn != nil {
			fnMeta = c.metaFn()
		}

		for id, entry := range allEntries {
			resolvedPath := id
			sourceType := "unknown"
			if strings.HasPrefix(id, "file:") {
				sourceType = "file"
				resolvedPath = strings.TrimPrefix(id, "file:")
			} else if strings.HasPrefix(id, "journald:") {
				sourceType = "journald"
				resolvedPath = strings.TrimPrefix(id, "journald:")
			}

			e := LogFileEntry{
				Path:        resolvedPath,
				SourceType:  sourceType,
				TailingMode: entry.TailingMode,
				Offset:      entry.Offset,
				LastUpdated: entry.LastUpdated,
			}

			if meta := matchItemMeta(resolvedPath, fnMeta); meta != nil {
				e.Source = meta.Source
				e.Service = meta.Service
				e.Tags = meta.Tags
			}

			stat.Entries = append(stat.Entries, e)
		}
	}
	return stat
}

// matchItemMeta finds the SourceMeta whose ConfigPath matches the resolved path.
func matchItemMeta(resolvedPath string, items []SourceMeta) *SourceMeta {
	for i := range items {
		configPath := items[i].ConfigPath
		if configPath == "" {
			continue
		}

		if util.ContainsDatePattern(configPath) {
			configPath = util.ExpandDatePattern(configPath, time.Now())
		}

		if configPath == resolvedPath {
			return &items[i]
		}

		configSlash := filepath.ToSlash(configPath)
		resolvedSlash := filepath.ToSlash(resolvedPath)
		if matched, _ := doublestar.Match(configSlash, resolvedSlash); matched {
			return &items[i]
		}
	}
	return nil
}

// ParseTags parses tag strings into a map, flexibly splitting by the first '=' or ':'.
func ParseTags(tags []string) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}

		iEq := strings.IndexRune(tag, '=')
		iColon := strings.IndexRune(tag, ':')
		idx := -1
		if iEq >= 0 && iColon >= 0 {
			if iEq < iColon {
				idx = iEq
			} else {
				idx = iColon
			}
		} else if iEq >= 0 {
			idx = iEq
		} else if iColon >= 0 {
			idx = iColon
		}

		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(tag[:idx])
		value := strings.TrimSpace(tag[idx+1:])
		if key == "" {
			continue
		}
		result[key] = value
	}
	return result
}
