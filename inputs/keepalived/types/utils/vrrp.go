package utils

import "github.com/hashicorp/go-version"

var vrrpScriptStateSupportedVersion = version.Must(version.NewVersion("1.4.0"))

// HasVRRPScriptStateSupport check if Keepalived version supports VRRP Script State in output.
func HasVRRPScriptStateSupport(v *version.Version) bool {
	return v == nil || v.GreaterThanOrEqual(vrrpScriptStateSupportedVersion)
}
