package config

import "flashcat.cloud/categraf/pkg/cfg"

type HttpRemoteProviderConfig struct {
	RemoteUrl string            `toml:"remote_url"`
	Tags      map[string]string `toml:"tags"`

	ConfigFormat   cfg.ConfigFormat `toml:"config_format"`
	ReloadInterval int              `toml:"reload_interval"`
}
