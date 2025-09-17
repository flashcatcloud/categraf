package container

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/hashicorp/go-version"

	"flashcat.cloud/categraf/inputs/keepalived/collector"
	"flashcat.cloud/categraf/inputs/keepalived/types/utils"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
)

// KeepalivedContainerCollectorHost implements Collector for when Keepalived is on container and Keepalived Exporter is on a host.
type KeepalivedContainerCollectorHost struct {
	version       *version.Version
	useJSON       bool
	containerName string
	dataPath      string
	jsonPath      string
	statsPath     string
	dockerCli     *client.Client
	pidPath       string

	SIGJSON  syscall.Signal
	SIGDATA  syscall.Signal
	SIGSTATS syscall.Signal
}

// NewKeepalivedContainerCollectorHost is creating new instance of KeepalivedContainerCollectorHost.
func NewKeepalivedContainerCollectorHost(
	useJSON bool,
	containerName, containerTmpDir, pidPath string,
) (*KeepalivedContainerCollectorHost, error) {
	k := &KeepalivedContainerCollectorHost{
		useJSON:       useJSON,
		containerName: containerName,
		pidPath:       pidPath,
	}

	var err error

	k.dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("Error creating docker env client: %w", err)
	}

	k.version, err = k.getKeepalivedVersion()
	if err != nil {
		slog.Debug("Version detection failed. Assuming it's the latest one.", "error", err)

	}

	if err = k.initSignals(); err != nil {
		_ = k.dockerCli.Close()
		return nil, err
	}

	k.initPaths(containerTmpDir)

	return k, nil
}

func (k *KeepalivedContainerCollectorHost) Refresh() error {
	if k.useJSON {
		if err := k.signal(k.SIGJSON); err != nil {
			slog.Debug("Failed to send JSON signal to keepalived", "error", err)

			return err
		}

		return nil
	}

	if err := k.signal(k.SIGSTATS); err != nil {
		slog.Debug("Failed to send STATS signal to keepalived", "error", err)

		return err
	}

	if err := k.signal(k.SIGDATA); err != nil {
		slog.Debug("Failed to send DATA signal to keepalived", "error", err)

		return err
	}

	return nil
}

func (k *KeepalivedContainerCollectorHost) initPaths(containerTmpDir string) {
	k.jsonPath = filepath.Join(containerTmpDir, "keepalived.json")
	k.statsPath = filepath.Join(containerTmpDir, "keepalived.stats")
	k.dataPath = filepath.Join(containerTmpDir, "keepalived.data")
}

// GetKeepalivedVersion returns Keepalived version.
func (k *KeepalivedContainerCollectorHost) getKeepalivedVersion() (*version.Version, error) {
	getVersionCmd := []string{"keepalived", "-v"}

	stdout, err := k.dockerExecCmd(getVersionCmd)
	if err != nil {
		return nil, err
	}

	return utils.ParseVersion(stdout.String())
}

func (k *KeepalivedContainerCollectorHost) initSignals() error {
	var err error

	if k.useJSON {
		if k.SIGJSON, err = k.sigNum("JSON"); err != nil {
			return fmt.Errorf("init SIGJSON: %w", err)
		}
	}

	if k.SIGDATA, err = k.sigNum("DATA"); err != nil {
		return fmt.Errorf("init SIGDATA: %w", err)
	}

	if k.SIGSTATS, err = k.sigNum("STATS"); err != nil {
		return fmt.Errorf("init SIGSTATS: %w", err)
	}

	return nil
}

// SigNum returns signal number for given signal name.
func (k *KeepalivedContainerCollectorHost) sigNum(sigString string) (syscall.Signal, error) {
	if !utils.HasSigNumSupport(k.version) {
		return utils.GetDefaultSignal(sigString)
	}

	sigNumCommand := []string{"keepalived", "--signum", sigString}

	stdout, err := k.dockerExecCmd(sigNumCommand)
	if err != nil {
		slog.Debug("Error executing command to get signal number",
			"signal", sigString,
			"container", k.containerName,
			"error", err,
		)

		return utils.InvalidSignal, fmt.Errorf("Error executing command to get signal number for keepalived: %s", sigString)
	}

	reg := regexp.MustCompile("[^0-9]+")
	strSigNum := reg.ReplaceAllString(stdout.String(), "")

	signum, err := strconv.ParseInt(strSigNum, 10, 32)
	if err != nil {

		slog.Debug("Error parsing signal number",
			"signal", sigString,
			"signum", strSigNum,
			"container", k.containerName,
			"error", err,
		)

		return utils.InvalidSignal, fmt.Errorf("invalid signum %q for signal %s", strSigNum, sigString)
	}

	return syscall.Signal(signum), nil
}

func (k *KeepalivedContainerCollectorHost) dockerExecSignal(signal syscall.Signal) error {
	pidData, err := os.ReadFile(k.pidPath)
	if err != nil {
		slog.Debug("Failed to read keepalived pid file",
			"error", err,
			"path", k.pidPath,
		)

		return err
	}

	pid := strings.TrimSpace(string(pidData))
	cmd := strslice.StrSlice{"kill", "-" + strconv.Itoa(int(signal)), pid}

	_, err = k.dockerExecCmd(cmd)

	return err
}

func (k *KeepalivedContainerCollectorHost) dockerSignal(signal syscall.Signal) error {
	err := k.dockerCli.ContainerKill(context.Background(), k.containerName, strconv.Itoa(int(signal)))
	if err != nil {
		slog.Debug("Failed to send signal to keepalived container",
			"container", k.containerName,
			"signal", int(signal),
			"error", err,
		)

		return err
	}

	return nil
}

// Signal sends signal to Keepalived process.
func (k *KeepalivedContainerCollectorHost) signal(signal syscall.Signal) error {
	if k.pidPath != "" {
		return k.dockerExecSignal(signal)
	}

	return k.dockerSignal(signal)
}

// JSONVrrps send SIGJSON and parse the data to the list of collector.VRRP struct.
func (k *KeepalivedContainerCollectorHost) JSONVrrps() ([]collector.VRRP, error) {
	f, err := os.Open(k.jsonPath)
	if err != nil {
		slog.Debug("Failed to open keepalived.json",
			"error", err,
			"path", k.jsonPath,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close keepalived.json file",
				"error", err,
				"path", k.jsonPath,
			)

		}
	}()

	return collector.ParseJSON(f)
}

// StatsVrrps send SIGSTATS and parse the stats.
func (k *KeepalivedContainerCollectorHost) StatsVrrps() (map[string]*collector.VRRPStats, error) {
	f, err := os.Open(k.statsPath)
	if err != nil {
		slog.Debug("Failed to open keepalived.stats",
			"error", err,
			"path", k.statsPath,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close keepalived.stats file",
				"error", err,
				"path", k.statsPath,
			)

		}
	}()

	return collector.ParseStats(f)
}

// DataVrrps send SIGDATA ans parse the data.
func (k *KeepalivedContainerCollectorHost) DataVrrps() (map[string]*collector.VRRPData, error) {
	f, err := os.Open(k.dataPath)
	if err != nil {
		slog.Debug("Failed to open keepalived.data",
			"error", err,
			"path", k.dataPath,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close keepalived.data file",
				"error", err,
				"path", k.dataPath,
			)

		}
	}()

	return collector.ParseVRRPData(f)
}

// ScriptVrrps parse the script data from keepalived.data.
func (k *KeepalivedContainerCollectorHost) ScriptVrrps() ([]collector.VRRPScript, error) {
	f, err := os.Open(k.dataPath)
	if err != nil {
		slog.Debug("Failed to open keepalived.data",
			"error", err,
			"path", k.dataPath,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close keepalived.data file",
				"error", err,
				"path", k.dataPath,
			)

		}
	}()

	return collector.ParseVRRPScript(f), nil
}

// HasVRRPScriptStateSupport check if Keepalived version supports VRRP Script State in output.
func (k *KeepalivedContainerCollectorHost) HasVRRPScriptStateSupport() bool {
	return utils.HasVRRPScriptStateSupport(k.version)
}

func (k *KeepalivedContainerCollectorHost) HasJSONSignalSupport() (bool, error) {
	// exec command to check if SIGJSON is supported
	cmd := strslice.StrSlice{"keepalived", "--version"}
	output, err := k.dockerExecCmd(cmd)
	if err != nil {
		return false, err
	}

	if strings.Contains(output.String(), "--enable-json") {
		return true, nil
	}
	slog.Debug("Keepalived does not support JSON signal. Please check if it was compiled with --enable-json option",
		"container", k.containerName,
		"version", k.version,
	)

	return false, nil
}

func (k *KeepalivedContainerCollectorHost) Close() error {
	if k.dockerCli != nil {
		return k.dockerCli.Close()
	}
	return nil
}
