package integration

import (
	"flashcat.cloud/categraf/logs/util/containers"
	"hash/fnv"
	"strconv"
)

// Data contains YAML code
type Data []byte

// RawMap is the generic type to hold YAML configurations
type RawMap map[interface{}]interface{}

// JSONMap is the generic type to hold JSON configurations
type JSONMap map[string]interface{}

// CreationTime represents the moment when the service was launched compare to the agent start.
type CreationTime int

const (
	// Before indicates the service was launched before the agent start
	Before CreationTime = iota
	// After indicates the service was launched after the agent start
	After
)

type Config struct {
	Name          string   `json:"name"`
	ADIdentifiers []string `json:"ad_identifiers"`

	Source     string `json:"source"`
	LogsConfig []byte `json:"logs"`
	Provider   string `json:"provider"`

	CreationTime CreationTime `json:"-"`             // creation time of service (include in digest: false)
	Entity       string       `json:"-"`             // the entity ID (optional) (include in digest: true)
	TaggerEntity string       `json:"-"`             // the tagger entity ID (optional) (include in digest: false)
	LogsExcluded bool         `json:"logs_excluded"` // whether logs collection is disabled (set by container listeners only) (include in digest: false)

}

// Equal determines whether the passed config is the same
func (c *Config) Equal(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	return c.Digest() == cfg.Digest()
}

// IsLogConfig returns true if config contains a logs config.
func (c *Config) IsLogConfig() bool {
	return c.LogsConfig != nil
}

// HasFilter returns true if metrics or logs collection must be disabled for this config.
// no containers.GlobalFilter case here because we don't create services that are globally excluded in AD
func (c *Config) HasFilter(filter containers.FilterType) bool {
	if filter == containers.LogsFilter {
		return c.LogsExcluded
	}
	return false
}

func (c *Config) IsTemplate() bool {
	return len(c.ADIdentifiers) > 0
}

func (c *Config) Digest() string {
	h := fnv.New64()
	for _, i := range c.ADIdentifiers {
		h.Write([]byte(i)) //nolint:errcheck
	}
	h.Write([]byte(c.LogsConfig)) //nolint:errcheck

	return strconv.FormatUint(h.Sum64(), 16)
}
