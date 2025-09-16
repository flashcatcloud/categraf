package collector

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

func (v *VRRPData) setState(state string) error {
	var ok bool
	if v.State, ok = vrrpDataStringToIntState(state); !ok {
		slog.Error("Unknown state found",
			"state", state,
			"iname", v.IName,
		)

		return fmt.Errorf("unknown state found: %s, iname: %s", state, v.IName)
	}

	return nil
}

func (v *VRRPData) setWantState(wantState string) error {
	var ok bool
	if v.WantState, ok = vrrpDataStringToIntState(wantState); !ok {
		slog.Error("Unknown wantstate found",
			"wantstate", wantState,
			"iname", v.IName,
		)

		return fmt.Errorf("unknown wantstate found: %s", wantState)
	}

	return nil
}

func (v *VRRPData) setGArpDelay(delay string) error {
	var err error
	if v.GArpDelay, err = strconv.Atoi(delay); err != nil {
		slog.Error("Failed to parse GArpDelay to int delay",
			"delay", delay,
			"iname", v.IName,
		)

		return err
	}

	return nil
}

func (v *VRRPData) setVRID(vrid string) error {
	var err error
	if v.VRID, err = strconv.Atoi(vrid); err != nil {
		slog.Error("Failed to parse vrid to int",
			"vrid", vrid,
			"iname", v.IName,
		)

		return err
	}

	return nil
}

func (v *VRRPData) addVIP(vip string) {
	vip = strings.TrimSpace(vip)
	v.VIPs = append(v.VIPs, vip)
}

func (v *VRRPData) addExcludedVIP(vip string) {
	vip = strings.TrimSpace(vip)
	v.ExcludedVIPs = append(v.ExcludedVIPs, vip)
}
