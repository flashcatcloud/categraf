package config

import "flashcat.cloud/categraf/pkg/tls"

type HTTPProviderConfig struct {
	tls.ClientConfig

	RemoteUrl      string   `toml:"remote_url"`
	Headers        []string `toml:"headers"`
	AuthUsername   string   `toml:"basic_auth_user"`
	AuthPassword   string   `toml:"basic_auth_pass"`
	Timeout        int      `toml:"timeout"`
	ReloadInterval int      `toml:"reload_interval"`
}
