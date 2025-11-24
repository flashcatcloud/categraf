package collector

import (
	"errors"
	"reflect"
	"strconv"
	"testing"
)

func TestSetState(t *testing.T) {
	t.Parallel()

	data := VRRPData{}
	acceptableStates := []string{"INIT", "BACKUP", "MASTER", "FAULT"}

	for expected, state := range acceptableStates {
		err := data.setState(state)
		if err != nil || data.State != expected {
			t.Fail()
		}
	}

	err := data.setState("NOGOOD")
	if err == nil || data.State != -1 {
		t.Fail()
	}
}

func TestSetWantState(t *testing.T) {
	t.Parallel()

	data := VRRPData{}
	acceptableStates := []string{"INIT", "BACKUP", "MASTER", "FAULT"}

	for expected, state := range acceptableStates {
		err := data.setWantState(state)
		if err != nil || data.WantState != expected {
			t.Fail()
		}
	}

	err := data.setWantState("NOGOOD")
	if err == nil || data.WantState != -1 {
		t.Fail()
	}
}

func TestSetGArpDelay(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		delay    string
		expected int
		err      error
	}{
		{delay: "1", expected: 1, err: nil},
		{delay: "1.1", expected: 0, err: strconv.ErrSyntax},
		{delay: "NA", expected: 0, err: strconv.ErrSyntax},
	}

	for _, tc := range testCases {
		t.Run(tc.delay, func(t *testing.T) {
			t.Parallel()

			data := VRRPData{}
			if err := data.setGArpDelay(tc.delay); !errors.Is(err, tc.err) || data.GArpDelay != tc.expected {
				t.Fail()
			}
		})
	}
}

func TestSetVRID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		vrid     string
		expected int
		err      error
	}{
		{vrid: "10", expected: 10, err: nil},
		{vrid: "1.1", expected: 0, err: strconv.ErrSyntax},
		{vrid: "NA", expected: 0, err: strconv.ErrSyntax},
	}

	for _, tc := range testCases {
		t.Run(tc.vrid, func(t *testing.T) {
			t.Parallel()

			data := VRRPData{}
			if err := data.setVRID(tc.vrid); !errors.Is(err, tc.err) || data.VRID != tc.expected {
				t.Fail()
			}
		})
	}
}

func TestAddVIP(t *testing.T) {
	t.Parallel()

	data := VRRPData{}

	vips := []string{"   1.1.1.1", "2.2.2.2", "3.3.3.3   "}
	expectedVIPs := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}

	for _, vip := range vips {
		data.addVIP(vip)
	}

	if !reflect.DeepEqual(expectedVIPs, data.VIPs) {
		t.Fail()
	}
}

func TestAddExcludedVIP(t *testing.T) {
	t.Parallel()

	data := VRRPData{}

	vips := []string{"   1.1.1.1", "2.2.2.2", "3.3.3.3   "}
	expectedVIPs := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}

	for _, vip := range vips {
		data.addExcludedVIP(vip)
	}

	if !reflect.DeepEqual(expectedVIPs, data.ExcludedVIPs) {
		t.Fail()
	}
}
