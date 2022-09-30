package integration

import (
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
