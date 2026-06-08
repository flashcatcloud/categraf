package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitConfigFailureKeepsCurrentConfig(t *testing.T) {
	oldConfig := Config
	oldHostInfo := HostInfo
	defer func() {
		Config = oldConfig
		HostInfo = oldHostInfo
	}()

	HostInfo = &HostInfoCache{name: "test-host", ip: "127.0.0.1", sn: "test-sn"}

	dir := t.TempDir()
	writeConfig(t, dir, `
[global]
hostname = "initial"
reload_on_change = true

[writer_opt]
batch = 1000
chan_size = 1000

[[writers]]
url = "http://127.0.0.1:17000/prometheus/v1/write"
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100
`)

	if err := InitConfig(dir, 0, false, false, 0, ""); err != nil {
		t.Fatalf("InitConfig valid config error = %v", err)
	}
	if got := Config.Global.Hostname; got != "initial" {
		t.Fatalf("hostname = %q, want initial", got)
	}
	if !Config.Global.ReloadOnChange {
		t.Fatal("reload_on_change was not loaded")
	}

	writeConfig(t, dir, "[global\nhostname = \"broken\"\n")

	if err := InitConfig(dir, 0, false, false, 0, ""); err == nil {
		t.Fatal("InitConfig invalid config error = nil, want error")
	}
	if got := Config.Global.Hostname; got != "initial" {
		t.Fatalf("hostname after failed reload = %q, want initial", got)
	}
	if !Config.Global.ReloadOnChange {
		t.Fatal("reload_on_change changed after failed reload")
	}
}

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
}
