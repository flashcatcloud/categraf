package config

type (
	Prometheus struct {
		Enable           bool   `toml:"enable"`
		LogLevel         string `toml:"log_level"`
		ScrapeConfigFile string `toml:"scrape_config_file"`
		WebAddress       string `toml:"web_address"`
		StoragePath      string `toml:"wal_storage_path"`

		MinBlockDuration  Duration `toml:"min_block_duration"`
		MaxBlockDuration  Duration `toml:"max_block_duration"`
		RetentionDuration Duration `toml:"retention_time"`
		RetentionSize     string   `toml:"retention_size"`
	}
)
