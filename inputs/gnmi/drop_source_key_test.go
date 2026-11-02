package gnmi

import (
	"reflect"
	"testing"

	"flashcat.cloud/categraf/types"
	"github.com/BurntSushi/toml"
	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
)

func TestDropSourceKeyAfterTagLookup(t *testing.T) {
	const base = "/qos/rdma-counters/interfaces/interface"
	const queue = base + "/input/queues/queue"
	const rawQueueTag = "openconfig-qos:" + queue + "/name"
	for _, match := range []string{"name", "elements"} {
		for _, drop := range []bool{false, true} {
			t.Run(match+"/drop="+map[bool]string{false: "false", true: "true"}[drop], func(t *testing.T) {
				h := newOriginTestHandler(t,
					[]Subscription{{Name: "gnmi_roce_input", Origin: "openconfig-qos", Path: queue + "/state"}},
					[]TagSubscription{
						{Subscription: Subscription{Name: "ifname", Origin: "openconfig-qos", Path: base + "/name", DisableConcatenation: true}, Match: match, Elements: []string{"interface"}, DropSourceKey: drop},
						{Subscription: Subscription{Name: "qename", Origin: "openconfig-qos", Path: queue + "/name", DisableConcatenation: true}, Match: "elements", Elements: []string{"interface", "queue"}, DropSourceKey: drop},
					})
				list := types.NewSampleList()
				h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
					Prefix: originTestPath(t, "openconfig-qos", base+"[name=FH0/67]"),
					Update: []*gnmiLib.Update{
						{Path: originTestPath(t, "", "/name"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "FH0/67"}}},
						{Path: originTestPath(t, "", "/input/queues/queue[name=queue0][id=7]/name"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "queue0"}}},
						{Path: originTestPath(t, "", "/input/queues/queue[name=queue0][id=7]/state/pfc-overspeed"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 0}}},
					},
				}}, nil)
				samples := list.PopBackAll()
				if len(samples) != 1 {
					t.Fatalf("samples = %d, want 1", len(samples))
				}
				labels := samples[0].Labels
				if labels["ifname"] != "FH0/67" || labels["qename"] != "queue0" {
					t.Fatalf("tag association lost: %v", labels)
				}
				if labels["id"] != "7" {
					t.Fatalf("unrelated queue key was removed: %v", labels)
				}
				for _, key := range []string{"name", rawQueueTag} {
					if _, exists := labels[key]; exists == drop {
						t.Fatalf("path tag %q presence = %t, drop = %t: %v", key, exists, drop, labels)
					}
				}
			})
		}
	}
}

func TestDropSourceKeyConfiguration(t *testing.T) {
	var plugin GNMI
	metadata, err := toml.Decode(`
[[instances]]
addresses = ["127.0.0.1:50051"]
[[instances.tag_subscription]]
name = "ifname"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/name"
match = "elements"
elements = ["interface"]
drop_source_key = true
[[instances.tag_subscription]]
name = "legacy"
path = "/system/hostname"
match = "unconditional"
`, &plugin)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Undecoded()) != 0 {
		t.Fatalf("unrecognized configuration: %v", metadata.Undecoded())
	}
	subs := plugin.Instances[0].TagSubscriptions
	if !subs[0].DropSourceKey || subs[1].DropSourceKey {
		t.Fatalf("unexpected enabled/default values: %v", subs)
	}
}

func TestDropSourceKeySafety(t *testing.T) {
	const root = "/qos/rdma-counters/interfaces/interface"
	const queue = root + "/input/queues/queue"
	const keyed = root + "[name=Ethernet1]/input/queues/queue[name=queue0]"
	const rawQueueTag = "openconfig-qos:" + queue + "/name"
	for _, tc := range []struct {
		name, leaf, value, metric, origin string
		conflict, concatenate, drop       bool
		generic                           bool
		unqualifiedActual                 bool
	}{
		{name: "queue only", leaf: "name", value: "queue0", drop: true},
		{name: "concatenated label", leaf: "name", value: "queue0", concatenate: true, drop: true},
		{name: "missing update", leaf: "name"},
		{name: "empty value", leaf: "name", value: ""},
		{name: "different value", leaf: "name", value: "other"},
		{name: "description is not a key", leaf: "description", value: "queue0"},
		{name: "conflicting label", leaf: "name", value: "queue0", conflict: true},
		{name: "different module", leaf: "name", value: "queue0", origin: "another-module"},
		{name: "generic subscription different actual module", leaf: "name", value: "queue0", origin: "another-module", generic: true},
		{name: "unknown actual source", leaf: "name", value: "queue0", origin: "another-module", generic: true, unqualifiedActual: true},
		{name: "output scope", leaf: "name", value: "queue0", metric: root + "[name=Ethernet1]/output/queues/queue[name=queue0]/state/value"},
		{name: "different interface", leaf: "name", value: "queue0", metric: root + "[name=Ethernet2]/input/queues/queue[name=queue0]/state/value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := TagSubscription{Subscription: Subscription{Name: "qename", Origin: "openconfig-qos", Path: queue + "/" + tc.leaf, DisableConcatenation: !tc.concatenate}, Match: "elements", Elements: []string{"interface", "queue"}, DropSourceKey: true}
			if tc.generic {
				sub.Origin = ""
			}
			h := newOriginTestHandler(t, nil, []TagSubscription{sub})
			sub = h.tagsubs[0]
			insert := func(sub TagSubscription, value string) {
				actualOrigin := "openconfig-qos"
				if tc.unqualifiedActual {
					actualOrigin = ""
				}
				p := newInfoFromPath(originTestPath(t, actualOrigin, keyed+"/"+tc.leaf))
				if err := h.tagStore.insert(sub, p, []updateField{{path: p, value: value}}, p.Tags()); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name != "missing update" {
				insert(sub, tc.value)
			}
			if tc.conflict {
				other := sub
				other.Path = queue + "/other"
				if err := other.buildFullPath(&Instance{}); err != nil {
					t.Fatal(err)
				}
				insert(other, "conflict")
			}
			metric := tc.metric
			if metric == "" {
				metric = keyed + "/state/value"
			}
			origin := tc.origin
			if origin == "" {
				origin = "openconfig-qos"
			}
			p := newInfoFromPath(originTestPath(t, origin, metric))
			tags := p.Tags()
			before := p.Tags()
			labels, drops := h.tagStore.lookupWithSourceKeys(p, tags)
			if !reflect.DeepEqual(tags, before) {
				t.Fatal("lookup mutated original path tags")
			}
			if tc.drop {
				if len(drops) != 1 || !drops[rawQueueTag] {
					t.Fatalf("drops = %v, want only queue source key", drops)
				}
				label := "qename"
				if tc.concatenate {
					label += "/name"
				}
				if labels[label] != "queue0" {
					t.Fatalf("replacement label missing: %v", labels)
				}
			} else if len(drops) != 0 {
				t.Fatalf("unsafe removal: %v", drops)
			}
		})
	}
}
