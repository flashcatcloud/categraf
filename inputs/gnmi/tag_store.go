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
	source       *pathInfo
	values       map[string]map[string]string
}

var tagLabelReplacer = strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")

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
			key := f.path.segments[len(f.path.segments)-1]
			if len(f.path.segments) > len(path.segments) {
				// Preserve the complete JSON suffix in concatenated labels,
				// including distinct nested fields with the same leaf name.
				key = strings.Join(f.path.segments[len(path.segments):], "/")
			}
			// Keep the existing raw label spelling for instance labels and
			// relabel rules. Historically only suffix hyphens are replaced here.
			name += "/" + strings.ReplaceAll(key, "-", "_")
		}
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
		selectors := make(map[int]map[string]string, len(configured.keyValues))
		for _, segment := range configured.keyValues {
			selectors[segment.index] = segment.kv
		}
		sourceID = tagIdentity(configured.origin, configured.explicitOrigin, configured.inlineModule, configured.segments, selectors)
	} else {
		sourceID = tagIdentity(sub.Origin, sub.Path)
	}
	id := tagIdentity(sourceID, sub.Name, sub.Match, sub.Elements, sub.DisableConcatenation, path.origin, path.explicitOrigin, path.inlineModule)
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.buckets[id]
	if bucket == nil {
		bucket = &tagBucket{
			subscription: sub,
			// The bucket identity includes this metadata. Do not retain the
			// first update's list keys: a bucket contains multiple scopes.
			source: &pathInfo{origin: path.origin, explicitOrigin: path.explicitOrigin, inlineModule: path.inlineModule},
			values: make(map[string]map[string]string),
		}
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
	labels, _ := s.lookupWithSourceKeys(path, metricTags)
	return labels
}

func (s *tagStore) lookupWithSourceKeys(path *pathInfo, metricTags map[string]string) (map[string]string, map[string]bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type tagLabel struct {
		name, value string
		conflict    bool
		sourceKeys  map[string]bool
	}
	labels := make(map[string]tagLabel)
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
			// Compare the final spelling used by Sample.ConvertTimeSeries,
			// while retaining the raw name until label processing has run.
			canonical := tagLabelReplacer.Replace(name)
			label, exists := labels[canonical]
			if exists && label.value != value {
				label.conflict = true
			}
			if !exists || name < label.name {
				// Same-value aliases share one deterministic original spelling.
				label.name, label.value = name, value
			}
			// Deletion requires symmetric source compatibility: an unknown
			// source must not act as a wildcard for a known metric module.
			if key, ok := sub.sourceKeyTag(path, name, value); ok && bucket.source.sourceMatchRank(path) > 0 && path.sourceMatchRank(bucket.source) > 0 {
				if label.sourceKeys == nil {
					label.sourceKeys = make(map[string]bool)
				}
				label.sourceKeys[key] = true
			}
			labels[canonical] = label
		}
	}
	result := make(map[string]string, len(labels))
	drops := make(map[string]bool)
	for canonical, label := range labels {
		if label.conflict {
			s.warnings.warn("W! gNMI %s: tag conflict for label %q; omitting conflicting label (diagnostics limited to once per minute)", s.address, canonical)
			continue
		}
		result[label.name] = label.value
		for key := range label.sourceKeys {
			drops[key] = true
		}
	}
	// A replacement may intentionally reuse an automatic label's spelling.
	// Never remove a label supplied by a successful tag subscription.
	for key := range drops {
		if label, exists := labels[tagLabelReplacer.Replace(key)]; exists && !label.conflict {
			delete(drops, key)
		}
	}
	return result, drops
}

// A source leaf such as interface/name replaces only the identically named
// key on its exact parent list. Full ancestry and origin must match even when
// the subscription's label application scope intentionally spans other paths.
func (sub *TagSubscription) sourceKeyTag(metric *pathInfo, label, value string) (string, bool) {
	source := sub.sourcePath
	if !sub.DropSourceKey || source == nil || len(source.segments) < 2 || source.sourceMatchRank(metric) == 0 {
		return "", false
	}
	parent := len(source.segments) - 2
	if len(metric.segments) <= parent || !source.matchesTagKeys(metric) {
		return "", false
	}
	for i := 0; i <= parent; i++ {
		if source.segments[i] != metric.segments[i] {
			return "", false
		}
	}
	key := source.segments[parent+1]
	expectedLabel := sub.Name
	if !sub.DisableConcatenation {
		expectedLabel += "/" + strings.ReplaceAll(key, "-", "_")
	}
	if label != expectedLabel {
		return "", false
	}
	rawName, keyValue, ok := metric.keyTag(parent, key)
	return rawName, ok && keyValue == value
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
