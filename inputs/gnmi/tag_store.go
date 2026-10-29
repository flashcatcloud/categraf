package gnmi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type tagStore struct {
	address  string
	mu       sync.RWMutex
	buckets  map[string]*tagBucket
	warnings tagWarningLimiter
}

type tagBucket struct {
	subscription TagSubscription
	values       map[string]map[string]string
}

func newTagStore(_ []TagSubscription) *tagStore {
	return &tagStore{buckets: make(map[string]*tagBucket)}
}

// Each source writes and clears only its own subscription/source bucket.
func (s *tagStore) insert(sub TagSubscription, path *pathInfo, values []updateField, tags map[string]string) error {
	scope := ""
	switch sub.Match {
	case "unconditional":
	case "name":
		var ok bool
		scope, ok = tags["name"]
		if !ok {
			return nil
		}
	case "elements":
		var ok bool
		scope, ok = s.getElementsKeys(path, sub.Elements)
		if !ok {
			return nil
		}
	default:
		return fmt.Errorf("unknown match strategy %q", sub.Match)
	}
	// Convert first so a bad field cannot partially overwrite a source batch.
	updates := make(map[string]string, len(values))
	for _, f := range values {
		name := sub.Name
		if len(f.path.segments) > 0 && !sub.DisableConcatenation {
			name += "/" + f.path.segments[len(f.path.segments)-1]
		}
		// Detect conflicts using the same label spelling emitted by types.NewSample.
		name = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_").Replace(name)
		value, err := ToString(f.value)
		if err != nil {
			return fmt.Errorf("conversion error for %v: %w", f.value, err)
		}
		updates[name] = value
	}
	if len(updates) == 0 {
		return nil
	}
	configured := sub.sourcePath
	var sourceID string
	if configured != nil {
		sourceID = tagIdentity(configured.origin, configured.explicitOrigin, configured.inlineModule, configured.segments)
	} else {
		sourceID = tagIdentity(sub.Origin, sub.Path)
	}
	id := tagIdentity(sourceID, sub.Name, sub.Match, sub.Elements, sub.DisableConcatenation, path.origin, path.explicitOrigin, path.inlineModule)
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.buckets[id]
	if bucket == nil {
		bucket = &tagBucket{subscription: sub, values: make(map[string]map[string]string)}
		s.buckets[id] = bucket
	}
	if bucket.values[scope] == nil {
		bucket.values[scope] = make(map[string]string)
	}
	for name, value := range updates {
		if value == "" {
			delete(bucket.values[scope], name)
		} else {
			bucket.values[scope][name] = value
		}
	}
	if len(bucket.values[scope]) == 0 {
		delete(bucket.values, scope)
	}
	if len(bucket.values) == 0 {
		delete(s.buckets, id)
	}
	return nil
}

// Origin identifies a tag source, not the metrics it applies to. Existing
// unconditional/name/elements application scopes intentionally span modules.
func (s *tagStore) lookup(path *pathInfo, metricTags map[string]string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string)
	conflicts := make(map[string]bool)
	for _, bucket := range s.buckets {
		sub := bucket.subscription
		scope := ""
		switch sub.Match {
		case "unconditional":
		case "name":
			var ok bool
			scope, ok = metricTags["name"]
			if !ok {
				continue
			}
		case "elements":
			var ok bool
			scope, ok = s.getElementsKeys(path, sub.Elements)
			if !ok {
				continue
			}
		}
		for name, value := range bucket.values[scope] {
			if old, ok := result[name]; ok && old != value {
				conflicts[name] = true
			}
			result[name] = value
		}
	}
	for name := range conflicts {
		delete(result, name)
		s.warnings.warn("W! gNMI %s: tag conflict for label %q; omitting conflicting label (diagnostics limited to once per minute)", s.address, name)
	}
	return result
}

func (s *tagStore) getElementsKeys(path *pathInfo, elements []string) (string, bool) {
	// Search for the required path elements and collect a ordered
	// list of their values to in the form
	//    elementName1={keyA=valueA,keyB=valueB,...},...,elementNameN={keyY=valueY,keyZ=valueZ}
	// where each elements' key-value list is enclosed in curly brackets.
	keyParts := make([]string, 0, len(elements))
	for _, requiredElement := range elements {
		var found bool
		var elementKVs []string
		for _, segment := range path.keyValues {
			if segment.name == requiredElement {
				for k, v := range segment.kv {
					elementKVs = append(elementKVs, k+"="+v)
				}
				found = true
				break
			}
		}

		// The element was not found, but all must match
		if !found {
			return "", false
		}

		// We need to order the element's key-value pairs as the map
		// returns elements in random order
		sort.Strings(elementKVs)

		// Collect the element
		keyParts = append(keyParts, requiredElement+"={"+strings.Join(elementKVs, ",")+"}")
	}
	return strings.Join(keyParts, ","), true
}
func ToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int8:
		return strconv.FormatInt(int64(v), 10), nil
	case int16:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case fmt.Stringer:
		return v.String(), nil
	case nil:
		return "", nil
	}
	return "", fmt.Errorf("type \"%T\" unsupported", value)
}
