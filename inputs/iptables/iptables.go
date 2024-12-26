//go:build linux
// +build linux

package iptables

import (
	"errors"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "iptables"

type Iptables struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Iptables{}
	})
}

func (ipt *Iptables) Clone() inputs.Input {
	return &Iptables{}
}

func (ipt *Iptables) Name() string {
	return inputName
}

func (ipt *Iptables) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(ipt.Instances))
	for i := 0; i < len(ipt.Instances); i++ {
		ret[i] = ipt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	UseSudo bool     `toml:"use_sudo"`
	UseLock bool     `toml:"use_lock"`
	Binary  string   `toml:"binary"`
	Table   string   `toml:"table"`
	Chains  []string `toml:"chains"`
	lister  chainLister
}

type chainLister func(table, chain string) (string, error)

func (ins *Instance) Init() error {
	if ins.Table == "" || len(ins.Chains) == 0 {
		log.Println("W! Table or Chains is empty")
		return types.ErrInstancesEmpty
	}
	if ins.lister == nil {
		ins.lister = ins.chainList
	}
	return nil
}

// Gather gathers iptables packets and bytes throughput from the configured tables and chains.
func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.Table == "" || len(ins.Chains) == 0 {
		log.Println("W! Table or Chains is empty")
		return
	}
	if ins.lister == nil {
		log.Println("E! Lister is empty or not initialized")
		return
	}
	// best effort : we continue through the chains even if an error is encountered,
	// but we keep track of the last error.
	for _, chain := range ins.Chains {
		data, err := ins.lister(ins.Table, chain)
		if err != nil {
			log.Println("E! ChainLister error:", err)
			continue
		}
		err = ins.parseAndGather(data, slist)
		if err != nil {
			log.Println("E! ParseAndGather failed:", err)
			continue
		}
	}
}

func (ins *Instance) chainList(table, chain string) (string, error) {
	var binary string
	if ins.Binary != "" {
		binary = ins.Binary
	} else {
		binary = "iptables"
	}
	iptablePath, err := exec.LookPath(binary)
	if err != nil {
		return "", err
	}
	var args []string
	name := iptablePath
	if ins.UseSudo {
		name = "sudo"
		args = append(args, iptablePath)
	}
	if ins.UseLock {
		args = append(args, "-w", "5")
	}
	args = append(args, "-nvL", chain, "-t", table, "-x")
	c := exec.Command(name, args...)
	out, err := c.Output()
	return string(out), err
}

var errParse = errors.New("E! Cannot parse iptables list information")
var chainNameRe = regexp.MustCompile(`^Chain\s+(\S+)`)
var fieldsHeaderRe = regexp.MustCompile(`^\s*pkts\s+bytes\s+target`)
var valuesRe = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\w+).*?/\*\s*(.+?)\s*\*/\s*`)

func (ins *Instance) parseAndGather(data string, slist *types.SampleList) error {
	lines := strings.Split(data, "\n")
	if len(lines) < 3 {
		return nil
	}
	mchain := chainNameRe.FindStringSubmatch(lines[0])
	if mchain == nil {
		return errParse
	}
	if !fieldsHeaderRe.MatchString(lines[1]) {
		return errParse
	}
	for _, line := range lines[2:] {
		matches := valuesRe.FindStringSubmatch(line)
		if len(matches) != 5 {
			continue
		}

		pkts := matches[1]
		bytes := matches[2]
		target := matches[3]
		comment := matches[4]

		tags := map[string]string{"table": ins.Table, "chain": mchain[1], "target": target, "ruleid": comment}
		fields := make(map[string]interface{})

		var err error
		fields["pkts"], err = strconv.ParseUint(pkts, 10, 64)
		if err != nil {
			continue
		}
		fields["bytes"], err = strconv.ParseUint(bytes, 10, 64)
		if err != nil {
			continue
		}
		slist.PushSamples(inputName, fields, tags)
	}
	return nil
}
