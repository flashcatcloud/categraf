package gnmi

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// A bounded limiter avoids both log floods and an unbounded map of device paths.
type tagWarningLimiter struct {
	mu   sync.Mutex
	next time.Time
}

func (l *tagWarningLimiter) warn(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := time.Now(); !now.Before(l.next) {
		l.next = now.Add(time.Minute)
		log.Printf(format, args...)
	}
}

func sameTagSegments(a, b *pathInfo) bool {
	if a == nil || b == nil || len(a.segments) != len(b.segments) {
		return false
	}
	for i := range a.segments {
		if a.segments[i] != b.segments[i] {
			return false
		}
	}
	return true
}

func (pi *pathInfo) keysAt(index int) map[string]string {
	for _, segment := range pi.keyValues {
		if segment.index == index {
			return segment.kv
		}
	}
	return nil
}

// An omitted selector accepts every list entry; '*' accepts any present key.
func (pi *pathInfo) matchesTagKeys(actual *pathInfo) bool {
	for _, segment := range pi.keyValues {
		keys := actual.keysAt(segment.index)
		for key, expected := range segment.kv {
			value, exists := keys[key]
			if !exists || expected != "*" && expected != value {
				return false
			}
		}
	}
	return true
}

// Two selectors are disjoint when they require different literal values for
// the same key on the same element. Partial selectors may still overlap.
func tagKeysOverlap(a, b *pathInfo) bool {
	for _, segment := range a.keyValues {
		keys := b.keysAt(segment.index)
		for key, value := range segment.kv {
			if other, exists := keys[key]; exists && value != "*" && other != "*" && value != other {
				return false
			}
		}
	}
	return true
}

// Exact explicit-origin matches outrank module compatibility, which outranks
// origin-less catch-all subscriptions. Known explicit conflicts never match.
func tagMatchRank(configured, actual *pathInfo) int {
	if !sameTagSegments(configured, actual) || !configured.matchesTagKeys(actual) {
		return 0
	}
	return configured.sourceMatchRank(actual)
}

func (h *handler) selectTagSubscription(actual *pathInfo) (int, error) {
	selected, best, ambiguous := -1, 0, false
	pathCandidate := false
	for i := range h.tagsubs {
		p := h.tagsubs[i].sourcePath
		pathCandidate = pathCandidate || sameTagSegments(p, actual) && p.matchesTagKeys(actual)
		rank := tagMatchRank(p, actual)
		if rank > best {
			selected, best, ambiguous = i, rank, false
		} else if rank > 0 && rank == best {
			ambiguous = true
		}
	}
	if ambiguous {
		return -1, fmt.Errorf("ambiguous tag source for %s (explicit_origin=%t module=%q)", actual, actual.explicitOrigin, actual.inlineModule)
	}
	if selected < 0 && pathCandidate {
		return -1, fmt.Errorf("tag source mismatch for %s (explicit_origin=%t module=%q)", actual, actual.explicitOrigin, actual.inlineModule)
	}
	return selected, nil
}

func validateTagSubscriptions(subs []TagSubscription) error {
	for i := range subs {
		for j := 0; j < i; j++ {
			a, b := subs[i].sourcePath, subs[j].sourcePath
			if !sameTagSegments(a, b) || !tagKeysOverlap(a, b) {
				continue
			}
			// Equal explicit origins can both match even if only one specifies
			// a module. Other module-only duplicates have identical sources.
			overlap := a.explicitOrigin && b.explicitOrigin && a.origin == b.origin && (a.inlineModule == "" || b.inlineModule == "" || a.inlineModule == b.inlineModule)
			if overlap || a.explicitOrigin == b.explicitOrigin && a.origin == b.origin && a.inlineModule == b.inlineModule {
				return fmt.Errorf("ambiguous tag subscriptions %q and %q for %s", subs[j].Name, subs[i].Name, a)
			}
		}
	}
	return nil
}

// JSON tuple encoding makes source and subscription identities collision-free.
func tagIdentity(parts ...interface{}) string {
	data, err := json.Marshal(parts)
	if err != nil {
		panic(err)
	} // Only JSON-serializable path metadata is passed.
	return string(data)
}
