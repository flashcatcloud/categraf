package gnmi

import (
	"regexp"
	"testing"

	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/prometheus/common/model"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

func TestTagLabelPipelineCompatibility(t *testing.T) {
	previous, previousHostInfo := config.Config, config.HostInfo
	config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
	config.HostInfo = &config.HostInfoCache{}
	t.Cleanup(func() { config.Config = previous; config.HostInfo = previousHostInfo })
	cases := []struct {
		name, path, rawName, exportedName string
		concatenate                       bool
	}{
		{"if-name", "/system/hostname", "if-name", "if_name", false},
		{"if.name", "/system/hostname", "if.name", "if_name", false},
		{"if/name", "/system/hostname", "if/name", "if_name", false},
		{"descr", "/system/host-name", "descr/host_name", "descr_host_name", true},
	}
	for _, tc := range cases {
		for _, action := range []string{"override", "delete", "relabel"} {
			t.Run(tc.rawName+"/"+action, func(t *testing.T) {
				tag := TagSubscription{Subscription: Subscription{Name: tc.name, Path: tc.path, DisableConcatenation: !tc.concatenate}, Match: "unconditional"}
				h := newOriginTestHandler(t, []Subscription{{Name: "gnmi_value", Path: "/system/value", DisableConcatenation: true}}, []TagSubscription{tag})
				list := types.NewSampleList()
				h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{Update: []*gnmiLib.Update{
					{Path: originTestPath(t, "", tag.Path), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: "edge"}}},
					{Path: originTestPath(t, "", "/system/value"), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_UintVal{UintVal: 1}}},
				}}}, nil)
				pipeline := config.InternalConfig{}
				want := map[string]string{"__name__": "gnmi_value", "source": "127.0.0.1"}
				switch action {
				case "override":
					pipeline.Labels = map[string]string{tc.rawName: "override"}
					want[tc.exportedName] = "override"
				case "delete":
					pipeline.Labels = map[string]string{tc.rawName: "-"}
				case "relabel":
					pipeline.RelabelConfigs = []*config.RelabelConfig{
						{SourceLabels: model.LabelNames{model.LabelName(tc.rawName)}, TargetLabel: "interface"},
						{Action: "labeldrop", Regex: regexp.QuoteMeta(tc.rawName)},
					}
					want["interface"] = "edge"
				}
				if err := pipeline.InitInternalConfig(); err != nil {
					t.Fatal(err)
				}
				samples := pipeline.Process(list).PopBackAll()
				if len(samples) != 1 {
					t.Fatalf("samples=%d", len(samples))
				}
				series := samples[0].ConvertTimeSeries("")
				if series == nil || len(series.Labels) != len(want) {
					t.Fatalf("unexpected exported labels: %v, want %v", series, want)
				}
				for _, label := range series.Labels {
					value, exists := want[label.Name]
					if !exists || value != label.Value {
						t.Fatalf("unexpected or duplicate label %s=%q", label.Name, label.Value)
					}
					delete(want, label.Name)
				}
			})
		}
	}
}

func TestTagCanonicalNameConflicts(t *testing.T) {
	for _, match := range []string{"unconditional", "name", "elements"} {
		t.Run(match, func(t *testing.T) {
			subs := []TagSubscription{
				{Subscription: Subscription{Name: "if-name", Path: "/interfaces/interface[name=Ethernet1]/state/first", DisableConcatenation: true}, Match: match, Elements: []string{"interface"}},
				{Subscription: Subscription{Name: "if_name", Path: "/interfaces/interface[name=Ethernet1]/state/second", DisableConcatenation: true}, Match: match, Elements: []string{"interface"}},
			}
			h := newOriginTestHandler(t, nil, subs)
			metric := newInfoFromPath(originTestPath(t, "", "/interfaces/interface[name=Ethernet1]/state/counters"))
			update := func(index int, value string) {
				t.Helper()
				list := types.NewSampleList()
				h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{Update: []*gnmiLib.Update{
					{Path: originTestPath(t, "", subs[index].Path), Val: &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_StringVal{StringVal: value}}},
				}}}, nil)
				if samples := list.PopBackAll(); len(samples) != 0 {
					t.Fatalf("tag emitted as metric: %v", samples)
				}
			}
			lookup := func() map[string]string { return h.tagStore.lookup(metric, metric.Tags()) }
			update(0, "edge")
			if got := lookup(); len(got) != 1 || got["if-name"] != "edge" {
				t.Fatalf("raw name lost: %v", got)
			}
			update(1, "core")
			if got := lookup(); len(got) != 0 {
				t.Fatalf("canonical conflict not omitted: %v", got)
			}
			update(1, "")
			if got := lookup(); len(got) != 1 || got["if-name"] != "edge" {
				t.Fatalf("clear affected another source: %v", got)
			}
			update(1, "edge")
			for i := 0; i < 10; i++ {
				if got := lookup(); len(got) != 1 || got["if-name"] != "edge" {
					t.Fatalf("same-value aliases did not merge deterministically: %v", got)
				}
			}
		})
	}
}
