package keepalived

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
	if k.DebugMod {
		opts := &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.SourceKey {
					if source, ok := a.Value.Any().(*slog.Source); ok {
						source.File = filepath.Base(source.File)
						return slog.Any(slog.SourceKey, source)
					}
				}
				return a
			},
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
		slog.SetDefault(logger)
	}
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

	var (
		c      collector.Collector
		closer io.Closer
	)
	defer func() {
		if closer != nil {
			_ = closer.Close()
		}
	}()

	if k.ContainerName != "" {
		coll, err := container.NewKeepalivedContainerCollectorHost(
			k.SigJson,
			k.ContainerName,
			k.ContainerTmp,
			k.ContainerPidPath,
		)
		if err != nil {
			slog.Error("failed to create keepalived collector",
				"mode", "container",
				"container", k.ContainerName,
				"error", err)
			slist.PushSample(inputName, "up", 0)
			return
		}
		c = coll
		closer = coll
	} else {
		coll, err := host.NewKeepalivedHostCollectorHost(k.SigJson, k.PidPath)
		if err != nil {
			slog.Error("failed to create keepalived collector",
				"mode", "host",
				"pidPath", k.PidPath,
				"error", err)
			slist.PushSample(inputName, "up", 0)
			return
		}
		c = coll
	}

	// json support check
	if k.SigJson {
		jsonSupport, err := c.HasJSONSignalSupport()
		if err != nil {
			slog.Error("Error checking JSON signal support", "error", err)
			slist.PushSample(inputName, "up", 0)
			return
		}

		if !jsonSupport {
			slog.Error("Keepalived does not support JSON signal. Please use a version that supports it.")
			slist.PushSample(inputName, "up", 0)
			return
		}
	}
	kaCollector := collector.NewKeepalivedCollector(k.SigJson, k.CheckScriptPath, c)
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	err := inputs.Collect(kaCollector, slist)
	if err != nil {
		slog.Error("Failed to collect keepalived metrics", "error", err)
		slist.PushSample(inputName, "up", 0)
	}
}
