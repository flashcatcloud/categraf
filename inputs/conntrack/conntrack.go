//go:build linux
// +build linux

package conntrack

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "conntrack"

type Conntrack struct {
	config.PluginConfig
	Dirs  []string `toml:"dirs"`
	Files []string `toml:"files"`
	Quiet bool     `toml:"quiet"`
}

var dfltDirs = []string{
	"/proc/sys/net/ipv4/netfilter",
	"/proc/sys/net/netfilter",
}

var dfltFiles = []string{
	"ip_conntrack_count",
	"ip_conntrack_max",
	"nf_conntrack_count",
	"nf_conntrack_max",
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Conntrack{}
	})
}

func (c *Conntrack) Clone() inputs.Input {
	return &Conntrack{}
}

func (c *Conntrack) Name() string {
	return inputName
}

func (c *Conntrack) setDefaults() {
	if len(c.Dirs) == 0 {
		c.Dirs = dfltDirs
	}

	if len(c.Files) == 0 {
		c.Files = dfltFiles
	}
}

func (c *Conntrack) Init() error {
	c.setDefaults()
	return nil
}

func (c *Conntrack) Gather(slist *types.SampleList) {
	var metricKey string
	fields := make(map[string]interface{})

	for _, dir := range c.Dirs {
		for _, file := range c.Files {
			// NOTE: no system will have both nf_ and ip_ prefixes,
			// so we're safe to branch on suffix only.
			parts := strings.SplitN(file, "_", 2)
			if len(parts) < 2 {
				continue
			}
			metricKey = "ip_" + parts[1]

			fName := filepath.Join(dir, file)
			if _, err := os.Stat(fName); err != nil {
				continue
			}

			contents, err := os.ReadFile(fName)
			if err != nil {
				log.Println("E! failed to read file:", fName, "error:", err)
				continue
			}

			v := strings.TrimSpace(string(contents))
			fields[metricKey], err = strconv.ParseFloat(v, 64)
			if err != nil {
				log.Println("E! failed to parse metric, expected number but found:", v, "error:", err)
			}
		}
	}

	if len(fields) == 0 && !c.Quiet {
		log.Println("E! Conntrack input failed to collect metrics. Is the conntrack kernel module loaded?")
	}

	slist.PushSamples("conntrack", fields)
}
