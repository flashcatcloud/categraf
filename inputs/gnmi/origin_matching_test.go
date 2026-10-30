package gnmi

import (
	"testing"

	"flashcat.cloud/categraf/types"
	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
)

func newOriginTestHandler(t *testing.T, subs []Subscription, tags []TagSubscription) *handler {
	t.Helper()
	aliases := make(map[*pathInfo]string)
	for i := range subs {
		if err := subs[i].buildFullPath(&Instance{}); err != nil {
			t.Fatal(err)
		}
		if err := subs[i].buildAlias(aliases); err != nil {
			t.Fatal(err)
		}
	}
	for i := range tags {
		if err := tags[i].buildFullPath(&Instance{}); err != nil {
			t.Fatal(err)
		}
		if err := tags[i].buildAlias(aliases); err != nil {
			t.Fatal(err)
		}
	}
	return &handler{address: "127.0.0.1:50051", aliases: aliases, subs: subs, tagsubs: tags, tagStore: newTagStore(tags), sourceTag: "source"}
}

func originTestPath(t *testing.T, origin, path string) *gnmiLib.Path {
	t.Helper()
	p, err := parsePath(origin, path, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCanonicalTagOriginWithInlineResponsePrefix(t *testing.T) {
	h := newOriginTestHandler(t, nil, []TagSubscription{{Subscription: Subscription{
		Name: "hostname", Origin: "rg-system", Path: "/system/hostname", DisableConcatenation: true,
	}, Match: "unconditional"}})
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: originTestPath(t, "", "/rg-system:system"),
		Update: []*gnmiLib.Update{{Path: originTestPath(t, "", "/hostname"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "spine98"}}}},
	}}, nil)
	samples := list.PopBackAll()
	tags := h.tagStore.lookup(newInfoFromString("/metrics/value"), nil)
	t.Logf("samples=%d hostname=%q", len(samples), tags["hostname"])
	if len(samples) != 0 || tags["hostname"] != "spine98" {
		t.Fatal("standalone module-origin tag does not recognize the equivalent inline response")
	}
}

func TestRFC7951ValueAliasWithInlineResponsePrefix(t *testing.T) {
	h := newOriginTestHandler(t, []Subscription{{
		Name: "gnmi_interface", Origin: "rfc7951", Path: "/rg-interfaces:interfaces-state/interface/speed",
	}}, nil)
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: originTestPath(t, "", "/rg-interfaces:interfaces-state/interface"),
		Update: []*gnmiLib.Update{{Path: originTestPath(t, "", "/speed"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 100}}}},
	}}, nil)
	samples := list.PopBackAll()
	for _, sample := range samples {
		t.Logf("metric=%s", sample.Metric)
	}
	if len(samples) != 1 || samples[0].Metric != "gnmi_interface_speed" {
		t.Fatal("configured rfc7951 origin prevents value alias matching")
	}
}

func TestDistinctModulesWithSharedExplicitOrigin(t *testing.T) {
	h := newOriginTestHandler(t, []Subscription{
		{Name: "gnmi_module_a", Origin: "rfc7951", Path: "/module-a:system/state/value"},
		{Name: "gnmi_module_b", Origin: "rfc7951", Path: "/module-b:system/state/value", Conversion: "ieeefloat32"},
	}, nil)
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: originTestPath(t, "rfc7951", "/module-a:system/state"),
		Update: []*gnmiLib.Update{{Path: originTestPath(t, "", "/value"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 100}}}},
	}}, nil)
	samples := list.PopBackAll()
	for _, sample := range samples {
		t.Logf("metric=%s", sample.Metric)
	}
	if len(samples) != 1 || samples[0].Metric != "gnmi_module_a_value" {
		t.Fatal("another module's conversion was applied and dropped the valid field")
	}
}

func TestExplicitOriginWithInlineModule(t *testing.T) {
	tagSub := TagSubscription{Subscription: Subscription{Name: "hostname", Origin: "rfc7951", Path: "/rg-system:system/hostname", DisableConcatenation: true}, Match: "unconditional"}
	if err := tagSub.buildFullPath(&Instance{}); err != nil {
		t.Fatal(err)
	}
	aliases := make(map[*pathInfo]string)
	if err := tagSub.buildAlias(aliases); err != nil {
		t.Fatal(err)
	}
	h := handler{address: "127.0.0.1:50051", aliases: aliases, tagsubs: []TagSubscription{tagSub}, tagStore: newTagStore([]TagSubscription{tagSub})}
	hostPath, _ := parsePath("", tagSub.Path, "")
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{Update: []*gnmiLib.Update{
		{Path: hostPath, Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "spine98"}}},
	}}}, nil)
	samples := list.PopBackAll()
	for _, sample := range samples {
		t.Logf("metric=%s labels=%v", sample.Metric, sample.Labels)
	}
	t.Logf("stored hostname=%q", h.tagStore.lookup(newInfoFromString("/metrics/value"), nil)["hostname"])
	if len(samples) != 0 || h.tagStore.lookup(newInfoFromString("/metrics/value"), nil)["hostname"] != "spine98" {
		t.Fatal("explicit origin prevents tag enrichment and emits a tag as a metric")
	}
}

func TestExplicitOriginPrecedesDefaultSubscription(t *testing.T) {
	subs := []Subscription{
		{Name: "gnmi_vendor_speed", Origin: "vendor", Path: "/interfaces/interface/state/speed", DisableConcatenation: true},
		{Name: "gnmi_default_speed", Path: "/interfaces/interface/state/speed", DisableConcatenation: false},
	}
	aliases := make(map[*pathInfo]string)
	for _, sub := range subs {
		if err := sub.buildAlias(aliases); err != nil {
			t.Fatal(err)
		}
	}
	h := handler{address: "127.0.0.1:50051", aliases: aliases, subs: subs, tagStore: newTagStore(nil)}
	updatePath, _ := parsePath("", subs[0].Path, "")
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: &gnmiLib.Path{Origin: "vendor"},
		Update: []*gnmiLib.Update{{Path: updatePath, Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 100}}}},
	}}, nil)
	samples := list.PopBackAll()
	for _, sample := range samples {
		t.Logf("metric=%s", sample.Metric)
	}
	if len(samples) != 1 || samples[0].Metric != "gnmi_vendor_speed" {
		t.Fatal("default-origin subscription overrode explicit-origin subscription")
	}
}

func TestOriginSourceCompatibility(t *testing.T) {
	tests := []struct {
		name, configuredOrigin, configuredPath, actualOrigin, actualPath string
		rank                                                             int
	}{
		{"explicit origin", "vendor", "/system", "vendor", "/system", 3},
		{"canonical to inline", "rg-system", "/system", "", "/rg-system:system", 2},
		{"inline to canonical", "", "/rg-system:system", "rg-system", "/system", 2},
		{"rfc7951 to inline", "rfc7951", "/rg-system:system", "", "/rg-system:system", 2},
		{"inline to rfc7951", "", "/rg-system:system", "rfc7951", "/rg-system:system", 2},
		{"explicit conflict", "rfc7951", "/rg-system:system", "other", "/rg-system:system", 0},
		{"module conflict", "rfc7951", "/module-a:system", "rfc7951", "/module-b:system", 0},
		{"origin conflict", "module-a", "/system", "module-b", "/system", 0},
		{"unverified source", "vendor", "/system", "", "/system", 0},
		{"originless fallback", "", "/system", "vendor", "/system", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured := newInfoFromPath(originTestPath(t, tt.configuredOrigin, tt.configuredPath))
			actual := newInfoFromPath(originTestPath(t, tt.actualOrigin, tt.actualPath))
			if got := tagMatchRank(configured, actual); got != tt.rank {
				t.Fatalf("tag source rank = %d, want %d", got, tt.rank)
			}
			// Value paths retain their existing fallback for completely
			// unqualified device responses, while tags require a known source.
			wantValueMatch := tt.rank > 0 || tt.actualOrigin == "" && actual.inlineModule == ""
			if got, _ := configured.subPathMatch(actual); got != wantValueMatch {
				t.Fatalf("value source match = %t, want %t", got, wantValueMatch)
			}
		})
	}
}

func TestCompatibleOriginAliasSuffix(t *testing.T) {
	for _, module := range []string{"m", "long-module-name"} {
		t.Run(module, func(t *testing.T) {
			h := newOriginTestHandler(t, []Subscription{{Name: "gnmi_system", Origin: "rfc7951", Path: "/" + module + ":system/state"}}, nil)
			list := types.NewSampleList()
			h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
				Prefix: originTestPath(t, "", "/"+module+":system/state"),
				Update: []*gnmiLib.Update{{Path: originTestPath(t, "", "/speed"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 100}}}},
			}}, nil)
			samples := list.PopBackAll()
			if len(samples) != 1 || samples[0].Metric != "gnmi_system_speed" {
				t.Fatalf("alias suffix depends on origin spelling: %v", samples)
			}
		})
	}
}

func TestCompatibleOriginValueOptions(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		subs := []Subscription{
			{Name: "gnmi_value", Origin: "rfc7951", Path: "/module:system/value", DisableConcatenation: true, Conversion: "ieeefloat32"},
			{Name: "gnmi_fallback", Path: "/system/value"},
		}
		if reverse {
			subs[0], subs[1] = subs[1], subs[0]
		}
		h := newOriginTestHandler(t, subs, nil)
		list := types.NewSampleList()
		h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
			Prefix: originTestPath(t, "", "/module:system"),
			Update: []*gnmiLib.Update{{Path: originTestPath(t, "", "/value"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "P4AAAA=="}}}},
		}}, nil)
		samples := list.PopBackAll()
		if len(samples) != 1 || samples[0].Metric != "gnmi_value" || samples[0].Value != float32(1) {
			t.Fatalf("reverse=%t: compatible origin lost its conversion or naming options: %v", reverse, samples)
		}
	}
}
