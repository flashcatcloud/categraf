package chrony

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/cmdx"
	"flashcat.cloud/categraf/types"
)

const inputName = "chrony"

type Chrony struct {
	config.PluginConfig
	ChronycCommand string `toml:"chronyc_command"`
	DNSLookup      bool   `toml:"dns_lookup"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Chrony{}
	})
}

func (c *Chrony) Clone() inputs.Input {
	return &Chrony{}
}

func (c *Chrony) Name() string {
	return inputName
}

func (c *Chrony) Init() error {
	if c.ChronycCommand == "" {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (c *Chrony) Gather(slist *types.SampleList) {
	if c.ChronycCommand == "" {
		return
	}

	flags := []string{}
	if !c.DNSLookup {
		flags = append(flags, "-n")
	}
	flags = append(flags, "tracking")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(c.ChronycCommand, flags...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err, timeout := cmdx.RunTimeout(cmd, time.Second*5)
	if timeout {
		log.Printf("E! run command: %s timeout", strings.Join(cmd.Args, " "))
		return
	}

	if err != nil {
		log.Printf("E! failed to run command: %s | error: %v | stdout: %s | stderr: %s",
			strings.Join(cmd.Args, " "), err, stdout.String(), stderr.String())
		return
	}

	fields, tags, err := processChronycOutput(stdout.String())
	if err != nil {
		log.Println("E! failed to gather chrony processOutput: ", err)
		return
	}

	if len(fields) == 0 {
		log.Println("E! Chrony input failed to collect metrics")
	}

	slist.PushSamples("chrony", fields, tags)
}

func processChronycOutput(out string) (map[string]interface{}, map[string]string, error) {
	tags := map[string]string{}
	fields := map[string]interface{}{}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		stats := strings.Split(line, ":")
		if len(stats) < 2 {
			return nil, nil, fmt.Errorf("unexpected output from chronyc, expected ':' in %s", out)
		}
		name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(stats[0]), " ", "_"))
		// ignore reference time
		if strings.Contains(name, "ref_time") {
			continue
		}
		valueFields := strings.Fields(stats[1])
		if len(valueFields) == 0 {
			return nil, nil, fmt.Errorf("unexpected output from chronyc: %s", out)
		}
		if strings.Contains(strings.ToLower(name), "stratum") {
			tags["stratum"] = valueFields[0]
			continue
		}
		if strings.Contains(strings.ToLower(name), "reference_id") {
			tags["reference_id"] = valueFields[0]
			continue
		}
		value, err := strconv.ParseFloat(valueFields[0], 64)
		if err != nil {
			tags[name] = strings.ToLower(strings.Join(valueFields, " "))
			continue
		}
		if strings.Contains(stats[1], "slow") {
			value = -value
		}
		fields[name] = value
	}
	return fields, tags, nil
}
