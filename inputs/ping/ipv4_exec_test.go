//go:build !windows

package ping

import (
	"slices"
	"testing"
)

func TestExecFamilyFlags(t *testing.T) {
	for _, system := range []string{"linux", "darwin", "freebsd", "netbsd", "openbsd"} {
		ins := &Instance{ipv4Only: true, Count: 1}
		args := ins.args("example.com", system)
		if slices.Contains(args, "-4") != (system == "linux") {
			t.Fatalf("%s args=%v", system, args)
		}
	}
	ins := &Instance{IPv6: true, Count: 1}
	if args := ins.args("example.com", "linux"); slices.Contains(args, "-4") {
		t.Fatalf("explicit IPv6 args=%v", args)
	}
}
