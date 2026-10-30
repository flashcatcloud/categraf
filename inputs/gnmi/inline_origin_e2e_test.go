package gnmi

import (
	"strings"
	"testing"

	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"

	"flashcat.cloud/categraf/types"
)

// TestInlineOriginTagSubscriptionAndConcatenation drives the full update
// handler with private YANG paths that carry an inline module prefix
// (rg-interfaces:..., rg-system:...), mirroring a Ruijie device that does not
// set Path.Origin. It verifies:
//   - tag subscriptions (hostname=unconditional, ifname=name) are recognized,
//     so they enrich the interface metrics and are NOT emitted as metrics;
//   - subscriptions with disable_concatenation keep their fixed name instead
//     of producing e.g. gnmi_interface_admin_status_admin_status;
//   - normal subscriptions still concatenate the leaf name.
func TestInlineOriginTagSubscriptionAndConcatenation(t *testing.T) {
	// Value subscriptions, configured exactly like the reported config.
	subs := []Subscription{
		{Name: "gnmi_interface", Path: "/rg-interfaces:interfaces-state/interface/statistics", DisableConcatenation: false},
		{Name: "gnmi_interface_admin_status", Path: "/rg-interfaces:interfaces-state/interface/admin-status", DisableConcatenation: true},
		{Name: "gnmi_interface_oper_status", Path: "/rg-interfaces:interfaces-state/interface/oper-status", DisableConcatenation: true},
		{Name: "gnmi_interface_speed", Path: "/rg-interfaces:interfaces-state/interface/speed", DisableConcatenation: true},
	}
	tagsubs := []TagSubscription{
		{Subscription: Subscription{Name: "hostname", Path: "/rg-system:system/hostname", DisableConcatenation: true}, Match: "unconditional"},
		{Subscription: Subscription{Name: "ifname", Path: "/rg-interfaces:interfaces-state/interface/name", DisableConcatenation: true}, Match: "name"},
	}
	for i := range subs {
		if err := subs[i].buildFullPath(&Instance{}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range tagsubs {
		if err := tagsubs[i].buildFullPath(&Instance{}); err != nil {
			t.Fatal(err)
		}
	}

	aliases := map[*pathInfo]string{}
	for i := range subs {
		if err := subs[i].buildAlias(aliases); err != nil {
			t.Fatal(err)
		}
	}

	h := &handler{
		address:   "10.8.177.98:50051",
		aliases:   aliases,
		subs:      subs,
		tagsubs:   tagsubs,
		tagStore:  newTagStore(tagsubs),
		sourceTag: "ident",
	}

	updates := []*gnmiLib.Update{
		// Hostname (rg-system, unconditional tag)
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-system:system"}, {Name: "hostname"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "spine98"}},
		},
		// Interface name (rg-interfaces, name-matched tag)
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-interfaces:interfaces-state"},
				{Name: "interface", Key: map[string]string{"name": "FH0/14"}},
				{Name: "name"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "FH0/14"}},
		},
		// Fixed-name scalar metrics
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-interfaces:interfaces-state"},
				{Name: "interface", Key: map[string]string{"name": "FH0/14"}},
				{Name: "admin-status"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 1}},
		},
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-interfaces:interfaces-state"},
				{Name: "interface", Key: map[string]string{"name": "FH0/14"}},
				{Name: "oper-status"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 1}},
		},
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-interfaces:interfaces-state"},
				{Name: "interface", Key: map[string]string{"name": "FH0/14"}},
				{Name: "speed"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 400000000}},
		},
		// A counters leaf: subscription has concatenation enabled, so the leaf
		// name must be appended to gnmi_interface.
		{
			Path: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{
				{Name: "rg-interfaces:interfaces-state"},
				{Name: "interface", Key: map[string]string{"name": "FH0/14"}},
				{Name: "statistics"}, {Name: "in-octets"},
			}},
			Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 123}},
		},
	}

	resp := &gnmiLib.SubscribeResponse_Update{
		Update: &gnmiLib.Notification{Update: updates},
	}
	slist := types.NewSampleList()
	h.handleSubscribeResponseUpdate(slist, resp, nil)

	got := map[string]*types.Sample{}
	for _, s := range slist.PopBackAll() {
		got[s.Metric] = s
	}

	// Tag subscriptions must NOT be emitted as their own metrics (under any
	// concatenated form such as gnmi_hostname_hostname).
	for _, banned := range []string{"gnmi_hostname", "gnmi_ifname"} {
		for name := range got {
			if strings.HasPrefix(name, banned) {
				t.Fatalf("tag subscription %q emitted as metric %q: %v", banned, name, metricNames(got))
			}
		}
	}

	// Fixed-name metrics must not have the leaf appended.
	for _, m := range []string{
		"gnmi_interface_admin_status",
		"gnmi_interface_oper_status",
		"gnmi_interface_speed",
	} {
		s, ok := got[m]
		if !ok {
			t.Fatalf("missing metric %q, got %v", m, metricNames(got))
		}
		if s.Labels["ifname"] != "FH0/14" {
			t.Fatalf("%s: ifname label = %q, want FH0/14 (labels=%v)", m, s.Labels["ifname"], s.Labels)
		}
		if s.Labels["hostname"] != "spine98" {
			t.Fatalf("%s: hostname label = %q, want spine98 (labels=%v)", m, s.Labels["hostname"], s.Labels)
		}
	}
	// Bad doubled names must not exist.
	for _, bad := range []string{
		"gnmi_interface_admin_status_admin_status",
		"gnmi_interface_oper_status_oper_status",
		"gnmi_interface_speed_speed",
	} {
		if _, ok := got[bad]; ok {
			t.Fatalf("unexpected concatenated metric %q", bad)
		}
	}

	// Concatenating subscription keeps the leaf suffix.
	if _, ok := got["gnmi_interface_in_octets"]; !ok {
		t.Fatalf("expected concatenated metric gnmi_interface_in_octets, got %v", metricNames(got))
	}
}

func metricNames(m map[string]*types.Sample) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}
