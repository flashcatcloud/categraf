//go:build !no_traces

package traces

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/service"
	"gopkg.in/yaml.v3"
)

// Config defines the OpenTelemetry Collector configuration.
//
//	Enable:     enable tracing or not.
//	UnParsed:   loaded as map[string]interface{} from the raw config file.
//	Parsed:     retrieved and validated from the UnParsed contents.
//	Factories:  struct holds in a single type all component factories that can be handled by the Config.
//	            We only create the needed factories as default, if you need more, import and init these by components.go
type Config struct {
	Enable    bool                   `toml:"enable"  yaml:"enable"  json:"enable"`
	UnParsed  map[string]interface{} `toml:",inline" yaml:",inline" json:",inline"`
	Parsed    *config.Config         `toml:"-"       yaml:"-"       json:"parsed"`
	Factories component.Factories    `toml:"-"       yaml:"-"       json:"-"`
}

// Parse parse the UnParsed contents to Parsed
func Parse(c *Config) error {
	if c == nil || len(c.UnParsed) == 0 || !c.Enable {
		log.Println("I! tracing disabled")
		return nil
	}

	ymlCfg, err := yaml.Marshal(c.UnParsed)
	if err != nil {
		return fmt.Errorf("unable to marshal trace config, %v", err)
	}

	provider, err := service.NewConfigProvider(service.ConfigProviderSettings{
		Locations:     []string{"yaml:" + string(ymlCfg)},
		MapProviders:  makeMapProvidersMap(yamlprovider.New()),
		MapConverters: []confmap.Converter{expandconverter.New()},
	})
	if err != nil {
		return fmt.Errorf("unable to new config provider: %v", err)
	}

	c.Factories, err = components()
	if err != nil {
		return fmt.Errorf("unable to init otlp factories: %v", err)
	}

	c.Parsed, err = provider.Get(context.Background(), c.Factories)
	if err != nil {
		return fmt.Errorf("failed to parse trace config: %v", err)
	}

	return nil
}

func makeMapProvidersMap(providers ...confmap.Provider) map[string]confmap.Provider {
	ret := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		ret[provider.Scheme()] = provider
	}
	return ret
}
