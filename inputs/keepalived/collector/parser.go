package collector

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"slices"
)

var (
	// VRRPScriptStatuses contains VRRP Script statuses.
	VRRPScriptStatuses = []string{"BAD", "GOOD"}
	// VRRPScriptStates contains VRRP Script states.
	VRRPScriptStates = []string{"idle", "running", "requested termination", "forcing termination"}
	// VRRPStates contains VRRP states.
	VRRPStates = []string{"INIT", "BACKUP", "MASTER", "FAULT"}
)

func (v *VRRPScript) getIntStatus() (int, bool) {
	for i, s := range VRRPScriptStatuses {
		if s == v.Status {
			return i, true
		}
	}

	return -1, false
}

func (v *VRRPScript) getIntState() (int, bool) {
	for i, s := range VRRPScriptStates {
		if s == v.State {
			return i, true
		}
	}

	return -1, false
}

func vrrpDataStringToIntState(state string) (int, bool) {
	for i, s := range VRRPStates {
		if s == state {
			return i, true
		}
	}

	return -1, false
}

func ParseJSON(i io.Reader) ([]VRRP, error) {
	stats := make([]VRRP, 0)

	err := json.NewDecoder(i).Decode(&stats)
	if err != nil {
		return stats, err
	}

	return stats, nil
}

// isKeyArray checks if key is array in keepalived.data file.
func isKeyArray(key string) bool {
	supportedKeys := []string{"Virtual IP"}
	if slices.Contains(supportedKeys, key) {
		return true
	}

	slog.Debug("Unsupported array key",
		"Key", key,
		"SupportedKeys", supportedKeys,
	)

	return false
}

func ParseVRRPData(i io.Reader) (map[string]*VRRPData, error) {
	data := make(map[string]*VRRPData)

	sep := "VRRP Instance"
	prop := "="
	arrayProp := ":"

	var instance, key, val string

	scanner := bufio.NewScanner(bufio.NewReader(i))

	for scanner.Scan() {
		l := scanner.Text()

		switch {
		case strings.HasPrefix(l, " "+sep) && strings.Contains(l, prop):
			s := strings.Split(strings.TrimSpace(l), prop)
			instance = strings.TrimSpace(s[1])
			data[instance] = &VRRPData{IName: instance}
		case strings.HasPrefix(l, "   ") && instance != "":
			if strings.HasPrefix(l, "     ") {
				val = strings.TrimSpace(l)
			} else {
				var args []string

				switch {
				case strings.Contains(l, prop):
					args = strings.Split(strings.TrimSpace(l), prop)
				case strings.Contains(l, arrayProp):
					args = strings.Split(strings.TrimSpace(l), arrayProp)
				default:
					continue
				}

				key = strings.TrimSpace(args[0])
				if isKeyArray(key) {
					continue
				}

				val = strings.TrimSpace(args[1])
			}

			if (strings.HasPrefix(key, "Virtual IP (") || key == "Virtual IP") && val != "" {
				data[instance].addVIP(val)
			}

			if (strings.HasPrefix(key, "Virtual IP Excluded")) && val != "" {
				data[instance].addExcludedVIP(val)
			}

			switch key {
			case "State":
				if err := data[instance].setState(val); err != nil {
					return data, err
				}
			case "Wantstate":
				if err := data[instance].setWantState(val); err != nil {
					return data, err
				}
			case "Interface", "Listening device":
				data[instance].Intf = val
			case "Gratuitous ARP delay":
				if err := data[instance].setGArpDelay(val); err != nil {
					return data, err
				}
			case "Virtual Router ID":
				if err := data[instance].setVRID(val); err != nil {
					return data, err
				}
			}
		case strings.HasPrefix(l, " VRRP Version") || strings.HasPrefix(l, " VRRP Script"):
			// Seen in version <= 1.3.5
			continue
		default:
			instance = ""
		}
	}

	return data, nil
}

func ParseVRRPScript(i io.Reader) []VRRPScript {
	scripts := make([]VRRPScript, 0)

	sep := "VRRP Script"
	prop := "="

	script := VRRPScript{}
	scanner := bufio.NewScanner(bufio.NewReader(i))

	for scanner.Scan() {
		l := scanner.Text()

		switch {
		case strings.HasPrefix(l, " "+sep) && strings.Contains(l, prop):
			if script.Name != "" {
				scripts = append(scripts, script)
				script = VRRPScript{}
			}

			s := strings.Split(strings.TrimSpace(l), prop)
			script.Name = strings.TrimSpace(s[1])
		case strings.HasPrefix(l, "   ") && script.Name != "":
			if !strings.Contains(l, prop) {
				continue
			}

			s := strings.Split(strings.TrimSpace(l), prop)
			key := strings.TrimSpace(s[0])
			val := strings.TrimSpace(s[1])

			switch key {
			case "Status":
				script.Status = val
			case "State":
				script.State = val
			}
		case !strings.HasPrefix(l, "    "):
			if script.Name != "" {
				scripts = append(scripts, script)
				script = VRRPScript{}
			}
		}
	}

	if script.Name != "" {
		scripts = append(scripts, script)
	}

	return scripts
}

func ParseStats(i io.Reader) (map[string]*VRRPStats, error) {
	stats := make(map[string]*VRRPStats)

	sep := "VRRP Instance"
	prop := ":"

	scanner := bufio.NewScanner(bufio.NewReader(i))

	var instance, section string

	for scanner.Scan() {
		l := scanner.Text()

		switch {
		case strings.HasPrefix(l, sep) && strings.Contains(l, prop):
			sp := strings.Split(strings.TrimSpace(l), prop)
			instance = strings.TrimSpace(sp[1])
			stats[instance] = &VRRPStats{}
		case strings.HasPrefix(l, "  ") && strings.HasSuffix(l, prop):
			sp := strings.Split(strings.TrimSpace(l), prop)
			section = strings.TrimSpace(sp[0])
		case strings.HasPrefix(l, "    ") && section != "":
			sp := strings.Split(strings.TrimSpace(l), prop)
			key := strings.TrimSpace(sp[0])
			val := strings.TrimSpace(sp[1])

			value, err := strconv.Atoi(val)
			if err != nil {
				slog.Error("Unknown metric value from keepalived.stats",
					"key", key,
					"val", val,
					"error", err,
				)

				return stats, err
			}

			switch section {
			case "Advertisements":
				switch key {
				case "Received":
					stats[instance].AdvertRcvd = value
				case "Sent":
					stats[instance].AdvertSent = value
				}
			case "Packet Errors":
				switch key {
				case "Length":
					stats[instance].PacketLenErr = value
				case "TTL":
					stats[instance].IPTTLErr = value
				case "Invalid Type":
					stats[instance].InvalidTypeRcvd = value
				case "Advertisement Interval":
					stats[instance].AdvertIntervalErr = value
				case "Address List":
					stats[instance].AddrListErr = value
				}
			case "Authentication Errors":
				switch key {
				case "Invalid Type":
					stats[instance].InvalidAuthType = value
				case "Type Mismatch":
					stats[instance].AuthTypeMismatch = value
				case "Failure":
					stats[instance].AuthFailure = value
				}
			case "Priority Zero":
				switch key {
				case "Received":
					stats[instance].PRIZeroRcvd = value
				case "Sent":
					stats[instance].PRIZeroSent = value
				}
			}
		case strings.HasPrefix(l, "  ") && !strings.HasSuffix(l, prop) && !strings.HasPrefix(l, "    "):
			sp := strings.Split(strings.TrimSpace(l), prop)
			key := strings.TrimSpace(sp[0])
			val := strings.TrimSpace(sp[1])
			section = ""

			value, err := strconv.Atoi(val)
			if err != nil {
				slog.Error("Unknown metric value from keepalived.stats",
					"key", key,
					"val", val,
					"error", err,
				)

				return stats, err
			}

			switch key {
			case "Became master":
				stats[instance].BecomeMaster = value
			case "Released master":
				stats[instance].ReleaseMaster = value
			}
		}
	}

	return stats, nil
}

func ParseVIP(vip string) (string, string, bool) {
	args := strings.Split(vip, " ")
	if len(args) < 3 {
		slog.Error("Failed to parse VIP from keepalived data",
			"VIP", vip,
			"ExpectedArgs", 3,
			"GotArgs", len(args),
		)

		return "", "", false
	}

	return args[0], args[2], true
}
