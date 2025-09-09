package host

import (
	"bytes"
	"errors"
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
func NewKeepalivedHostCollectorHost(useJSON bool, pidPath string) *KeepalivedHostCollectorHost {
	k := &KeepalivedHostCollectorHost{
		useJSON: useJSON,
		pidPath: pidPath,
	}

	var err error
	if k.version, err = k.getKeepalivedVersion(); err != nil {
		slog.Warn("Version detection failed. Assuming it's the latest one.", "error", err)
	}

	k.initSignals()

	return k
}

func (k *KeepalivedHostCollectorHost) Refresh() error {
	if k.useJSON {
		if err := k.signal(k.SIGJSON); err != nil {
			slog.Error("Failed to send JSON signal to keepalived", "error", err)

			return err
		}

		return nil
	}

	if err := k.signal(k.SIGSTATS); err != nil {
		slog.Error("Failed to send STATS signal to keepalived", "error", err)

		return err
	}

	if err := k.signal(k.SIGDATA); err != nil {
		slog.Error("Failed to send DATA signal to keepalived", "error", err)

		return err
	}

	return nil
}

func (k *KeepalivedHostCollectorHost) initSignals() {
	if k.useJSON {
		k.SIGJSON = k.sigNum("JSON")
	}

	k.SIGDATA = k.sigNum("DATA")
	k.SIGSTATS = k.sigNum("STATS")
}

// GetKeepalivedVersion returns Keepalived version.
func (k *KeepalivedHostCollectorHost) getKeepalivedVersion() (*version.Version, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command("bash", "-c", "keepalived -v")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Error("Error getting keepalived version",
			"stderr", stderr.String(),
			"stdout", stdout.String(),
			"error", err,
		)

		return nil, errors.New("error getting keepalived version")
	}

	return utils.ParseVersion(stderr.String())
}

func (k *KeepalivedHostCollectorHost) HasJSONSignalSupport() (bool, error) {
	cmd := exec.Command("keepalived", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Failed to run keepalived --version command",
			"output", string(output),
			"error", err,
		)

		return false, err
	}

	if strings.Contains(string(output), "--enable-json") {
		return true, nil
	}

	slog.Error("Keepalived does not support JSON signal",
		"version", k.version,
		"output", string(output),
	)

	return false, nil
}

// Signal sends signal to Keepalived process.
func (k *KeepalivedHostCollectorHost) signal(signal os.Signal) error {
	data, err := os.ReadFile(k.pidPath)
	if err != nil {
		slog.Error("Failed to read Keepalived PID file",
			"path", k.pidPath,
			"error", err,
		)

		return err
	}

	pid, err := strconv.Atoi(strings.TrimSuffix(string(data), "\n"))
	if err != nil {
		slog.Error("Failed to parse Keepalived PID",
			"path", k.pidPath,
			"pid", string(data),
			"error", err,
		)

		return err
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		slog.Error("Failed to find Keepalived process",
			"path", k.pidPath,
			"pid", pid,
			"error", err,
		)

		return err
	}

	err = proc.Signal(signal)
	if err != nil {
		slog.Error("Failed to send signal to Keepalived process",
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
func (k *KeepalivedHostCollectorHost) sigNum(sigString string) syscall.Signal {
	if !utils.HasSigNumSupport(k.version) {
		return utils.GetDefaultSignal(sigString)
	}

	var stdout, stderr bytes.Buffer

	sigNumCommand := "keepalived --signum=" + sigString
	cmd := exec.Command("bash", "-c", sigNumCommand)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		slog.Error("Error executing command to get signal number",
			"signal", sigString,
			"command", sigNumCommand,
			"stdout", stdout.String(),
			"stderr", stderr.String(),
			"error", err,
		)
		os.Exit(1)
	}

	return syscall.Signal(parseSigNum(stdout, sigString))
}

func (k *KeepalivedHostCollectorHost) JSONVrrps() ([]collector.VRRP, error) {
	const fileName = "/tmp/keepalived.json"

	f, err := os.Open(fileName)
	if err != nil {
		slog.Error("Failed to open JSON VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error("Failed to close file",
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
		slog.Error("Failed to open Stats VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error("Failed to close Stats VRRP file",
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
		slog.Error("Failed to open Data VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error("Failed to close Data VRRP file",
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
		slog.Error("Failed to open Script VRRP file",
			"fileName", fileName,
			"error", err,
		)

		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error("Failed to close Script VRRP file",
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
