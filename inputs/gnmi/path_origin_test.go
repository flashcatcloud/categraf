package gnmi

import (
	"testing"

	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
)

// Regression test for inline-origin handling (private YANG modules that put
// the module name in the first path element instead of the dedicated Origin
// field), e.g. /rg-interfaces:interfaces-state/interface/speed.
//
// Mirrors telegraf issue #14833 / PR #14838: pathInfo.append() and
// appendSegments() must normalize the assembled path so the inline origin is
// extracted. Otherwise the measurement alias lookup fails with
// "No measurement alias for gNMI path".
func TestAppendExtractsInlineOrigin(t *testing.T) {
	// An empty subscription prefix, as sent by devices that put everything in
	// the update path.
	prefix := &pathInfo{segments: []string{}}

	updatePath := &gnmiLib.Path{
		Elem: []*gnmiLib.PathElem{
			{Name: "rg-interfaces:interfaces-state"},
			{Name: "interface", Key: map[string]string{"name": "Ethernet1"}},
			{Name: "speed"},
		},
	}

	full := prefix.append(updatePath)

	if full.origin != "rg-interfaces" {
		t.Fatalf("origin not extracted, got %q (segments: %v)", full.origin, full.segments)
	}

	wantSegs := []string{"interfaces-state", "interface", "speed"}
	if len(full.segments) != len(wantSegs) {
		t.Fatalf("segment length mismatch, got %v", full.segments)
	}
	for i, s := range wantSegs {
		if full.segments[i] != s {
			t.Fatalf("segment %d = %q, want %q (full: %v)", i, full.segments[i], s, full.segments)
		}
	}

	if got, want := full.String(), "rg-interfaces:/interfaces-state/interface/speed"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	// A subscription/alias configured with a separated origin must now match.
	alias := &pathInfo{
		origin:   "rg-interfaces",
		segments: []string{"interfaces-state", "interface"},
	}
	if !alias.isSubPathOf(full) {
		t.Fatalf("alias %q is not a sub-path of field path %q", alias.String(), full.String())
	}
}

func TestAppendSegmentsExtractsInlineOrigin(t *testing.T) {
	// JSON / JSON_IETF encoded values are flattened and appended via
	// appendSegments(); ensure an inline prefix there is normalized as well.
	base := &pathInfo{segments: []string{}}
	full := base.appendSegments("rg-interfaces:interfaces-state", "interface", "admin-status")

	if full.origin != "rg-interfaces" {
		t.Fatalf("origin not extracted, got %q (segments: %v)", full.origin, full.segments)
	}
	if got, want := full.String(), "rg-interfaces:/interfaces-state/interface/admin-status"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
