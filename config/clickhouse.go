package config

type MetricsHouse struct {
	Enable          bool     `toml:"enable"`
	Debug           bool     `toml:"debug"`
	Endpoints       []string `toml:"endpoints"`
	Database        string   `toml:"database"`
	Table           string   `toml:"table"`
	Username        string   `toml:"username"`
	Password        string   `toml:"password"`
	DialTimeout     Duration `toml:"dial_timeout"`
	MaxOpenConns    int      `toml:"max_open_conns"`
	MaxIdleConns    int      `toml:"max_idle_conns"`
	ConnMaxLifetime Duration `toml:"conn_max_lifetime"`
	QueueSize       int      `toml:"queue_size"`
	BatchSize       int      `toml:"batch_size"`
	IdleDuration    Duration `toml:"idle_duration"`
}
