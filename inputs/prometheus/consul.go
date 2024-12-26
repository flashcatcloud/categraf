package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"text/template"
	"time"

	"flashcat.cloud/categraf/config"

	"github.com/hashicorp/consul/api"
)

type ConsulConfig struct {
	// Address of the Consul agent. The address must contain a hostname or an IP address
	// and optionally a port (format: "host:port").
	Enabled       bool            `toml:"enabled"`
	Agent         string          `toml:"agent"`
	QueryInterval config.Duration `toml:"query_interval"`
	Queries       []*ConsulQuery  `toml:"query"`
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

	// Store last error status and change log level depending on repeated occurrence
	lastQueryFailed bool
}

func (ins *Instance) InitConsulClient(ctx context.Context) error {
	consulAPIConfig := api.DefaultConfig()
	if ins.ConsulConfig.Agent != "" {
		consulAPIConfig.Address = ins.ConsulConfig.Agent
	}
	consul, err := api.NewClient(consulAPIConfig)
	if err != nil {
		return fmt.Errorf("cannot connect to the Consul agent: %w", err)
	}

	i := 0
	// Parse the template for metrics URL, drop queries with template parse errors
	for _, q := range ins.ConsulConfig.Queries {
		serviceURLTemplate, err := template.New("URL").Parse(ins.ConsulConfig.Queries[i].ServiceURL)
		if err != nil {
			return fmt.Errorf("failed to parse the Consul query URL template (%s): %s", ins.ConsulConfig.Queries[i].ServiceURL, err)
		}
		q.serviceURLTemplate = serviceURLTemplate

		// Allow to use join function in tags
		templateFunctions := template.FuncMap{"join": strings.Join}
		// Parse the tag value templates
		q.serviceExtraTagsTemplate = make(map[string]*template.Template)
		for tagName, tagTemplateString := range ins.ConsulConfig.Queries[i].ServiceExtraTags {
			tagTemplate, err := template.New(tagName).Funcs(templateFunctions).Parse(tagTemplateString)
			if err != nil {
				log.Printf("failed to parse the Consul query Extra Tag template (%s): %s", tagTemplateString, err)
				continue
			}
			q.serviceExtraTagsTemplate[tagName] = tagTemplate
		}
		ins.ConsulConfig.Queries[i] = q
		i++
	}

	// Prevent memory leak by erasing truncated values
	for j := i; j < len(ins.ConsulConfig.Queries); j++ {
		ins.ConsulConfig.Queries[j] = nil
	}
	ins.ConsulConfig.Queries = ins.ConsulConfig.Queries[:i]

	catalog := consul.Catalog()

	ins.wg.Add(1)
	go func() {
		// Store last error status and change log level depending on repeated occurence
		var refreshFailed = false
		defer ins.wg.Done()
		err := ins.refreshConsulServices(catalog)
		if err != nil {
			refreshFailed = true
			log.Printf("Unable to refreh Consul services: %v", err)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(ins.ConsulConfig.QueryInterval)):
				err := ins.refreshConsulServices(catalog)
				if err != nil {
					message := fmt.Sprintf("Unable to refreh Consul services: %v", err)
					if refreshFailed {
						log.Println("E!", message)
					} else {
						log.Println("W!", message)
					}
					refreshFailed = true
				} else if refreshFailed {
					refreshFailed = false
					log.Println("Successfully refreshed Consul services after previous errors")
				}
			}
		}
	}()

	return nil
}

func (ins *Instance) UrlsFromConsul() ([]*ScrapeUrl, error) {
	ins.lock.Lock()
	defer ins.lock.Unlock()

	urls := make([]*ScrapeUrl, 0, len(ins.consulServices))
	for _, u := range ins.consulServices {
		urls = append(urls, u)
	}

	return urls, nil
}

func (ins *Instance) refreshConsulServices(c *api.Catalog) error {
	consulServiceURLs := make(map[string]*ScrapeUrl)

	if ins.DebugMod {
		log.Println("Refreshing Consul services")
	}

	for _, q := range ins.ConsulConfig.Queries {
		queryOptions := api.QueryOptions{}
		if q.ServiceDc != "" {
			queryOptions.Datacenter = q.ServiceDc
		}

		// Request services from Consul
		consulServices, _, err := c.Service(q.ServiceName, q.ServiceTag, &queryOptions)
		if err != nil {
			return err
		}
		if len(consulServices) == 0 {
			if ins.DebugMod {
				log.Printf("Queried Consul for Service (%s, %s) but did not find any instances\n", q.ServiceName, q.ServiceTag)
			}
			continue
		}
		if ins.DebugMod {
			log.Printf("Queried Consul for Service (%s, %s) and found %d instances\n", q.ServiceName, q.ServiceTag, len(consulServices))
		}

		for _, consulService := range consulServices {
			uaa, err := ins.getConsulServiceURL(q, consulService)
			if err != nil {
				message := fmt.Sprintf("Unable to get scrape URLs from Consul for Service (%s, %s): %s", q.ServiceName, q.ServiceTag, err)
				if q.lastQueryFailed {
					log.Println("E!", message)
				} else {
					log.Println("W!", message)
				}
				q.lastQueryFailed = true
				break
			}
			if q.lastQueryFailed {
				log.Printf("Created scrape URLs from Consul for Service (%s, %s)\n", q.ServiceName, q.ServiceTag)
			}
			q.lastQueryFailed = false
			log.Printf("Adding scrape URL from Consul for Service (%s, %s): %s\n", q.ServiceName, q.ServiceTag, uaa.URL.String())
			consulServiceURLs[uaa.URL.String()] = uaa
		}
	}

	ins.lock.Lock()
	ins.consulServices = consulServiceURLs
	ins.lock.Unlock()

	return nil
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

	if ins.DebugMod {
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
