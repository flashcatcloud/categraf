package ipmi

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

const (
	cmd = "ipmitool"
)

var (
	reV1ParseLine        = regexp.MustCompile(`^(?P<name>[^|]*)\|(?P<description>[^|]*)\|(?P<status_code>.*)`)
	reV2ParseLine        = regexp.MustCompile(`^(?P<name>[^|]*)\|[^|]+\|(?P<status_code>[^|]*)\|(?P<entity_id>[^|]*)\|(?:(?P<description>[^|]+))?`)
	reV2ParseDescription = regexp.MustCompile(`^(?P<analogValue>-?[0-9.]+)\s(?P<analogUnit>.*)|(?P<status>.+)|^$`)
	reV2ParseUnit        = regexp.MustCompile(`^(?P<realAnalogUnit>[^,]+)(?:,\s*(?P<statusDesc>.*))?`)
)

// Instance stores the configuration values for the ipmi_sensor input plugin
type Instance struct {
	config.InstanceConfig

	Path          string          `toml:"path"`
	Privilege     string          `toml:"privilege"`
	HexKey        string          `toml:"hex_key"`
	Servers       []string        `toml:"servers"`
	Timeout       config.Duration `toml:"timeout"`
	MetricVersion int             `toml:"metric_version" default:"1"`
	UseSudo       bool            `toml:"use_sudo"`
	UseCache      bool            `toml:"use_cache"`
	CachePath     string          `toml:"cache_path"`
}

func (m *Instance) Init() error {
	// Set defaults
	if m.Path == "" {
		path, err := exec.LookPath(cmd)
		if err != nil {
			return fmt.Errorf("looking up %q failed: %w", cmd, err)
		}
		m.Path = path
	}
	if m.CachePath == "" {
		m.CachePath = os.TempDir()
	}

	// Check parameters
	if m.Path == "" {
		return fmt.Errorf("no path for %q specified", cmd)
	}
	if m.Timeout == config.Duration(0) {
		m.Timeout = config.Duration(20 * time.Second)
	}

	if len(m.Servers) == 0 && !m.UseSudo {
		return types.ErrInstancesEmpty
	}

	return nil
}

// Gather is the main execution function for the plugin
func (m *Instance) Gather(slist *types.SampleList) {
	if len(m.Path) == 0 {
		log.Println("ipmitool not found: verify that ipmitool is installed and that ipmitool is in your PATH")
		return
	}

	if len(m.Servers) > 0 {
		wg := sync.WaitGroup{}
		for _, server := range m.Servers {
			wg.Add(1)
			go func(slit *types.SampleList, s string) {
				defer wg.Done()
				err := m.parse(slist, s)
				if err != nil {
					log.Println("E! [inputs.ipmi] error: ", err)
				}
			}(slist, server)
		}
		wg.Wait()
	} else {
		err := m.parse(slist, "")
		if err != nil {
			log.Println("E! [inputs.ipmi] error: ", err)
			return
		}
	}
}

func (m *Instance) parse(slist *types.SampleList, server string) error {
	opts := make([]string, 0)
	hostname := ""
	if server != "" {
		conn := NewConnection(server, m.Privilege, m.HexKey)
		hostname = conn.Hostname
		opts = conn.options()
	}
	opts = append(opts, "sdr")
	if m.UseCache {
		cacheFile := filepath.Join(m.CachePath, server+"_ipmi_cache")
		_, err := os.Stat(cacheFile)
		if os.IsNotExist(err) {
			dumpOpts := opts
			// init cache file
			dumpOpts = append(dumpOpts, "dump")
			dumpOpts = append(dumpOpts, cacheFile)
			name := m.Path
			if m.UseSudo {
				// -n - avoid prompting the user for input of any kind
				dumpOpts = append([]string{"-n", name}, dumpOpts...)
				name = "sudo"
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.Timeout))
			defer cancel()
			cmd := exec.CommandContext(ctx, name, dumpOpts...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to run command %q: %w - %s", strings.Join(sanitizeIPMICmd(cmd.Args), " "), err, string(out))
			}
		}
		opts = append(opts, "-S")
		opts = append(opts, cacheFile)
	}
	if m.MetricVersion == 2 {
		opts = append(opts, "elist")
	}
	name := m.Path
	if m.UseSudo {
		// -n - avoid prompting the user for input of any kind
		opts = append([]string{"-n", name}, opts...)
		name = "sudo"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.Timeout))
	defer cancel()
	cmd := exec.CommandContext(ctx, name, opts...)
	out, err := cmd.CombinedOutput()
	timestamp := time.Now()
	if err != nil {
		return fmt.Errorf("failed to run command %q: %w - %s", strings.Join(sanitizeIPMICmd(cmd.Args), " "), err, string(out))
	}
	if m.MetricVersion == 2 {
		return m.parseV2(slist, hostname, out, timestamp)
	}
	return m.parseV1(slist, hostname, out, timestamp)
}

func (m *Instance) parseV1(slist *types.SampleList, hostname string, cmdOut []byte, measuredAt time.Time) error {
	// each line will look something like
	// Planar VBAT      | 3.05 Volts        | ok
	scanner := bufio.NewScanner(bytes.NewReader(cmdOut))
	for scanner.Scan() {
		ipmiFields := m.extractFieldsFromRegex(reV1ParseLine, scanner.Text())
		if len(ipmiFields) != 3 {
			continue
		}

		metric := transform(ipmiFields["name"])
		tags := map[string]string{}

		// tag the server is we have one
		if hostname != "" {
			tags["server"] = hostname
		}

		fields := make(map[string]interface{})
		tags["status_code"] = trim(ipmiFields["status_code"])
		description := ipmiFields["description"]
		tags["description"] = transform(description)

		// handle hex description field
		if strings.HasPrefix(description, "0x") {
			descriptionInt, err := strconv.ParseInt(description, 0, 64)
			if err != nil {
				continue
			}

			fields[metric] = float64(descriptionInt)
			delete(tags, "description")
		} else if strings.Index(description, " ") > 0 {
			// split middle column into value and unit
			valunit := strings.SplitN(description, " ", 2)
			var err error
			fields[metric], err = aToFloat(valunit[0])
			if err != nil {
				fields[metric] = 0.0
			} else {
				delete(tags, "description")
				if len(valunit) > 1 {
					tags["unit"] = transform(valunit[1])
				}
			}
		} else {
			fields[metric] = 0.0
		}

		slist.PushSamples(inputName, fields, tags)
	}

	return scanner.Err()
}

func (m *Instance) parseV2(slist *types.SampleList, hostname string, cmdOut []byte, measuredAt time.Time) error {
	// each line will look something like
	// CMOS Battery     | 65h | ok  |  7.1 |
	// Temp             | 0Eh | ok  |  3.1 | 55 degrees C
	// Drive 0          | A0h | ok  |  7.1 | Drive Present
	scanner := bufio.NewScanner(bytes.NewReader(cmdOut))
	for scanner.Scan() {
		ipmiFields := m.extractFieldsFromRegex(reV2ParseLine, scanner.Text())
		if len(ipmiFields) < 3 || len(ipmiFields) > 4 {
			continue
		}

		tags := map[string]string{}

		metric := transform(ipmiFields["name"])
		// tag the server is we have one
		if hostname != "" {
			tags["server"] = hostname
		}
		tags["entity_id"] = transform(ipmiFields["entity_id"])
		tags["status_code"] = trim(ipmiFields["status_code"])
		tags["description"] = transform(ipmiFields["description"])

		fields := make(map[string]interface{})
		descriptionResults := m.extractFieldsFromRegex(reV2ParseDescription, trim(ipmiFields["description"]))
		// This is an analog value with a unit
		if descriptionResults["analogValue"] != "" && len(descriptionResults["analogUnit"]) >= 1 {
			var err error
			fields[metric], err = aToFloat(descriptionResults["analogValue"])
			if err != nil {
				continue
			}
			// Some implementations add an extra status to their analog units
			unitResults := m.extractFieldsFromRegex(reV2ParseUnit, descriptionResults["analogUnit"])
			tags["unit"] = transform(unitResults["realAnalogUnit"])
			if unitResults["statusDesc"] != "" {
				tags["status_desc"] = transform(unitResults["statusDesc"])
			}
		} else {
			// This is a status value
			fields[metric] = 0.0
			// Extended status descriptions aren't required, in which case for consistency re-use the status code
			if descriptionResults["status"] != "" {
				tags["status_desc"] = transform(descriptionResults["status"])
			} else {
				tags["status_desc"] = transform(ipmiFields["status_code"])
			}
		}

		slist.PushSamples(inputName, fields, tags)
	}

	return scanner.Err()
}

// extractFieldsFromRegex consumes a regex with named capture groups and returns a kvp map of strings with the results
func (m *Instance) extractFieldsFromRegex(re *regexp.Regexp, input string) map[string]string {
	submatches := re.FindStringSubmatch(input)
	results := make(map[string]string)
	subexpNames := re.SubexpNames()
	if len(subexpNames) > len(submatches) {
		return results
	}
	for i, name := range subexpNames {
		if name != input && name != "" && input != "" {
			results[name] = trim(submatches[i])
		}
	}
	return results
}

// aToFloat converts string representations of numbers to float64 values
func aToFloat(val string) (float64, error) {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0.0, err
	}
	return f, nil
}

func sanitizeIPMICmd(args []string) []string {
	for i, v := range args {
		if v == "-P" {
			args[i+1] = "REDACTED"
		}
	}

	return args
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func transform(s string) string {
	s = trim(s)
	s = strings.ToLower(s)
	return strings.ReplaceAll(s, " ", "_")
}

func (m *Instance) Drop() {
	if m.UseCache && m.CachePath != "" {
		os.RemoveAll(m.CachePath)
	}
}
