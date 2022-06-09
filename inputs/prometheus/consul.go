package prometheus

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"strings"
	"text/template"

	"flashcat.cloud/categraf/config"
	"github.com/hashicorp/consul/api"
)

type ConsulConfig struct {
	// Address of the Consul agent. The address must contain a hostname or an IP address
	// and optionally a port (format: "host:port").
	Enabled bool           `toml:"enabled"`
	Agent   string         `toml:"agent"`
	Queries []*ConsulQuery `toml:"query"`
	Catalog *api.Catalog   `toml:"-"`
}

// One Consul service discovery query
type ConsulQuery struct {
	// A name of the searched services (not ID)
	ServiceName string `toml:"name"`

	// A tag of the searched services
	ServiceTag string `toml:"tag"`

	// A DC of the searched services
	ServiceDc string `toml:"dc"`

	// A template URL of the Prometheus gathering interface. The hostname part
	// of the URL will be replaced by discovered address and port.
	ServiceURL string `toml:"url"`

	// Extra tags to add to metrics found in Consul
	ServiceExtraTags map[string]string `toml:"tags"`

	serviceURLTemplate       *template.Template
	serviceExtraTagsTemplate map[string]*template.Template
}

func (ins *Instance) InitConsulClient() error {
	consulAPIConfig := api.DefaultConfig()
	if ins.ConsulConfig.Agent != "" {
		consulAPIConfig.Address = ins.ConsulConfig.Agent
	}

	// Parse the template for metrics URL, drop queries with template parse errors
	for i := range ins.ConsulConfig.Queries {
		serviceURLTemplate, err := template.New("URL").Parse(ins.ConsulConfig.Queries[i].ServiceURL)
		if err != nil {
			return fmt.Errorf("failed to parse the Consul query URL template (%s): %s", ins.ConsulConfig.Queries[i].ServiceURL, err)
		}
		ins.ConsulConfig.Queries[i].serviceURLTemplate = serviceURLTemplate

		// Allow to use join function in tags
		templateFunctions := template.FuncMap{"join": strings.Join}
		// Parse the tag value templates
		ins.ConsulConfig.Queries[i].serviceExtraTagsTemplate = make(map[string]*template.Template)
		for tagName, tagTemplateString := range ins.ConsulConfig.Queries[i].ServiceExtraTags {
			tagTemplate, err := template.New(tagName).Funcs(templateFunctions).Parse(tagTemplateString)
			if err != nil {
				return fmt.Errorf("failed to parse the Consul query Extra Tag template (%s): %s", tagTemplateString, err)
			}
			ins.ConsulConfig.Queries[i].serviceExtraTagsTemplate[tagName] = tagTemplate
		}
	}

	// Prevent memory leak by erasing truncated values
	// for j := i; j < len(ins.ConsulConfig.Queries); j++ {
	// 	ins.ConsulConfig.Queries[j] = nil
	// }
	// ins.ConsulConfig.Queries = ins.ConsulConfig.Queries[:i]

	consul, err := api.NewClient(consulAPIConfig)
	if err != nil {
		return fmt.Errorf("failed to connect the Consul agent(%s): %v", consulAPIConfig.Address, err)
	}

	ins.ConsulConfig.Catalog = consul.Catalog()

	return nil
}

func (ins *Instance) UrlsFromConsul() ([]ScrapeUrl, error) {
	if !ins.ConsulConfig.Enabled {
		return []ScrapeUrl{}, nil
	}

	if config.Config.DebugMode {
		log.Println("D! get urls from consul:", ins.ConsulConfig.Agent)
	}

	urlset := map[string]struct{}{}
	var returls []ScrapeUrl

	for _, q := range ins.ConsulConfig.Queries {
		queryOptions := api.QueryOptions{}
		if q.ServiceDc != "" {
			queryOptions.Datacenter = q.ServiceDc
		}

		// Request services from Consul
		consulServices, _, err := ins.ConsulConfig.Catalog.Service(q.ServiceName, q.ServiceTag, &queryOptions)
		if err != nil {
			return nil, err
		}

		if len(consulServices) == 0 {
			if config.Config.DebugMode {
				log.Println("D! query consul did not find any instances, service:", q.ServiceName, " tag:", q.ServiceTag)
			}
			continue
		}

		if config.Config.DebugMode {
			log.Println("D! query consul found", len(consulServices), "instances, service:", q.ServiceName, " tag:", q.ServiceTag)
		}

		for _, consulService := range consulServices {
			su, err := ins.getConsulServiceURL(q, consulService)
			if err != nil {
				return nil, fmt.Errorf("unable to get scrape URLs from Consul for Service (%s, %s): %s", q.ServiceName, q.ServiceTag, err)
			}

			if _, has := urlset[su.URL.String()]; has {
				continue
			}

			urlset[su.URL.String()] = struct{}{}
			returls = append(returls, *su)
		}
	}

	return returls, nil
}

func (ins *Instance) getConsulServiceURL(q *ConsulQuery, s *api.CatalogService) (*ScrapeUrl, error) {
	var buffer bytes.Buffer
	buffer.Reset()
	err := q.serviceURLTemplate.Execute(&buffer, s)
	if err != nil {
		return nil, err
	}
	serviceURL, err := url.Parse(buffer.String())
	if err != nil {
		return nil, err
	}

	extraTags := make(map[string]string)
	for tagName, tagTemplate := range q.serviceExtraTagsTemplate {
		buffer.Reset()
		err = tagTemplate.Execute(&buffer, s)
		if err != nil {
			return nil, err
		}
		extraTags[tagName] = buffer.String()
	}

	if config.Config.DebugMode {
		log.Println("D! found consul service:", serviceURL.String())
	}

	return &ScrapeUrl{
		URL:  serviceURL,
		Tags: extraTags,
	}, nil
}

type ScrapeUrl struct {
	URL  *url.URL
	Tags map[string]string
}
