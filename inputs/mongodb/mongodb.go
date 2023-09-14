package mongodb

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "mongodb"

type MongoDB struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MongoDB{}
	})
}

func (r *MongoDB) Clone() inputs.Input {
	return &MongoDB{}
}

func (r *MongoDB) Name() string {
	return inputName
}

func (r *MongoDB) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

func (r *MongoDB) Drop() {
	for _, i := range r.Instances {
		if i == nil {
			continue
		}
		for _, server := range i.clients {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := server.client.Disconnect(ctx); err != nil {
				log.Printf("E! Disconnecting from %q failed: %v", server.hostname, err)
			}
			cancel()
		}
	}
}

type Ssl struct {
	Enabled bool     `toml:"ssl_enabled" deprecated:"1.3.0;use 'tls_*' options instead"`
	CaCerts []string `toml:"cacerts" deprecated:"1.3.0;use 'tls_ca' instead"`
}

type Instance struct {
	config.InstanceConfig

	// telegraf
	Servers                     []string
	Ssl                         Ssl
	GatherClusterStatus         bool `toml:"enable_cluster_status,omitempty"`
	EnableReplicasetStatus      bool `toml:"enable_replicaset_status,omitempty"`
	EnableDBStats               bool `toml:"enable_db_stats,omitempty"`
	EnableCollStats             bool `toml:"enable_coll_stats,omitempty"`
	EnableTopMetrics            bool `toml:"enable_top_metrics,omitempty"`
	DisconnectedServersBehavior string
	ColStatsDbs                 []string `toml:"coll_stats_namespaces,omitempty"`
	CollectAll                  bool     `toml:"collect_all,omitempty"`

	clients []*Server
	//
	tls.ClientConfig

	// Address (host:port) of MongoDB server.
	MongodbURI                    string   `toml:"mongodb_uri,omitempty"` // deprecated
	Username                      string   `toml:"username,omitempty"`
	Password                      string   `toml:"password,omitempty"`
	IndexStatsCollections         []string `toml:"index_stats_collections,omitempty"` // 保留 ，就加过滤逻辑
	CollStatsLimit                int      `toml:"coll_stats_limit,omitempty"`
	CompatibleMode                bool     `toml:"compatible_mode,omitempty"`
	DirectConnect                 bool     `toml:"direct_connect,omitempty"`
	DiscoveringMode               bool     `toml:"discovering_mode,omitempty"`
	EnableDiagnosticData          bool     `toml:"enable_diagnostic_data,omitempty"`
	EnableIndexStats              bool     `toml:"enable_index_stats,omitempty"`
	EnableOverrideDescendingIndex bool     `toml:"enable_override_descending_index,omitempty"`

	Queries []QueryConfig
}

type QueryConfig struct {
	Mesurement    string          `toml:"mesurement"`
	LabelFields   []string        `toml:"label_fields"`
	MetricFields  []string        `toml:"metric_fields"`
	FieldToAppend string          `toml:"field_to_append"`
	Timeout       config.Duration `toml:"timeout"`
	Request       string          `toml:"request"`
}

func (ins *Instance) Init() error {
	if len(ins.MongodbURI) != 0 {
		log.Printf("W! ins.mongodb_uri is deprecated, use ins.servers instead")
		ins.Servers = append(ins.Servers, ins.MongodbURI)
	}
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}
	if ins.UseTLS {
		_, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return err
		}
	}

	for _, connURL := range ins.Servers {
		if err := ins.setupConnection(connURL); err != nil {
			return err
		}
	}

	if ins.CollectAll {
		ins.EnableTopMetrics = true
		ins.EnableDBStats = true
		ins.EnableCollStats = true
		ins.EnableReplicasetStatus = true
		ins.GatherClusterStatus = true
	}

	return nil
}

func (ins *Instance) setupConnection(connURL string) error {
	if !strings.HasPrefix(connURL, "mongodb://") && !strings.HasPrefix(connURL, "mongodb+srv://") {
		// Preserve backwards compatibility for hostnames without a
		// scheme, broken in go 1.8. Remove in Telegraf 2.0
		connURL = "mongodb://" + connURL
		log.Printf("Using %q as connection URL; please update your configuration to use an URL", connURL)
	}

	u, err := url.Parse(connURL)
	if err != nil {
		return fmt.Errorf("unable to parse connection URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Client().ApplyURI(connURL)
	if ins.UseTLS {
		opts.TLSConfig, _ = ins.ClientConfig.TLSConfig()
	}
	if opts.ReadPreference == nil {
		opts.ReadPreference = readpref.Nearest()
	}

	opts.SetDirect(ins.DirectConnect)
	opts.SetAppName("categraf")
	if len(ins.Username) > 0 || len(ins.Password) > 0 {
		opts.SetAuth(options.Credential{
			Username: ins.Username,
			Password: ins.Password,
		})
	}

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return fmt.Errorf("unable to connect to MongoDB: %w", err)
	}

	err = client.Ping(ctx, opts.ReadPreference)
	if err != nil {
		if ins.DisconnectedServersBehavior == "error" {
			return fmt.Errorf("unable to ping MongoDB: %w", err)
		}

		log.Printf("E! Unable to ping MongoDB: %s", err)
	}

	server := &Server{
		client:   client,
		hostname: u.Host,
	}
	ins.clients = append(ins.clients, server)
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	tags := ins.GetLabels()
	var wg sync.WaitGroup
	for _, client := range ins.clients {
		wg.Add(1)
		go func(srv *Server) {
			defer wg.Done()
			tags["instance"] = srv.hostname
			if err := srv.ping(); err != nil {
				log.Printf("E! Failed to ping server: %s", err)
				slist.PushSample(inputName, "up", 0, tags)
				return
			}
			slist.PushSample(inputName, "up", 1, tags)

			err := srv.gatherData(slist, ins.GatherClusterStatus,
				ins.EnableReplicasetStatus, ins.EnableDBStats, ins.EnableCollStats,
				ins.EnableTopMetrics, ins.ColStatsDbs, ins.GetLabels())
			if err != nil {
				log.Printf("E! Failed to gather data: %s", err)
			}
		}(client)
	}

	wg.Wait()
}
