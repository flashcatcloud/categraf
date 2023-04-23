package systemd

import (
	"regexp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"github.com/coreos/go-systemd/v22/dbus"
)

const (
	// minSystemdVersionSystemState is the minimum SystemD version for availability of
	// the 'SystemState' manager property and the timer property 'LastTriggerUSec'
	// https://github.com/prometheus/node_exporter/issues/291
	minSystemdVersionSystemState = 212
	inputName                    = `systemd`
)

type (
	Systemd struct {
		config.PluginConfig
		Enable bool `toml:"enable"`

		UnitInclude string `toml:"unit_include"`
		UnitExclude string `toml:"unit_exclude"`

		unitIncludePattern *regexp.Regexp `toml:"-"`
		unitExcludePattern *regexp.Regexp `toml:"-"`

		SystemdPrivate         bool `toml:"systemd_private"`
		EnableTaskMetrics      bool `toml:"enable_task_metrics"`
		EnableRestartMetrics   bool `toml:"enable_restarts_metrics"`
		EnableStartTimeMetrics bool `toml:"enable_start_time_metrics"`

		conn *dbus.Conn
	}
	unit struct {
		dbus.UnitStatus
	}
)

var _ inputs.SampleGatherer = new(Systemd)

var (
	systemdVersionRE = regexp.MustCompile(`[0-9]{3,}(\.[0-9]+)?`)
	unitStatesName   = []string{"active", "activating", "deactivating", "inactive", "failed"}
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Systemd{}
	})
}

func (s *Systemd) Clone() inputs.Input {
	return &Systemd{}
}

func (s *Systemd) Name() string {
	return inputName
}
