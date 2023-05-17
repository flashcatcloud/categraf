package smart

import (
	"bufio"
	"context"
	"errors"
	"flashcat.cloud/categraf/pkg/stringx"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

// Instance plugin reads metrics from storage devices supporting S.M.A.R.T.
type Instance struct {
	config.InstanceConfig

	Path             string          `toml:"path" deprecated:"1.16.0;use 'path_smartctl' instead"`
	PathSmartctl     string          `toml:"path_smartctl"`
	PathNVMe         string          `toml:"path_nvme"`
	Nocheck          string          `toml:"nocheck"`
	EnableExtensions []string        `toml:"enable_extensions"`
	Attributes       bool            `toml:"attributes"`
	Excludes         []string        `toml:"excludes"`
	Devices          []string        `toml:"devices"`
	UseSudo          bool            `toml:"use_sudo"`
	Timeout          config.Duration `toml:"timeout"`
	ReadMethod       string          `toml:"read_method"`
}

type nvmeDevice struct {
	name         string `toml:"name"`
	vendorID     string `toml:"vendor_id"`
	model        string `toml:"model"`
	serialNumber string `toml:"serial_number"`
}

// Init performs one time setup of the plugin and returns an error if the configuration is invalid.
func (m *Instance) Init() error {
	if len(m.Devices) == 0 && !m.UseSudo {
		return types.ErrInstancesEmpty
	}
	if m.Timeout == config.Duration(0) {
		m.Timeout = config.Duration(time.Second * 30)
	}
	if m.ReadMethod == "" {
		m.ReadMethod = "concurrent"
	}
	if m.Nocheck == "" {
		m.Nocheck = "standby"
	}
	// if deprecated `path` (to smartctl binary) is provided in config and `path_smartctl` override does not exist
	if len(m.Path) > 0 && len(m.PathSmartctl) == 0 {
		m.PathSmartctl = m.Path
	}

	// if `path_smartctl` is not provided in config, try to find smartctl binary in PATH
	if len(m.PathSmartctl) == 0 {
		m.PathSmartctl, _ = exec.LookPath("smartctl")
	}

	// if `path_nvme` is not provided in config, try to find nvme binary in PATH
	if len(m.PathNVMe) == 0 {
		m.PathNVMe, _ = exec.LookPath("nvme")
	}

	if !contains(knownReadMethods, m.ReadMethod) {
		return fmt.Errorf("provided read method %q is not valid", m.ReadMethod)
	}

	err := validatePath(m.PathSmartctl)
	if err != nil {
		m.PathSmartctl = ""
		// without smartctl, plugin will not be able to gather basic metrics
		return fmt.Errorf("smartctl not found: verify that smartctl is installed and it is in your PATH (or specified in config): %w", err)
	}

	err = validatePath(m.PathNVMe)
	if err != nil {
		m.PathNVMe = ""
		// without nvme, plugin will not be able to gather vendor specific attributes (but it can work without it)
		log.Printf(
			"W! nvme not found: verify that nvme is installed and it is in your PATH (or specified in config) to gather vendor specific attributes: %s",
			err.Error(),
		)
	}

	return nil
}

// Gather takes in an accumulator and adds the metrics that the SMART tools gather.
func (m *Instance) Gather(slist *types.SampleList) {
	var err error
	var scannedNVMeDevices []string
	var scannedNonNVMeDevices []string

	devicesFromConfig := m.Devices
	isNVMe := len(m.PathNVMe) != 0
	isVendorExtension := len(m.EnableExtensions) != 0

	if len(m.Devices) != 0 {
		m.getAttributes(slist, devicesFromConfig)

		// if nvme-cli is present, vendor specific attributes can be gathered
		if isVendorExtension && isNVMe {
			scannedNVMeDevices, _, err = m.scanAllDevices(true)
			if err != nil {
				log.Println("E! error while scanning devices:", err)
				return
			}
			nvmeDevices := distinguishNVMeDevices(devicesFromConfig, scannedNVMeDevices)

			m.getVendorNVMeAttributes(slist, nvmeDevices)
		}
		return
	}
	scannedNVMeDevices, scannedNonNVMeDevices, err = m.scanAllDevices(false)
	if err != nil {
		log.Println("E! error while scanning all devices:", err)
		return
	}
	var devicesFromScan []string
	devicesFromScan = append(devicesFromScan, scannedNVMeDevices...)
	devicesFromScan = append(devicesFromScan, scannedNonNVMeDevices...)

	m.getAttributes(slist, devicesFromScan)
	if isVendorExtension && isNVMe {
		m.getVendorNVMeAttributes(slist, scannedNVMeDevices)
	}
	return
}

func (m *Instance) scanAllDevices(ignoreExcludes bool) ([]string, []string, error) {
	// this will return all devices (including NVMe devices) for smartctl version >= 7.0
	// for older versions this will return non NVMe devices
	devices, err := m.scanDevices(ignoreExcludes, "--scan")
	if err != nil {
		return nil, nil, err
	}

	// this will return only NVMe devices
	nvmeDevices, err := m.scanDevices(ignoreExcludes, "--scan", "--device=nvme")
	if err != nil {
		return nil, nil, err
	}

	// to handle all versions of smartctl this will return only non NVMe devices
	nonNVMeDevices := difference(devices, nvmeDevices)
	return nvmeDevices, nonNVMeDevices, nil
}

func distinguishNVMeDevices(userDevices []string, availableNVMeDevices []string) []string {
	var nvmeDevices []string

	for _, userDevice := range userDevices {
		for _, availableNVMeDevice := range availableNVMeDevices {
			// double check. E.g. in case when nvme0 is equal nvme0n1, will check if "nvme0" part is present.
			if strings.Contains(availableNVMeDevice, userDevice) || strings.Contains(userDevice, availableNVMeDevice) {
				nvmeDevices = append(nvmeDevices, userDevice)
			}
		}
	}
	return nvmeDevices
}

// Scan for S.M.A.R.T. devices from smartctl
func (m *Instance) scanDevices(ignoreExcludes bool, scanArgs ...string) ([]string, error) {
	out, err := runCmd(m.Timeout, m.UseSudo, m.PathSmartctl, scanArgs...)
	if err != nil {
		return []string{}, fmt.Errorf("failed to run command '%s %s': %w - %s", m.PathSmartctl, scanArgs, err, string(out))
	}
	var devices []string
	for _, line := range strings.Split(string(out), "\n") {
		dev := strings.Split(line, " ")
		if len(dev) <= 1 {
			continue
		}
		if !ignoreExcludes {
			if !excludedDev(m.Excludes, strings.TrimSpace(dev[0])) {
				devices = append(devices, strings.TrimSpace(dev[0]))
			}
		} else {
			devices = append(devices, strings.TrimSpace(dev[0]))
		}
	}
	return devices, nil
}

// Wrap with sudo
var runCmd = func(timeout config.Duration, sudo bool, command string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout))
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	if sudo {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n", command}, args...)...)
	}
	return cmd.CombinedOutput()
}

func excludedDev(excludes []string, deviceLine string) bool {
	device := strings.Split(deviceLine, " ")
	if len(device) != 0 {
		for _, exclude := range excludes {
			if device[0] == exclude {
				return true
			}
		}
	}
	return false
}

// Get info and attributes for each S.M.A.R.T. device
func (m *Instance) getAttributes(slist *types.SampleList, devices []string) {
	var wg sync.WaitGroup
	wg.Add(len(devices))
	for _, device := range devices {
		switch m.ReadMethod {
		case "concurrent":
			go m.gatherDisk(slist, device, &wg)
		case "sequential":
			m.gatherDisk(slist, device, &wg)
		default:
			wg.Done()
		}
	}

	wg.Wait()
}

func (m *Instance) getVendorNVMeAttributes(slist *types.SampleList, devices []string) {
	nvmeDevices := getDeviceInfoForNVMeDisks(slist, devices, m.PathNVMe, m.Timeout, m.UseSudo)

	var wg sync.WaitGroup

	for _, device := range nvmeDevices {
		if contains(m.EnableExtensions, "auto-on") {
			//nolint:revive // one case switch on purpose to demonstrate potential extensions
			switch device.vendorID {
			case intelVID:
				wg.Add(1)
				switch m.ReadMethod {
				case "concurrent":
					go gatherIntelNVMeDisk(slist, m.Timeout, m.UseSudo, m.PathNVMe, device, &wg)
				case "sequential":
					gatherIntelNVMeDisk(slist, m.Timeout, m.UseSudo, m.PathNVMe, device, &wg)
				default:
					wg.Done()
				}
			}
		} else if contains(m.EnableExtensions, "Intel") && device.vendorID == intelVID {
			wg.Add(1)
			switch m.ReadMethod {
			case "concurrent":
				go gatherIntelNVMeDisk(slist, m.Timeout, m.UseSudo, m.PathNVMe, device, &wg)
			case "sequential":
				gatherIntelNVMeDisk(slist, m.Timeout, m.UseSudo, m.PathNVMe, device, &wg)
			default:
				wg.Done()
			}
		}
	}
	wg.Wait()
}

func getDeviceInfoForNVMeDisks(slist *types.SampleList, devices []string, nvme string, timeout config.Duration, useSudo bool) []nvmeDevice {
	nvmeDevices := make([]nvmeDevice, 0, len(devices))
	for _, device := range devices {
		newDevice, err := gatherNVMeDeviceInfo(nvme, device, timeout, useSudo)
		if err != nil {
			log.Printf("E! cannot find device info for %s device", device)
			continue
		}
		nvmeDevices = append(nvmeDevices, newDevice)
	}
	return nvmeDevices
}

func gatherNVMeDeviceInfo(nvme, deviceName string, timeout config.Duration, useSudo bool) (device nvmeDevice, err error) {
	args := []string{"id-ctrl"}
	args = append(args, strings.Split(deviceName, " ")...)
	out, err := runCmd(timeout, useSudo, nvme, args...)
	if err != nil {
		return device, err
	}
	outStr := string(out)
	device, err = findNVMeDeviceInfo(outStr)
	if err != nil {
		return device, err
	}
	device.name = deviceName
	return device, nil
}

func findNVMeDeviceInfo(output string) (nvmeDevice, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var vid, sn, mn string

	for scanner.Scan() {
		line := scanner.Text()

		if matches := nvmeIDCtrlExpressionPattern.FindStringSubmatch(line); len(matches) > 2 {
			matches[1] = strings.TrimSpace(matches[1])
			matches[2] = strings.TrimSpace(matches[2])
			if matches[1] == "vid" {
				if _, err := fmt.Sscanf(matches[2], "%s", &vid); err != nil {
					return nvmeDevice{}, err
				}
			}
			if matches[1] == "sn" {
				sn = matches[2]
			}
			if matches[1] == "mn" {
				mn = matches[2]
			}
		}
	}

	newDevice := nvmeDevice{
		vendorID:     vid,
		model:        mn,
		serialNumber: sn,
	}
	return newDevice, nil
}

func gatherIntelNVMeDisk(slist *types.SampleList, timeout config.Duration, usesudo bool, nvme string, device nvmeDevice, wg *sync.WaitGroup) {
	defer wg.Done()

	args := []string{"intel", "smart-log-add"}
	args = append(args, strings.Split(device.name, " ")...)
	out, e := runCmd(timeout, usesudo, nvme, args...)
	outStr := string(out)

	_, er := exitStatus(e)
	if er != nil {
		log.Printf("E! failed to run command '%s %s': %w - %s", nvme, strings.Join(args, " "), e, outStr)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(outStr))

	for scanner.Scan() {
		line := scanner.Text()
		tags := map[string]string{}
		fields := make(map[string]interface{})

		tags["device"] = path.Base(device.name)
		tags["model"] = strings.Join(strings.Split(tags["model"], " "), "_")
		tags["serial_no"] = device.serialNumber

		// Create struct to initialize later with intel attributes.
		var (
			attr = struct {
				ID    string
				Name  string
				Parse func(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error
			}{}
			attrExists bool
		)

		if matches := intelExpressionPattern.FindStringSubmatch(line); len(matches) > 3 && len(matches[1]) > 1 {
			// Check if nvme shows metrics in deprecated format or in format with ID.
			// Based on that, an attribute map with metrics is chosen.
			// If string has more than one character it means it has KEY there, otherwise it's empty string ("").
			if separatedIDAndKey := nvmeIDSeparatePattern.FindStringSubmatch(matches[1]); len(strings.TrimSpace(separatedIDAndKey[2])) > 1 {
				matches[1] = strings.TrimSpace(separatedIDAndKey[2])
				attr, attrExists = intelAttributes[matches[1]]
			} else {
				matches[1] = strings.TrimSpace(matches[1])
				attr, attrExists = intelAttributesDeprecatedFormat[matches[1]]
			}

			matches[3] = strings.TrimSpace(matches[3])

			if attrExists {
				tags["name"] = attr.Name
				if attr.ID != "" {
					tags["id"] = attr.ID
				}

				parse := parseCommaSeparatedIntWithAccumulator
				if attr.Parse != nil {
					parse = attr.Parse
				}

				if err := parse(slist, fields, tags, stringx.SnakeCase(attr.Name), matches[3]); err != nil {
					continue
				}
			}
		}
	}
}

func (m *Instance) gatherDisk(slist *types.SampleList, device string, wg *sync.WaitGroup) {
	defer wg.Done()
	// smartctl 5.41 & 5.42 have are broken regarding handling of --nocheck/-n
	args := []string{"--info", "--health", "--attributes", "--tolerance=verypermissive", "-n", m.Nocheck, "--format=brief"}
	args = append(args, strings.Split(device, " ")...)
	out, e := runCmd(m.Timeout, m.UseSudo, m.PathSmartctl, args...)
	outStr := string(out)

	// Ignore all exit statuses except if it is a command line parse error
	exitStatus, er := exitStatus(e)
	if er != nil {
		log.Printf("E! failed to run command '%s %s': %w - %s", m.PathSmartctl, strings.Join(args, " "), e, outStr)
		return
	}

	deviceTags := map[string]string{}
	deviceNode := strings.Split(device, " ")[0]
	deviceTags["device"] = path.Base(deviceNode)
	deviceFields := make(map[string]interface{})
	deviceFields["exit_status"] = exitStatus

	scanner := bufio.NewScanner(strings.NewReader(outStr))

	for scanner.Scan() {
		line := scanner.Text()

		model := modelInfo.FindStringSubmatch(line)
		if len(model) > 2 {
			deviceTags["model"] = strings.Join(strings.Split(model[2], " "), "_")
		}

		serial := serialInfo.FindStringSubmatch(line)
		if len(serial) > 1 {
			deviceTags["serial_no"] = serial[1]
		}

		wwn := wwnInfo.FindStringSubmatch(line)
		if len(wwn) > 1 {
			deviceTags["wwn"] = strings.ReplaceAll(wwn[1], " ", "")
		}

		capacity := userCapacityInfo.FindStringSubmatch(line)
		if len(capacity) > 1 {
			deviceTags["capacity"] = strings.ReplaceAll(capacity[1], ",", "")
		}

		enabled := smartEnabledInfo.FindStringSubmatch(line)
		if len(enabled) > 1 {
			deviceTags["enabled"] = enabled[1]
		}

		health := smartOverallHealth.FindStringSubmatch(line)
		if len(health) > 2 {
			if health[2] == "PASSED" || health[2] == "OK" {
				deviceFields["health_ok"] = 1
			} else {
				deviceFields["health_ok"] = 0
			}
		}

		// checks to see if there is a power mode to print to user
		// if not look for Device is in STANDBY which happens when
		// nocheck is set to standby (will exit to not spin up the disk)
		// otherwise nothing is found so nothing is printed (NVMe does not show power)
		if power := powermodeInfo.FindStringSubmatch(line); len(power) > 1 {
			deviceTags["power"] = power[1]
		} else {
			if power := standbyInfo.FindStringSubmatch(line); len(power) > 1 {
				deviceTags["power"] = power[1]
			}
		}

		tags := map[string]string{}
		fields := make(map[string]interface{})

		if m.Attributes {
			// add power mode
			keys := [...]string{"device", "model", "serial_no", "wwn", "capacity", "enabled", "power"}
			for _, key := range keys {
				if value, ok := deviceTags[key]; ok {
					tags[key] = value
				}
			}
		}

		attr := attribute.FindStringSubmatch(line)
		if len(attr) > 1 {
			// attribute has been found, add it only if m.Attributes is true
			if m.Attributes {
				tags["id"] = attr[1]
				tags["name"] = attr[2]
				tags["flags"] = attr[3]

				fields["exit_status"] = exitStatus
				if i, err := strconv.ParseInt(attr[4], 10, 64); err == nil {
					fields["value"] = i
				}
				if i, err := strconv.ParseInt(attr[5], 10, 64); err == nil {
					fields["worst"] = i
				}
				if i, err := strconv.ParseInt(attr[6], 10, 64); err == nil {
					fields["threshold"] = i
				}

				tags["fail"] = attr[7]
				metric := stringx.SnakeCase(attr[2])
				if val, err := parseRawValue(attr[8]); err == nil {
					fields[metric] = val
				}

				slist.PushSamples("smart_attribute", fields, tags)
			}

			// If the attribute matches on the one in deviceFieldIds
			// save the raw value to a field.
			if field, ok := deviceFieldIds[attr[1]]; ok {
				if val, err := parseRawValue(attr[8]); err == nil {
					deviceFields[field] = val
				}
			}

			if len(attr) > 4 {
				// If the attribute name matches on in deviceFieldNames
				// save the value to a field
				if field, ok := deviceFieldNames[attr[2]]; ok {
					if val, err := parseRawValue(attr[4]); err == nil {
						deviceFields[field] = val
					}
				}
			}
		} else {
			// what was found is not a vendor attribute
			if matches := sasNVMeAttr.FindStringSubmatch(line); len(matches) > 2 {
				if attr, ok := sasNVMeAttributes[matches[1]]; ok {
					tags["name"] = attr.Name
					metric := stringx.SnakeCase(attr.Name)
					if attr.ID != "" {
						tags["id"] = attr.ID
					}

					parse := parseCommaSeparatedInt
					if attr.Parse != nil {
						parse = attr.Parse
					}

					if err := parse(fields, deviceFields, metric, matches[2]); err != nil {
						log.Printf("E!error parsing %s: %q: %w", attr.Name, matches[2], err)
						continue
					}
					// if the field is classified as an attribute, only add it
					// if m.Attributes is true
					if m.Attributes {
						slist.PushSamples("smart_attribute", fields, tags)
					}
				}
			}
		}
	}
	slist.PushSamples("smart_device", deviceFields, deviceTags)
}

// Command line parse errors are denoted by the exit code having the 0 bit set.
// All other errors are drive/communication errors and should be ignored.
func exitStatus(err error) (int, error) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, err
}

func contains(args []string, element string) bool {
	for _, arg := range args {
		if arg == element {
			return true
		}
	}
	return false
}

func difference(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}

func parseRawValue(rawVal string) (int64, error) {
	// Integer
	if i, err := strconv.ParseInt(rawVal, 10, 64); err == nil {
		return i, nil
	}

	// Duration: 65h+33m+09.259s
	unit := regexp.MustCompile("^(.*)([hms])$")
	parts := strings.Split(rawVal, "+")
	if len(parts) == 0 {
		return 0, fmt.Errorf("couldn't parse RAW_VALUE %q", rawVal)
	}

	duration := int64(0)
	for _, part := range parts {
		timePart := unit.FindStringSubmatch(part)
		if len(timePart) == 0 {
			continue
		}
		switch timePart[2] {
		case "h":
			duration += parseInt(timePart[1]) * int64(3600)
		case "m":
			duration += parseInt(timePart[1]) * int64(60)
		case "s":
			// drop fractions of seconds
			duration += parseInt(strings.Split(timePart[1], ".")[0])
		default:
			// Unknown, ignore
		}
	}
	return duration, nil
}

func parseBytesWritten(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
	var value int64

	if _, err := fmt.Sscanf(str, "sectors: %d", &value); err != nil {
		return err
	}
	fields["bytes_written"] = value
	slist.PushSamples("smart_attribute", fields, tags)
	return nil
}

func parseThermalThrottle(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
	var percentage float64
	var count int64

	if _, err := fmt.Sscanf(str, "%f%%, cnt: %d", &percentage, &count); err != nil {
		return err
	}

	fields["thermal_throttle_status_prc"] = percentage
	tags["name"] = "Thermal_Throttle_Status_Prc"
	slist.PushSamples("smart_attribute", fields, tags)

	fields["thermal_throttle_status_cnt"] = count
	tags["name"] = "Thermal_Throttle_Status_Cnt"
	slist.PushSamples("smart_attribute", fields, tags)

	return nil
}

func parseWearLeveling(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
	var min, max, avg int64

	if _, err := fmt.Sscanf(str, "min: %d, max: %d, avg: %d", &min, &max, &avg); err != nil {
		return err
	}
	values := []int64{min, max, avg}
	for i, submetricName := range []string{"Min", "Max", "Avg"} {
		name := fmt.Sprintf("%s_%s", metric, submetricName)
		tags["name"] = name
		fields[stringx.SnakeCase(name)] = values[i]
		slist.PushSamples("smart_attribute", fields, tags)
	}

	return nil
}

func parseTimedWorkload(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
	var value float64

	if _, err := fmt.Sscanf(str, "%f", &value); err != nil {
		return err
	}
	// fields["timed_workload"] = value
	fields[stringx.SnakeCase(metric)] = value

	slist.PushSamples("smart_attribute", fields, tags)
	return nil
}

func parseInt(str string) int64 {
	if i, err := strconv.ParseInt(str, 10, 64); err == nil {
		return i
	}
	return 0
}

func parseCommaSeparatedInt(fields, _ map[string]interface{}, metric, str string) error {
	// remove any non-utf8 values
	// '1\xa0292' --> 1292
	value := strings.ToValidUTF8(strings.Join(strings.Fields(str), ""), "")

	// remove any non-alphanumeric values
	// '16,626,888' --> 16626888
	// '16 829 004' --> 16829004
	numRegex, err := regexp.Compile(`[^0-9\-]+`)
	if err != nil {
		return fmt.Errorf("failed to compile numeric regex")
	}
	value = numRegex.ReplaceAllString(value, "")

	i, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}

	fields[metric] = i

	return nil
}

func parsePercentageInt(fields, deviceFields map[string]interface{}, metric, str string) error {
	return parseCommaSeparatedInt(fields, deviceFields, metric, strings.TrimSuffix(str, "%"))
}

func parseDataUnits(fields, deviceFields map[string]interface{}, metric, str string) error {
	// Remove everything after '['
	units := strings.Split(str, "[")[0]
	return parseCommaSeparatedInt(fields, deviceFields, metric, units)
}

func parseCommaSeparatedIntWithAccumulator(slist *types.SampleList, fields map[string]interface{}, tags map[string]string, metric, str string) error {
	i, err := strconv.ParseInt(strings.ReplaceAll(str, ",", ""), 10, 64)
	if err != nil {
		return err
	}

	fields[metric] = i
	slist.PushSamples("smart_attribute", fields, tags)
	return nil
}

func parseTemperature(fields, deviceFields map[string]interface{}, metric, str string) error {
	var temp int64
	if _, err := fmt.Sscanf(str, "%d C", &temp); err != nil {
		return err
	}

	fields[metric] = temp
	deviceFields["temp_c"] = temp

	return nil
}

func parseTemperatureSensor(fields, _ map[string]interface{}, metric, str string) error {
	var temp int64
	if _, err := fmt.Sscanf(str, "%d C", &temp); err != nil {
		return err
	}

	fields[metric] = temp

	return nil
}

func validatePath(filePath string) error {
	pathInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("provided path does not exist: [%s]", filePath)
	}
	if mode := pathInfo.Mode(); !mode.IsRegular() {
		return fmt.Errorf("provided path does not point to a regular file: [%s]", filePath)
	}
	return nil
}
