package container

import (
	"testing"

	"github.com/hashicorp/go-version"
)

func TestInitPaths(t *testing.T) {
	t.Parallel()

	k := KeepalivedContainerCollectorHost{}
	k.initPaths("/custom-tmp")

	if k.jsonPath != "/custom-tmp/keepalived.json" {
		t.Fail()
	}

	if k.statsPath != "/custom-tmp/keepalived.stats" {
		t.Fail()
	}

	if k.dataPath != "/custom-tmp/keepalived.data" {
		t.Fail()
	}
}

func TestHasVRRPScriptStateSupport(t *testing.T) {
	t.Parallel()

	testCaseses := []struct {
		name            string
		version         *version.Version
		expectedSupport bool
	}{
		{name: "nil", version: nil, expectedSupport: true},
		{name: "1.4.0", version: version.Must(version.NewVersion("1.4.0")), expectedSupport: true},
		{name: "1.3.5", version: version.Must(version.NewVersion("1.3.5")), expectedSupport: false},
	}

	for _, tc := range testCaseses {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := KeepalivedContainerCollectorHost{
				version: tc.version,
			}

			if c.HasVRRPScriptStateSupport() != tc.expectedSupport {
				t.Fail()
			}
		})
	}
}
