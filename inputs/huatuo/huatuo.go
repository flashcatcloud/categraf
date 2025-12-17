package huatuo

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/parser"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/types"
)

const inputName = "huatuo"

type Huatuo struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Huatuo{}
	})
}

func (h *Huatuo) Clone() inputs.Input {
	return &Huatuo{}
}

func (h *Huatuo) Name() string {
	return inputName
}

func (h *Huatuo) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(h.Instances))
	for i := 0; i < len(h.Instances); i++ {
		ret[i] = h.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	// Mode 1: Local Managed
	InstallPath      string                 `toml:"install_path"`
	HuatuoTarball    string                 `toml:"huatuo_tarball"`
	ConfigOverwrites map[string]interface{} `toml:"config_overwrites"`

	// Mode 2: Remote/Unmanaged
	URL     string          `toml:"url"`
	Timeout config.Duration `toml:"timeout"`

	// Internal state
	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	realURL    string // Actual URL to scrape (configured URL or discovered local port)
	parser     parser.Parser
	lock       sync.Mutex
}

func setNestedMap(m map[string]interface{}, path string, val interface{}) {
	parts := strings.Split(path, ".")
	current := m
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		next, ok := current[part]
		if !ok || next == nil {
			nextMap := make(map[string]interface{})
			current[part] = nextMap
			current = nextMap
		} else if nextMap, ok := next.(map[string]interface{}); ok {
			current = nextMap
		} else {
			nextMap := make(map[string]interface{})
			current[part] = nextMap
			current = nextMap
		}
	}
	current[parts[len(parts)-1]] = val
}

func (ins *Instance) Init() error {
	if ins.InstallPath == "" && ins.URL == "" {
		return types.ErrInstancesEmpty
	}

	ins.parser = prometheus.EmptyParser()

	if ins.InstallPath != "" {
		// Local Managed Mode
		if err := ins.ensureInstalled(); err != nil {
			return fmt.Errorf("failed to install huatuo: %w", err)
		}
		if err := ins.setupConfig(); err != nil {
			return fmt.Errorf("failed to setup config: %w", err)
		}

		// Start process asynchronously
		ctx, cancel := context.WithCancel(context.Background())
		ins.cancelFunc = cancel
		go ins.manageProcess(ctx)
		ins.wg.Add(1)
	} else {
		// Remote Mode
		ins.realURL = ins.URL
	}
	return nil
}

func (ins *Instance) ensureInstalled() error {
	binPath := filepath.Join(ins.InstallPath, "bin", "huatuo-bamai")
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}

	// Not found, try installing from tarball
	if ins.HuatuoTarball == "" {
		return fmt.Errorf("huatuo binary not found at %s and no tarball specified", binPath)
	}

	f, err := os.Open(ins.HuatuoTarball)
	if err != nil {
		return fmt.Errorf("open tarball: %w", err)
	}
	defer f.Close()

	if err := os.MkdirAll(ins.InstallPath, 0755); err != nil {
		return err
	}

	if err := unpackTarGz(f, ins.InstallPath); err != nil {
		return err
	}

	// Check again if binary exists at expected location
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}

	// Handle case where tarball has a top-level directory (e.g. huatuo-v2.1.0-linux-amd64/)
	entries, err := os.ReadDir(ins.InstallPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if binary is inside this subdirectory
			subBinPath := filepath.Join(ins.InstallPath, entry.Name(), "bin", "huatuo-bamai")
			if _, err := os.Stat(subBinPath); err == nil {
				// Found it! Move everything up one level.
				log.Printf("I! Detected subdirectory %s, moving files up to %s", entry.Name(), ins.InstallPath)
				srcDir := filepath.Join(ins.InstallPath, entry.Name())

				// Move content
				subEntries, err := os.ReadDir(srcDir)
				if err != nil {
					return err
				}
				for _, sub := range subEntries {
					oldPath := filepath.Join(srcDir, sub.Name())
					newPath := filepath.Join(ins.InstallPath, sub.Name())
					// Rename logic: remove dest if exists to ensure overwrite, then rename
					if err := os.RemoveAll(newPath); err != nil {
						return fmt.Errorf("failed to remove existing file %s: %w", newPath, err)
					}
					if err := os.Rename(oldPath, newPath); err != nil {
						return fmt.Errorf("failed to move %s to %s: %w", oldPath, newPath, err)
					}
				}
				// Remove empty subdir
				if err := os.Remove(srcDir); err != nil {
					log.Printf("W! failed to remove empty dir %s: %v", srcDir, err)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("binary not found after unpacking")
}

func unpackTarGz(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Zip Slip protection
		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				closeErr := f.Close()
				if closeErr != nil {
					return fmt.Errorf("write error: %v; close error: %v", err, closeErr)
				}
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ins *Instance) setupConfig() error {
	// Find config file
	targetFile, err := ins.getConfigPath()
	if err != nil {
		// If overwrites defined, missing config is error
		if len(ins.ConfigOverwrites) > 0 {
			return err
		}
		// Otherwise, if just using default/existing, we can proceed with default port
		// But manageProcess needs config... assume default location for process?
		// Stick to existing logic: if config overwrites empty, we didn't error on applyConfig,
		// but detectPort used fallback.
		// However, verify manageProcess logic.
	}

	// If file found, read and process
	var data map[string]interface{}
	if targetFile != "" {
		content, err := os.ReadFile(targetFile)
		if err != nil {
			return err
		}

		if err := toml.Unmarshal(content, &data); err != nil {
			return err
		}

		// Apply overwrites if needed
		if len(ins.ConfigOverwrites) > 0 {
			for k, v := range ins.ConfigOverwrites {
				setNestedMap(data, k, v)
			}
			newContent, err := toml.Marshal(data)
			if err != nil {
				return err
			}
			if err := os.WriteFile(targetFile, newContent, 0644); err != nil {
				return err
			}
		}
	}

	// Detect port from Map (if available)
	addr := ":19704" // default
	if data != nil {
		if apiServer, ok := data["APIServer"].(map[string]interface{}); ok {
			if val, ok := apiServer["TCPAddr"].(string); ok {
				addr = val
			}
		}
	}

	// Normalize addr
	if addr == "" {
		addr = ":19704"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	ins.realURL = fmt.Sprintf("http://%s/metrics", addr)

	return nil
}

func (ins *Instance) getConfigPath() (string, error) {
	candidates := []string{
		filepath.Join(ins.InstallPath, "huatuo-bamai.conf"),
		filepath.Join(ins.InstallPath, "conf", "huatuo-bamai.conf"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("config file not found in install path")
}

func (ins *Instance) manageProcess(ctx context.Context) {
	defer ins.wg.Done()

	// Assuming bin is at bin/huatuo-bamai
	binPath := filepath.Join(ins.InstallPath, "bin", "huatuo-bamai")
	// Config path
	confPath, err := ins.getConfigPath()
	if err != nil {
		// Fallback or error?
		// If we are here, setupConfig probably succeeded with default or found it.
		// If not found now, maybe deleted?
		// Try fallback construction for command arg
		confPath = filepath.Join(ins.InstallPath, "huatuo-bamai.conf")
	}

	// Determine region
	var region = "default"
	if val, ok := ins.ConfigOverwrites["Region"]; ok {
		if s, ok := val.(string); ok {
			region = s
		}
	}
	// TODO: if not in overwrite, maybe read from file?
	// CLI --region is required by huatuo main.go.
	// If not supplied, it panics? No, main.go checks "required: true".
	// So we MUST provide it. Safe to default to "default" or "unknown" if not set.

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Start Process
			// Huatuo requires root permissions for eBPF and PID file locking
			cmd := exec.Command(binPath, "--config", filepath.Base(confPath), "--region", region)
			cmd.Dir = ins.InstallPath // Set workdir to install path so it finds config if relative

			// Redirect stdout/stderr to log?
			if ins.DebugMod {
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
			}

			if err := cmd.Start(); err != nil {
				log.Printf("E! failed to start huatuo: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
					continue
				}
			}

			ins.lock.Lock()
			ins.cmd = cmd
			ins.lock.Unlock()

			// Wait for exit or context cancel
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case <-ctx.Done():
				// Context cancelled, kill process
				if cmd.Process != nil {
					_ = cmd.Process.Signal(os.Interrupt)
					// Give slight grace period?
					time.Sleep(1 * time.Second)
					_ = cmd.Process.Kill()
				}
				return
			case err := <-done:
				log.Printf("I! huatuo process exited: %v", err)
			}

			// Allow quick restart unless context cancelled
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// continue loop
			}
		}
	}
}

func (ins *Instance) Drop() {
	if ins.cancelFunc != nil {
		ins.cancelFunc()
	}
	// Lock for cmd access
	ins.lock.Lock()
	if ins.cmd != nil && ins.cmd.Process != nil {
		_ = ins.cmd.Process.Signal(os.Interrupt)
		// Give it a moment to shut down gracefully
		time.Sleep(1 * time.Second)
		_ = ins.cmd.Process.Kill()
	}
	ins.lock.Unlock()
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.realURL == "" {
		return
	}

	err := ins.scrape(slist)
	if err != nil {
		log.Printf("E! failed to scrape huatuo: %v", err)
	}
}

func (ins *Instance) scrape(slist *types.SampleList) error {
	var timeout = 5 * time.Second
	if ins.Timeout > 0 {
		timeout = time.Duration(ins.Timeout)
	}

	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(ins.realURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return ins.parser.Parse(content, slist)
}
