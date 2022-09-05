package config

type HttpRemoteProviderConfig struct {
	RemoteUrl             string            `toml:"remote_url"`
	Headers               map[string]string `toml:"headers"`
	AuthUsername          string            `toml:"basic_auth_user"`
	AuthPassword          string            `toml:"basic_auth_pass"`
	TlsInsecureSkipVerify bool              `toml:"tls_insecure_skip_verify"`
	Timeout               int               `toml:"timeout"`

	ReloadInterval int `toml:"reload_interval"`
}
