package gnmi

import "testing"

func TestSubscriptionPathAfterDependencyCleanup(t *testing.T) {
	ins := &Instance{Prefix: "/interfaces", Origin: "openconfig", Target: "router"}
	sub := &Subscription{Path: "/interface[name=Ethernet1/2]/state/counters"}
	if err := sub.buildFullPath(ins); err != nil {
		t.Fatal(err)
	}
	path := sub.fullPath
	if path.Origin != "openconfig" || path.Target != "router" || len(path.Elem) != 4 || path.Elem[1].Key["name"] != "Ethernet1/2" {
		t.Fatalf("subscription path changed: %v", path)
	}
	parsed, err := parsePath("vendor", sub.Path, "target")
	if err != nil || parsed.Origin != "vendor" || parsed.Target != "target" {
		t.Fatalf("parsePath: %v, %v", parsed, err)
	}
}
