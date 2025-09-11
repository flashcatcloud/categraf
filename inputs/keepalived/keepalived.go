package keepalived

import (
	"log/slog"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/keepalived/collector"
	"flashcat.cloud/categraf/inputs/keepalived/types/container"
	"flashcat.cloud/categraf/inputs/keepalived/types/host"
	"flashcat.cloud/categraf/types"
)

const inputName = "keepalived"

type Keepalived struct {
	config.PluginConfig

	Enable bool `toml:"enable"`

	SigJson bool   `toml:"sig_json"`
	PidPath string `toml:"pid_path"`

	ContainerPidPath string `toml:"container_pid_path"`
	ContainerName    string `toml:"container_name"`
	ContainerTmp     string `toml:"container_tmp"`

	CheckScriptPath string `toml:"check_script_path"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Keepalived{}
	})
}

func (*Keepalived) Clone() inputs.Input {
	return &Keepalived{}
}

func (*Keepalived) Name() string {
	return inputName
}

func (k *Keepalived) Gather(slist *types.SampleList) {
	// default options
	if !k.Enable {
		return
	}
	if k.PidPath == "" {
		k.PidPath = "/var/run/keepalived.pid"
	}
	if k.ContainerTmp == "" {
		k.ContainerTmp = "/tmp"
	}

	var c collector.Collector
	if k.ContainerName != "" {
		c = container.NewKeepalivedContainerCollectorHost(
			k.SigJson,
			k.ContainerName,
			k.ContainerTmp,
			k.ContainerPidPath,
		)
	} else {
		c = host.NewKeepalivedHostCollectorHost(k.SigJson, k.PidPath)
	}

	// json support check
	if k.SigJson {
		jsonSupport, err := c.HasJSONSignalSupport()
		if err != nil {
			slog.Error("Error checking JSON signal support", "error", err)
			return
		}

		if !jsonSupport {
			slog.Error("Keepalived does not support JSON signal. Please use a version that supports it.")
			return
		}
	}
	kaCollector := collector.NewKeepalivedCollector(k.SigJson, k.CheckScriptPath, c)
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	err := inputs.Collect(kaCollector, slist)
	if err != nil {
		slog.Error("Keepalived does not support JSON signal. Please use a version that supports it.")
	}
}
