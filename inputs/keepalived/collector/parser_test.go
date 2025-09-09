package collector

import (
	"os"
	"reflect"
	"testing"
)

func TestGetIntStatus(t *testing.T) {
	t.Parallel()

	acceptableStatuses := []string{"BAD", "GOOD"}
	script := VRRPScript{}

	for expected, status := range acceptableStatuses {
		script.Status = status
		if result, ok := script.getIntStatus(); !ok || result != expected {
			t.Fail()
		}
	}

	script.Status = "NOTGOOD"
	if result, ok := script.getIntStatus(); ok || result != -1 {
		t.Fail()
	}
}

func TestGetIntState(t *testing.T) {
	t.Parallel()

	acceptableStates := []string{"idle", "running", "requested termination", "forcing termination"}
	script := VRRPScript{}

	for expected, state := range acceptableStates {
		script.State = state
		if result, ok := script.getIntState(); !ok || result != expected {
			t.Fail()
		}
	}

	script.State = "NOTGOOD"
	if result, ok := script.getIntState(); ok || result != -1 {
		t.Fail()
	}
}

func TestVRRPDataStringToIntState(t *testing.T) {
	t.Parallel()

	acceptableStates := []string{"INIT", "BACKUP", "MASTER", "FAULT"}

	for expected, state := range acceptableStates {
		result, ok := vrrpDataStringToIntState(state)
		if !ok || result != expected {
			t.Fail()
		}
	}

	result, ok := vrrpDataStringToIntState("NOGOOD")
	if ok || result != -1 {
		t.Fail()
	}
}

func TestV215ParseVRRPData(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.1.5/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpData, err := ParseVRRPData(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(vrrpData) != 3 {
		t.Fail()
	}

	viExt1 := VRRPData{
		IName:     "VI_EXT_1",
		State:     2,
		WantState: 2,
		Intf:      "ens192",
		GArpDelay: 5,
		VRID:      10,
		VIPs:      []string{"192.168.2.1 dev ens192 scope global set"},
	}
	viExt2 := VRRPData{
		IName:     "VI_EXT_2",
		State:     1,
		WantState: 1,
		Intf:      "ens192",
		GArpDelay: 5,
		VRID:      20,
		VIPs:      []string{"192.168.2.2 dev ens192 scope global"},
	}
	viExt3 := VRRPData{
		IName:     "VI_EXT_3",
		State:     1,
		WantState: 1,
		Intf:      "ens192",
		GArpDelay: 5,
		VRID:      30,
		VIPs:      []string{"192.168.2.3 dev ens192 scope global"},
	}

	for _, data := range vrrpData {
		switch data.IName {
		case "VI_EXT_1":
			if !reflect.DeepEqual(*data, viExt1) {
				t.Fail()
			}
		case "VI_EXT_2":
			if !reflect.DeepEqual(*data, viExt2) {
				t.Fail()
			}
		case "VI_EXT_3":
			if !reflect.DeepEqual(*data, viExt3) {
				t.Fail()
			}
		}
	}
}

func TestV2010ParseVRRPData(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.0.10/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpData, err := ParseVRRPData(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(vrrpData) != 1 {
		t.Fail()
	}

	vi1 := VRRPData{
		IName:     "VI_1",
		State:     2,
		WantState: 2,
		Intf:      "ens192",
		GArpDelay: 5,
		VRID:      52,
		VIPs:      []string{"2.2.2.2/32 dev ens192 scope global"},
	}

	for _, data := range vrrpData {
		if data.IName == "VI_1" {
			if !reflect.DeepEqual(*data, vi1) {
				t.Fail()
			}
		} else {
			t.Fail()
		}
	}
}

func TestV215ParseVRRPScript(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.0.10/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpScripts := ParseVRRPScript(f)

	if len(vrrpScripts) != 1 {
		t.Fail()
	}

	for _, script := range vrrpScripts {
		if script.Name != "chk_service" {
			t.Fail()
		}

		if script.Status != "GOOD" {
			t.Fail()
		}

		if script.State != "idle" {
			t.Fail()
		}
	}
}

func TestV2010ParseVRRPScript(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.1.5/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpScripts := ParseVRRPScript(f)

	if len(vrrpScripts) != 1 {
		t.Fail()
	}

	for _, script := range vrrpScripts {
		if script.Name != "check_script" {
			t.Fail()
		}

		if script.Status != "GOOD" {
			t.Fail()
		}

		if script.State != "idle" {
			t.Fail()
		}
	}
}

func TestV215ParseStats(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.1.5/keepalived.stats")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	stats, err := ParseStats(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(stats) != 3 {
		t.Fail()
	}

	// check for VI_EXT_1
	viExt1 := VRRPStats{
		AdvertRcvd:        11,
		AdvertSent:        12,
		BecomeMaster:      2,
		ReleaseMaster:     1,
		PacketLenErr:      1,
		IPTTLErr:          1,
		InvalidTypeRcvd:   1,
		AdvertIntervalErr: 1,
		AddrListErr:       1,
		InvalidAuthType:   2,
		AuthTypeMismatch:  2,
		AuthFailure:       2,
		PRIZeroRcvd:       1,
		PRIZeroSent:       1,
	}
	if !reflect.DeepEqual(viExt1, *stats["VI_EXT_1"]) {
		t.Fail()
	}

	// check for VI_EXT_2
	viExt2 := VRRPStats{
		AdvertRcvd:        10,
		AdvertSent:        158,
		BecomeMaster:      2,
		ReleaseMaster:     2,
		PacketLenErr:      10,
		IPTTLErr:          10,
		InvalidTypeRcvd:   10,
		AdvertIntervalErr: 10,
		AddrListErr:       10,
		InvalidAuthType:   20,
		AuthTypeMismatch:  20,
		AuthFailure:       20,
		PRIZeroRcvd:       12,
		PRIZeroSent:       12,
	}
	if !reflect.DeepEqual(viExt2, *stats["VI_EXT_2"]) {
		t.Fail()
	}

	// check for VI_EXT_3
	viExt3 := VRRPStats{
		AdvertRcvd:        23,
		AdvertSent:        172,
		BecomeMaster:      4,
		ReleaseMaster:     4,
		PacketLenErr:      30,
		IPTTLErr:          30,
		InvalidTypeRcvd:   30,
		AdvertIntervalErr: 30,
		AddrListErr:       30,
		InvalidAuthType:   10,
		AuthTypeMismatch:  10,
		AuthFailure:       2,
		PRIZeroRcvd:       1,
		PRIZeroSent:       2,
	}
	if !reflect.DeepEqual(viExt3, *stats["VI_EXT_3"]) {
		t.Fail()
	}
}

func TestV135ParseVRRPData(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v1.3.5/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpData, err := ParseVRRPData(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(vrrpData) != 1 {
		t.Fail()
	}

	vi1 := VRRPData{
		IName:     "VI_1",
		State:     2,
		WantState: 0,
		Intf:      "eth0",
		GArpDelay: 5,
		VRID:      51,
		VIPs:      []string{"10.32.75.200/32 dev eth0 scope global"},
	}

	for _, data := range vrrpData {
		if data.IName == "VI_1" {
			if !reflect.DeepEqual(*data, vi1) {
				t.Fail()
			}
		} else {
			t.Fail()
		}
	}
}

func TestV135ParseVRRPScript(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v1.3.5/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpScripts := ParseVRRPScript(f)

	if len(vrrpScripts) != 1 {
		t.Fail()
	}

	for _, script := range vrrpScripts {
		if script.Name != "check_haproxy" {
			t.Fail()
		}

		if script.Status != "BAD" {
			t.Fail()
		}

		if script.State != "" {
			t.Fail()
		}
	}
}

func TestV135ParseStats(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v1.3.5/keepalived.stats")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	stats, err := ParseStats(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(stats) != 1 {
		t.Fail()
	}

	vi1 := VRRPStats{
		AdvertRcvd:        1,
		AdvertSent:        17,
		BecomeMaster:      1,
		ReleaseMaster:     1,
		PacketLenErr:      10,
		IPTTLErr:          1,
		InvalidTypeRcvd:   5,
		AdvertIntervalErr: 4,
		AddrListErr:       3,
		InvalidAuthType:   3,
		AuthTypeMismatch:  6,
		AuthFailure:       7,
		PRIZeroRcvd:       9,
		PRIZeroSent:       2,
	}
	if !reflect.DeepEqual(vi1, *stats["VI_1"]) {
		t.Fail()
	}
}

func TestParseVIP(t *testing.T) {
	t.Parallel()

	vips := []string{"192.168.2.2 dev ens192 scope global", "192.168.2.2 dev ens192 scope global set"}
	excpectedIP := "192.168.2.2"
	excpectedIntf := "ens192"

	for _, vip := range vips {
		ip, intf, ok := ParseVIP(vip)
		if !ok {
			t.Error("Error parsing")
			t.Fail()
		}

		if ip != excpectedIP || intf != excpectedIntf {
			t.Error("ip or interface not equals")
			t.Fail()
		}
	}

	badVIP := "192.168.2.2 dev"
	if ip, intf, ok := ParseVIP(badVIP); ok || ip != "" || intf != "" {
		t.Fail()
	}
}

func TestIsKeyArray(t *testing.T) {
	t.Parallel()

	supportedKeys := []string{"Virtual IP"}

	for _, key := range supportedKeys {
		if !isKeyArray(key) {
			t.Fail()
		}
	}

	if notArrayKey := "NoArray"; isKeyArray(notArrayKey) {
		t.Fail()
	}
}

func TestV227ParseVRRPData(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../test_files/v2.2.7/keepalived.data")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	defer f.Close() //nolint: errcheck

	vrrpData, err := ParseVRRPData(f)
	if err != nil {
		t.Log(err)
		t.Fail()
	}

	if len(vrrpData) != 1 {
		t.Fail()
	}

	viExt1 := VRRPData{
		IName:        "VI_227_1",
		State:        2,
		WantState:    2,
		Intf:         "ens3",
		GArpDelay:    5,
		VRID:         52,
		VIPs:         []string{"10.1.0.1/24 dev ens3 scope global set"},
		ExcludedVIPs: []string{"10.10.0.1 dev ens3 scope global set"},
	}

	for _, data := range vrrpData {
		if data.IName == "VI_227_1" {
			if !reflect.DeepEqual(*data, viExt1) {
				t.Fail()
			}
		}
	}
}
