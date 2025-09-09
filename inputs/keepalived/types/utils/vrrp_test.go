package utils

import (
	"testing"

	"github.com/hashicorp/go-version"
)

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

			if HasSigNumSupport(tc.version) != tc.expectedSupport {
				t.Fail()
			}
		})
	}
}
