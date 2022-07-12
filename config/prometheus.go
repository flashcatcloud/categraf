package config

type (
	Prometheus struct {
		Enable           bool   `toml:"enable"`
		ScrapeConfigFile string `toml:"scrape_config_file"`
		WebAddress       string `toml:"web_address"`
		StoragePath      string `toml:"wal_storage_path"`
		MinBlockDuration string `toml:"wal_min_duration"`
	}
)
