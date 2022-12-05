//go:build no_traces

package traces

type Config struct {
}

func Parse(c *Config) error {
	return nil
}
