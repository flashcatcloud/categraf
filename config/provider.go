package config

import "flashcat.cloud/categraf/pkg/cfg"

type HttpRemoteProviderConfig struct {
	RemoteUrl    string            `toml:"remote_url"`
	Headers      map[string]string `toml:"headers"`
	AuthUsername string            `toml:"basic_auth_user"`
	AuthPassword string            `toml:"basic_auth_pass"`

	ConfigFormat   cfg.ConfigFormat `toml:"config_format"`
	ReloadInterval int              `toml:"reload_interval"`
}
