package gnmi

import (
	"regexp"
	"strings"

	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
)

// Regular expression to see if a path element contains an origin
var originPattern = regexp.MustCompile(`^([\w-]+):`)

type keySegment struct {
	name string
	path string
	kv   map[string]string
	// index identifies the path element even when node names repeat.
	index int
}

type pathInfo struct {
	origin string
	// Source metadata is internal; origin may also be inferred from a module.
	explicitOrigin bool
	inlineModule   string
	target         string
	segments       []string
	keyValues      []keySegment
}

func newInfoFromString(path string) *pathInfo {
	if path == "" {
		return &pathInfo{}
	}

	info := &pathInfo{}
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			info.segments = append(info.segments, s)
		}
	}
	info.normalize()

	return info
}

func newInfoFromPathWithoutKeys(path *gnmiLib.Path) *pathInfo {
	info := &pathInfo{
		origin:         path.Origin,
		explicitOrigin: path.Origin != "",
		segments:       make([]string, 0, len(path.Elem)),
	}
	for _, elem := range path.Elem {
		if elem.Name == "" {
			continue
		}
		info.segments = append(info.segments, elem.Name)
	}
	info.normalize()

	return info
}

func newInfoFromPath(paths ...*gnmiLib.Path) *pathInfo {
	if len(paths) == 0 {
		return nil
	}

	info := &pathInfo{}
	if paths[0] != nil {
		info.origin = paths[0].Origin
		info.explicitOrigin = paths[0].Origin != ""
		info.target = paths[0].Target
	}

	for _, p := range paths {
		if p == nil {
			continue
		}
		if !info.explicitOrigin && p.Origin != "" {
			info.origin = p.Origin
			info.explicitOrigin = true
		}
		for _, elem := range p.Elem {
			if elem.Name == "" {
				continue
			}
			info.segments = append(info.segments, elem.Name)

			if len(elem.Key) == 0 {
				continue
			}
			keyInfo := keySegment{
				name:  elem.Name,
				path:  info.String(),
				kv:    make(map[string]string, len(elem.Key)),
				index: len(info.segments) - 1,
			}
			for k, v := range elem.Key {
				keyInfo.kv[k] = v
			}
			info.keyValues = append(info.keyValues, keyInfo)
		}
	}
	info.normalize()

	return info
}

func (pi *pathInfo) empty() bool {
	return len(pi.segments) == 0
}

func (pi *pathInfo) append(paths ...*gnmiLib.Path) *pathInfo {
	// Copy the existing info
	path := &pathInfo{
		origin:         pi.origin,
		explicitOrigin: pi.explicitOrigin,
		inlineModule:   pi.inlineModule,
		target:         pi.target,
		segments:       append([]string{}, pi.segments...),
		keyValues:      make([]keySegment, 0, len(pi.keyValues)),
	}
	for _, elem := range pi.keyValues {
		keyInfo := keySegment{
			name:  elem.name,
			path:  elem.path,
			kv:    make(map[string]string, len(elem.kv)),
			index: elem.index,
		}
		for k, v := range elem.kv {
			keyInfo.kv[k] = v
		}
		path.keyValues = append(path.keyValues, keyInfo)
	}

	// Add the new segments
	for _, p := range paths {
		// Inherit a standalone gNMI origin (set in Path.Origin rather than as
		// an inline "module:" prefix) carried by an appended path, e.g. when the
		// device places the origin on the update path instead of the prefix.
		// An explicit prefix origin wins; an inferred module origin can be
		// replaced by the update's explicit origin without losing its module.
		if !path.explicitOrigin && p.Origin != "" {
			path.origin = p.Origin
			path.explicitOrigin = true
		}
		for _, elem := range p.Elem {
			if elem.Name == "" {
				continue
			}
			path.segments = append(path.segments, elem.Name)

			if len(elem.Key) == 0 {
				continue
			}
			keyInfo := keySegment{
				name:  elem.Name,
				path:  path.String(),
				kv:    make(map[string]string, len(elem.Key)),
				index: len(path.segments) - 1,
			}
			for k, v := range elem.Key {
				keyInfo.kv[k] = v
			}
			path.keyValues = append(path.keyValues, keyInfo)
		}
	}

	// Some devices supply the origin as part of the first path element
	// (e.g. private YANG modules using an inline "module:" prefix), so
	// normalize the assembled path to extract it.
	path.normalize()

	return path
}

func (pi *pathInfo) appendSegments(segments ...string) *pathInfo {
	// Copy the existing info
	path := &pathInfo{
		origin:         pi.origin,
		explicitOrigin: pi.explicitOrigin,
		inlineModule:   pi.inlineModule,
		target:         pi.target,
		segments:       append([]string{}, pi.segments...),
		keyValues:      make([]keySegment, 0, len(pi.keyValues)),
	}
	for _, elem := range pi.keyValues {
		keyInfo := keySegment{
			name:  elem.name,
			path:  elem.path,
			kv:    make(map[string]string, len(elem.kv)),
			index: elem.index,
		}
		for k, v := range elem.kv {
			keyInfo.kv[k] = v
		}
		path.keyValues = append(path.keyValues, keyInfo)
	}

	// Add the new segments
	for _, s := range segments {
		if s == "" {
			continue
		}
		path.segments = append(path.segments, s)
	}

	// Some devices supply the origin as part of the first path element
	// (e.g. private YANG modules using an inline "module:" prefix), so
	// normalize the assembled path to extract it.
	path.normalize()

	return path
}

func (pi *pathInfo) normalize() {
	if len(pi.segments) == 0 {
		return
	}

	// Some devices supply the origin as part of the first path element,
	// so try to find and extract it there.
	groups := originPattern.FindStringSubmatch(pi.segments[0])
	if len(groups) == 2 {
		firstNode := pi.segments[0]
		pi.inlineModule = groups[1]
		// Match normalizeGNMIPath: strip the inline module prefix even when
		// an origin is already present, but never overwrite an explicit origin
		// inherited from the subscription, notification prefix or update path.
		if pi.origin == "" {
			pi.origin = groups[1]
		}
		pi.segments[0] = pi.segments[0][len(groups[1])+1:]
		// elements matching must see the same normalized first node name.
		if len(pi.keyValues) > 0 && pi.keyValues[0].index == 0 && pi.keyValues[0].name == firstNode {
			pi.keyValues[0].name = pi.segments[0]
		}
	}
}

// normalizeGNMIPath extracts an inline "origin:" prefix (e.g.
// "rg-interfaces:interfaces-state") from the first path element of a gNMI
// Path. Device updates are normalized the same way when turned into a
// pathInfo, so subscription paths and update paths must agree.
//
// Behavior:
//   - when Origin is empty, the extracted module becomes the path origin
//     (e.g. "/rg-interfaces:interfaces-state" -> origin "rg-interfaces");
//   - when an explicit Origin is already set, the inline prefix is still
//     stripped from the element so its name aligns with device updates, but
//     the explicit origin wins and is left untouched. (Configuring both an
//     explicit origin and an inline module is unusual, but without this the
//     tag/value leaf names could never equal-match.)
func normalizeGNMIPath(p *gnmiLib.Path) {
	if p == nil || len(p.Elem) == 0 {
		return
	}
	groups := originPattern.FindStringSubmatch(p.Elem[0].Name)
	if len(groups) != 2 {
		return
	}
	if p.Origin == "" {
		p.Origin = groups[1]
	}
	p.Elem[0].Name = p.Elem[0].Name[len(groups[1])+1:]
}

func (pi *pathInfo) isSubPathOf(path *pathInfo) bool {
	ok, _ := pi.subPathMatch(path)
	return ok
}

// sourceMatchRank compares source metadata independently of the path segments.
// Explicit origins must agree when both are supplied. Otherwise a known module
// can establish compatibility between standalone and inline origin forms.
func (pi *pathInfo) sourceMatchRank(actual *pathInfo) int {
	if pi.inlineModule != "" && actual.inlineModule != "" && pi.inlineModule != actual.inlineModule {
		return 0
	}
	if pi.explicitOrigin && actual.explicitOrigin {
		if pi.origin == actual.origin {
			return 3
		}
		return 0
	}
	if pi.inlineModule != "" && pi.inlineModule == actual.inlineModule {
		return 2
	}
	if pi.origin != "" {
		if pi.origin == actual.origin {
			return 2
		}
		return 0
	}
	return 1
}

// subPathMatch reports whether pi (the subscription/alias path) is a sub-path
// of path (the device field path), and whether the match is "strong".
//
// Source compatibility follows the same rules as tag subscriptions. Value paths
// without any source metadata retain the legacy origin-agnostic matching.
//
// A weak match must never override a strong one, otherwise an origin-less
// catch-all subscription would hijack disable_concatenation/conversion/alias of
// an explicit-origin subscription (double-concatenated metric names).
func (pi *pathInfo) subPathMatch(path *pathInfo) (match, strong bool) {
	if pi.sourceMatchRank(path) == 0 && (path.origin != "" || path.inlineModule != "") {
		return false, false
	}

	// The "parent" path should have the same length or be shorter than the
	// sub-path to have a chance to match.
	if len(pi.segments) > len(path.segments) {
		return false, false
	}

	// Compare the elements and exit if we find a mismatch.
	for i, p := range pi.segments {
		if p != path.segments[i] {
			return false, false
		}
	}

	// A subscription without an origin only weakly matches an origin-bearing
	// field; every other containment match is strong.
	strong = pi.origin != "" || path.origin == ""
	return true, strong
}

func (pi *pathInfo) keepCommonPart(path *pathInfo) {
	shortestLen := len(pi.segments)
	if len(path.segments) < shortestLen {
		shortestLen = len(path.segments)
	}

	// Compare the elements and stop as soon as they do mismatch
	var matchLen int
	for i, p := range pi.segments[:shortestLen] {
		if p != path.segments[i] {
			break
		}
		matchLen = i + 1
	}
	if matchLen < 1 {
		pi.segments = nil
		return
	}
	pi.segments = pi.segments[:matchLen]
}

func (pi *pathInfo) split() (dir, base string) {
	if len(pi.segments) == 0 {
		return "", ""
	}
	if len(pi.segments) == 1 {
		return "", pi.segments[0]
	}

	dir = "/" + strings.Join(pi.segments[:len(pi.segments)-1], "/")
	if pi.origin != "" {
		dir = pi.origin + ":" + dir
	}
	return dir, pi.segments[len(pi.segments)-1]
}

func (pi *pathInfo) String() string {
	if len(pi.segments) == 0 {
		return ""
	}

	out := "/" + strings.Join(pi.segments, "/")
	if pi.origin != "" {
		out = pi.origin + ":" + out
	}
	return out
}

func (pi *pathInfo) Tags() map[string]string {
	tags := make(map[string]string, len(pi.keyValues))
	for _, s := range pi.keyValues {
		for k, v := range s.kv {
			key := strings.ReplaceAll(k, "-", "_")

			// Use short-form of key if possible
			if _, exists := tags[key]; !exists {
				tags[key] = v
				continue
			}
			tags[s.path+"/"+key] = v
		}
	}

	return tags
}

// keyTag locates the original automatic label for a specific list key,
// including the full-path spelling used when an ancestor has the same key.
func (pi *pathInfo) keyTag(index int, key string) (string, string, bool) {
	short := strings.ReplaceAll(key, "-", "_")
	seen := false
	for _, segment := range pi.keyValues {
		if segment.index == index {
			value, ok := segment.kv[key]
			if !ok {
				return "", "", false
			}
			// Ambiguous normalized spellings must not cause data loss.
			for other := range segment.kv {
				if other != key && strings.ReplaceAll(other, "-", "_") == short {
					return "", "", false
				}
			}
			if seen {
				return segment.path + "/" + short, value, true
			}
			return short, value, true
		}
		for other := range segment.kv {
			if strings.ReplaceAll(other, "-", "_") == short {
				seen = true
			}
		}
	}
	return "", "", false
}
