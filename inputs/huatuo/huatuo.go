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
	URL string `toml:"url"`

	// Internal state
	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	realURL    string // Actual URL to scrape (configured URL or discovered local port)
	parser     parser.Parser
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
		if err := ins.applyConfig(); err != nil {
			return fmt.Errorf("failed to apply config: %w", err)
		}
		if err := ins.detectPort(); err != nil {
			return fmt.Errorf("failed to detect port: %w", err)
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

		target := filepath.Join(dest, header.Name)
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
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func (ins *Instance) applyConfig() error {
	if len(ins.ConfigOverwrites) == 0 {
		return nil
	}

	// Assuming conf at root of unpacked dir
	candidates := []string{
		filepath.Join(ins.InstallPath, "huatuo-bamai.conf"),
		filepath.Join(ins.InstallPath, "conf", "huatuo-bamai.conf"),
	}
	var targetFile string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			targetFile = c
			break
		}
	}
	if targetFile == "" {
		// If both fail, we can't overwrite. But maybe first install?
		return fmt.Errorf("config file not found in install path")
	}

	content, err := os.ReadFile(targetFile)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := toml.Unmarshal(content, &data); err != nil {
		return err
	}

	for k, v := range ins.ConfigOverwrites {
		setNestedMap(data, k, v)
	}

	newContent, err := toml.Marshal(data)
	if err != nil {
		return err
	}

	return os.WriteFile(targetFile, newContent, 0644)
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

func (ins *Instance) detectPort() error {
	// Re-read config to find APIServer.TCPAddr
	candidates := []string{
		filepath.Join(ins.InstallPath, "huatuo-bamai.conf"),
		filepath.Join(ins.InstallPath, "conf", "huatuo-bamai.conf"),
	}
	var targetFile string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			targetFile = c
			break
		}
	}
	if targetFile == "" {
		ins.realURL = "http://127.0.0.1:19704/metrics" // fallback
		return nil
	}

	content, err := os.ReadFile(targetFile)
	if err != nil {
		return err
	}

	var data struct {
		APIServer struct {
			TCPAddr string
		}
	}
	// Loose decoding just for this field
	_ = toml.Unmarshal(content, &data)

	addr := data.APIServer.TCPAddr
	if addr == "" {
		addr = ":19704" // Default
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	ins.realURL = fmt.Sprintf("http://%s/metrics", addr)
	return nil
}

func (ins *Instance) manageProcess(ctx context.Context) {
	defer ins.wg.Done()

	// Assuming bin is at bin/huatuo-bamai
	binPath := filepath.Join(ins.InstallPath, "bin", "huatuo-bamai")
	// Config path
	confPath := filepath.Join(ins.InstallPath, "huatuo-bamai.conf") // try root first
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		confPath = filepath.Join(ins.InstallPath, "conf", "huatuo-bamai.conf")
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
				time.Sleep(10 * time.Second)
				continue
			}
			ins.cmd = cmd

			// Wait for exit
			err := cmd.Wait()
			log.Printf("I! huatuo process exited: %v", err)

			// Allow quick restart unless context cancelled
			time.Sleep(5 * time.Second)
		}
	}
}

func (ins *Instance) Drop() {
	if ins.cancelFunc != nil {
		ins.cancelFunc()
	}
	if ins.cmd != nil && ins.cmd.Process != nil {
		_ = ins.cmd.Process.Signal(os.Interrupt)
		// Give it a moment to shut down gracefully
		time.Sleep(1 * time.Second)
		_ = ins.cmd.Process.Kill()
	}
	// Not waiting for wg here to avoid blocking Drop?
	// Or should we? Categraf drop implementation usually simple.
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
	client := http.Client{
		Timeout: 5 * time.Second,
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
