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

// Exact explicit-origin matches outrank module compatibility, which outranks
// origin-less catch-all subscriptions. Known explicit conflicts never match.
func tagMatchRank(configured, actual *pathInfo) int {
	if !sameTagSegments(configured, actual) {
		return 0
	}
	if configured.inlineModule != "" && actual.inlineModule != "" && configured.inlineModule != actual.inlineModule {
		return 0
	}
	if configured.explicitOrigin {
		if actual.explicitOrigin {
			if configured.origin == actual.origin {
				return 3
			}
			return 0
		}
		if configured.inlineModule != "" && configured.inlineModule == actual.inlineModule {
			return 2
		}
		return 0
	}
	if configured.inlineModule != "" {
		if configured.inlineModule == actual.inlineModule {
			return 2
		}
		if actual.explicitOrigin && configured.origin == actual.origin {
			return 2
		}
		return 0
	}
	return 1
}

func (h *handler) selectTagSubscription(actual *pathInfo) (int, error) {
	selected, best, ambiguous := -1, 0, false
	pathCandidate := false
	for i := range h.tagsubs {
		p := h.tagsubs[i].sourcePath
		pathCandidate = pathCandidate || sameTagSegments(p, actual)
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
			if !sameTagSegments(a, b) {
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
	} // Only strings, booleans and string slices are passed.
	return string(data)
}
