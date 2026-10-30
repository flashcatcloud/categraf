package gnmi

import (
	"testing"

	"flashcat.cloud/categraf/types"
	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
)

func jsonTestValue(data string, ietf bool) *gnmiLib.TypedValue {
	if ietf {
		return &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_JsonIetfVal{JsonIetfVal: []byte(data)}}
	}
	return &gnmiLib.TypedValue{Value: &gnmiLib.TypedValue_JsonVal{JsonVal: []byte(data)}}
}

func TestNestedJSONConversion(t *testing.T) {
	subs := []Subscription{
		{Name: "gnmi_value", Path: "/system", DisableConcatenation: true},
		{Name: "gnmi_value", Path: "/system/state/value", DisableConcatenation: true, Conversion: "ieeefloat32"},
	}
	aliases := make(map[*pathInfo]string)
	for _, sub := range subs {
		if err := sub.buildAlias(aliases); err != nil {
			t.Fatal(err)
		}
	}
	for _, ietf := range []bool{false, true} {
		h := handler{address: "127.0.0.1:50051", aliases: aliases, subs: subs, tagStore: newTagStore(nil)}
		list := types.NewSampleList()
		h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
			Prefix: &gnmiLib.Path{Elem: []*gnmiLib.PathElem{{Name: "system"}}},
			Update: []*gnmiLib.Update{{Path: &gnmiLib.Path{}, Val: jsonTestValue(`{"state":{"value":"P4AAAA=="}}`, ietf)}},
		}}, nil)
		samples := list.PopBackAll()
		if len(samples) != 1 || samples[0].Metric != "gnmi_value" || samples[0].Value != float32(1) {
			t.Fatalf("ietf=%t: nested JSON field lost its conversion: %v", ietf, samples)
		}
		if samples[0].ConvertTimeSeries("") == nil {
			t.Fatal("the converted field was dropped during time-series export")
		}
	}
}

func TestNestedJSONLeafAliases(t *testing.T) {
	h := newOriginTestHandler(t, []Subscription{
		{Name: "gnmi_voltage", Path: "/system/state/voltage", DisableConcatenation: true},
		{Name: "gnmi_current", Path: "/system/state/current", DisableConcatenation: true},
	}, nil)
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: originTestPath(t, "", "/system"),
		Update: []*gnmiLib.Update{{Path: &gnmiLib.Path{}, Val: jsonTestValue(`{"state":{"voltage":2,"current":3}}`, true)}},
	}}, nil)
	values := make(map[string]interface{})
	for _, sample := range list.PopBackAll() {
		values[sample.Metric] = sample.Value
	}
	if len(values) != 2 || values["gnmi_voltage"] != float64(2) || values["gnmi_current"] != float64(3) {
		t.Fatalf("nested leaf aliases or fixed names were lost: %v", values)
	}
}

func TestNestedJSONTagSuffixes(t *testing.T) {
	h := newOriginTestHandler(t, nil, []TagSubscription{{Subscription: Subscription{Name: "info", Path: "/system"}, Match: "unconditional"}})
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: originTestPath(t, "", "/system"),
		Update: []*gnmiLib.Update{{Path: &gnmiLib.Path{}, Val: jsonTestValue(`{"config":{"name":"configured"},"state":{"name":"operational"}}`, true)}},
	}}, nil)
	if samples := list.PopBackAll(); len(samples) != 0 {
		t.Fatalf("JSON tags were emitted as metrics: %v", samples)
	}
	tags := h.tagStore.lookup(newInfoFromString("/metrics/value"), nil)
	if len(tags) != 2 || tags["info_config_name"] != "configured" || tags["info_state_name"] != "operational" {
		t.Fatalf("splitting JSON paths changed or merged tag suffixes: %v", tags)
	}
}

func TestJSONRootInlineOriginAlias(t *testing.T) {
	h := newOriginTestHandler(t, []Subscription{{Name: "gnmi_value", Origin: "rfc7951", Path: "/module:system/state/value", DisableConcatenation: true}}, nil)
	list := types.NewSampleList()
	h.handleSubscribeResponseUpdate(list, &gnmiLib.SubscribeResponse_Update{Update: &gnmiLib.Notification{
		Prefix: &gnmiLib.Path{Origin: "rfc7951"},
		Update: []*gnmiLib.Update{{Path: &gnmiLib.Path{}, Val: jsonTestValue(`{"module:system":{"state":{"value":1}}}`, true)}},
	}}, nil)
	samples := list.PopBackAll()
	if len(samples) != 1 || samples[0].Metric != "gnmi_value" || samples[0].Value != float64(1) {
		t.Fatalf("JSON module or explicit origin was lost: %v", samples)
	}
}
