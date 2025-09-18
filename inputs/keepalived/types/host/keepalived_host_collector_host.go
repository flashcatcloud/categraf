package host

import (
	"bytes"
	"fmt"
	"github.com/hashicorp/go-version"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"flashcat.cloud/categraf/inputs/keepalived/collector"
	"flashcat.cloud/categraf/inputs/keepalived/types/utils"
)

// KeepalivedHostCollectorHost implements Collector for when Keepalived and Keepalived Exporter are both on a same host.
type KeepalivedHostCollectorHost struct {
	pidPath string
	version *version.Version
	useJSON bool

	SIGJSON  syscall.Signal
	SIGDATA  syscall.Signal
	SIGSTATS syscall.Signal
}

// NewKeepalivedHostCollectorHost is creating new instance of KeepalivedHostCollectorHost.
func NewKeepalivedHostCollectorHost(useJSON bool, pidPath string) (*KeepalivedHostCollectorHost, error) {
	k := &KeepalivedHostCollectorHost{
		useJSON: useJSON,
		pidPath: pidPath,
	}

	var err error
	if k.version, err = k.getKeepalivedVersion(); err != nil {
		slog.Debug("Version detection failed. Assuming it's the latest one.", "error", err)

	}

	if err = k.initSignals(); err != nil {
		return nil, err
	}

	return k, nil
}

func (k *KeepalivedHostCollectorHost) Refresh() error {
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

func (k *KeepalivedHostCollectorHost) initSignals() error {
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

// GetKeepalivedVersion returns Keepalived version.
func (k *KeepalivedHostCollectorHost) getKeepalivedVersion() (*version.Version, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command("bash", "-c", "keepalived -v")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Debug("Error getting keepalived version",
			"stderr", stderr.String(),
			"stdout", stdout.String(),
			"error", err,
		)

		return nil, fmt.Errorf("error getting keepalived version: %w", err)
	}

	return utils.ParseVersion(stderr.String())
}

func (k *KeepalivedHostCollectorHost) HasJSONSignalSupport() (bool, error) {
	cmd := exec.Command("keepalived", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("Failed to run keepalived --version command",
			"output", string(output),
			"error", err,
		)

		return false, err
	}

	if strings.Contains(string(output), "--enable-json") {
		return true, nil
	}
	slog.Debug("Keepalived does not support JSON signal",
		"version", k.version,
		"output", string(output),
	)

	return false, nil
}

// Signal sends signal to Keepalived process.
func (k *KeepalivedHostCollectorHost) signal(signal os.Signal) error {
	data, err := os.ReadFile(k.pidPath)
	if err != nil {
		slog.Debug("Failed to read Keepalived PID file",
			"path", k.pidPath,
			"error", err,
		)

		return err
	}

	pid, err := strconv.Atoi(strings.TrimSuffix(string(data), "\n"))
	if err != nil {
		slog.Debug("Failed to parse Keepalived PID",
			"path", k.pidPath,
			"pid", string(data),
			"error", err,
		)

		return err
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		slog.Debug("Failed to find Keepalived process",
			"path", k.pidPath,
			"pid", pid,
			"error", err,
		)

		return err
	}

	err = proc.Signal(signal)
	if err != nil {
		slog.Debug("Failed to send signal to Keepalived process",
			"path", k.pidPath,
			"pid", pid,
			"signal", signal,
			"error", err,
		)

		return err
	}

	return nil
}

// SigNum returns signal number for given signal name.
func (k *KeepalivedHostCollectorHost) sigNum(sigString string) (syscall.Signal, error) {
	if !utils.HasSigNumSupport(k.version) {
		return utils.GetDefaultSignal(sigString)
	}

	var stdout, stderr bytes.Buffer

	sigNumCommand := "keepalived --signum=" + sigString
	cmd := exec.Command("bash", "-c", sigNumCommand)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Debug("Error executing command to get signal number",
			"signal", sigString,
			"command", sigNumCommand,
			"stdout", stdout.String(),
			"stderr", stderr.String(),
			"error", err,
		)

		return utils.InvalidSignal, err
	}
	if parseSigNum(stdout, sigString) == -1 {
		return utils.InvalidSignal, fmt.Errorf("parse sigNum Invalid Signal")
	}
	return syscall.Signal(parseSigNum(stdout, sigString)), nil
}

func (k *KeepalivedHostCollectorHost) JSONVrrps() ([]collector.VRRP, error) {
	const fileName = "/tmp/keepalived.json"

	f, err := os.Open(fileName)
	if err != nil {
		slog.Debug("Failed to open JSON VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close file",
				"fileName", fileName,
				"error", err,
			)

		}
	}()

	return collector.ParseJSON(f)
}

func (k *KeepalivedHostCollectorHost) StatsVrrps() (map[string]*collector.VRRPStats, error) {
	const fileName = "/tmp/keepalived.stats"

	f, err := os.Open(fileName)
	if err != nil {
		slog.Debug("Failed to open Stats VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close Stats VRRP file",
				"fileName", fileName,
				"error", err,
			)

		}
	}()

	return collector.ParseStats(f)
}

func (k *KeepalivedHostCollectorHost) DataVrrps() (map[string]*collector.VRRPData, error) {
	const fileName = "/tmp/keepalived.data"

	f, err := os.Open(fileName)
	if err != nil {
		slog.Debug("Failed to open Data VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close Data VRRP file",
				"fileName", fileName,
				"error", err,
			)

		}
	}()

	return collector.ParseVRRPData(f)
}

func (k *KeepalivedHostCollectorHost) ScriptVrrps() ([]collector.VRRPScript, error) {
	const fileName = "/tmp/keepalived.data"

	f, err := os.Open(fileName)
	if err != nil {
		slog.Debug("Failed to open Script VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Debug("Failed to close Script VRRP file",
				"fileName", fileName,
				"error", err,
			)
		}
	}()

	return collector.ParseVRRPScript(f), nil
}

// HasVRRPScriptStateSupport check if Keepalived version supports VRRP Script State in output.
func (k *KeepalivedHostCollectorHost) HasVRRPScriptStateSupport() bool {
	return utils.HasVRRPScriptStateSupport(k.version)
}
