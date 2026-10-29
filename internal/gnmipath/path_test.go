package gnmipath

import (
	"testing"
)

func TestGNMIPathCompatibility(t *testing.T) {
	for _, tc := range []struct {
		path             string
		name, key, value string
		count            int
	}{
		{"", "", "", "", 0},
		{"/", "", "", "", 0},
		{"/interfaces/interface[name=Ethernet1/2/3]/state/counters", "interface", "name", "Ethernet1/2/3", 4},
		{`/a/b[k=escaped\]value]/c`, "b", "k", "escaped]value", 3},
		{`/a/b[k=escaped\[value]/c`, "b", "k", "escaped[value", 3},
		{"/a/b[k=first][other=second]/c", "b", "other", "second", 3},
		{"/a/openconfig:b[k=value]/c", "openconfig:b", "k", "value", 3},
	} {
		t.Run(tc.path, func(t *testing.T) {
			path, err := ToGNMIPath(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(path.Elem) != tc.count {
				t.Fatalf("elements = %v", path.Elem)
			}
			if tc.count > 0 && (path.Elem[1].Name != tc.name || path.Elem[1].Key[tc.key] != tc.value) {
				t.Fatalf("parsed element changed: %v", path.Elem[1])
			}
		})
	}
	for _, path := range []string{"/a/b[k=", "/a/b[k=]/c", "/a/invalid name", "/a/b[k=v]junk", "/a/[k=v]/[other=v]"} {
		if _, err := ToGNMIPath(path); err == nil {
			t.Errorf("expected malformed path error: %q", path)
		}
	}
}
