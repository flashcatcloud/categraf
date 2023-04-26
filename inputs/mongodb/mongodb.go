package mongodb

import (
	"fmt"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/mongodb/exporter"
	"flashcat.cloud/categraf/types"
	"github.com/sirupsen/logrus"
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

		if i.e != nil {
			i.e.Close()
		}
	}
}

type Instance struct {
	config.InstanceConfig

	LogLevel string `toml:"log_level"`

	// Address (host:port) of MongoDB server.
	MongodbURI                    string   `toml:"mongodb_uri,omitempty"`
	Username                      string   `toml:"username,omitempty"`
	Password                      string   `toml:"password,omitempty"`
	CollStatsNamespaces           []string `toml:"coll_stats_namespaces,omitempty"`
	IndexStatsCollections         []string `toml:"index_stats_collections,omitempty"`
	CollStatsLimit                int      `toml:"coll_stats_limit,omitempty"`
	CompatibleMode                bool     `toml:"compatible_mode,omitempty"`
	DirectConnect                 bool     `toml:"direct_connect,omitempty"`
	DiscoveringMode               bool     `toml:"discovering_mode,omitempty"`
	CollectAll                    bool     `toml:"collect_all,omitempty"`
	EnableDBStats                 bool     `toml:"enable_db_stats,omitempty"`
	EnableDiagnosticData          bool     `toml:"enable_diagnostic_data,omitempty"`
	EnableReplicasetStatus        bool     `toml:"enable_replicaset_status,omitempty"`
	EnableTopMetrics              bool     `toml:"enable_top_metrics,omitempty"`
	EnableIndexStats              bool     `toml:"enable_index_stats,omitempty"`
	EnableCollStats               bool     `toml:"enable_coll_stats,omitempty"`
	EnableOverrideDescendingIndex bool     `toml:"enable_override_descending_index,omitempty"`

	e *exporter.Exporter `toml:"-"`
}

func (ins *Instance) Init() error {
	if len(ins.MongodbURI) == 0 {
		return types.ErrInstancesEmpty
	}

	if len(ins.LogLevel) == 0 {
		ins.LogLevel = "info"
	}
	level, err := logrus.ParseLevel(ins.LogLevel)
	if err != nil {
		return err
	}

	l := logrus.New()
	l.SetLevel(level)

	e, err := exporter.New(&exporter.Opts{
		URI:                           string(ins.MongodbURI),
		Username:                      ins.Username,
		Password:                      ins.Password,
		CollStatsNamespaces:           ins.CollStatsNamespaces,
		IndexStatsCollections:         ins.IndexStatsCollections,
		CollStatsLimit:                0,
		CompatibleMode:                ins.CompatibleMode,
		DirectConnect:                 ins.DirectConnect,
		DiscoveringMode:               ins.DiscoveringMode,
		CollectAll:                    ins.CollectAll,
		EnableDBStats:                 ins.EnableDBStats,
		EnableDiagnosticData:          ins.EnableDiagnosticData,
		EnableReplicasetStatus:        ins.EnableReplicasetStatus,
		EnableTopMetrics:              ins.EnableTopMetrics,
		EnableIndexStats:              ins.EnableIndexStats,
		EnableCollStats:               ins.EnableCollStats,
		EnableOverrideDescendingIndex: ins.EnableOverrideDescendingIndex,
		Logger:                        l,
	})
	if err != nil {
		return fmt.Errorf("could not instantiate mongodb lag exporter: %w", err)
	}

	ins.e = e
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	err := inputs.Collect(ins.e, slist)
	if err != nil {
		log.Println("E! failed to collect metrics:", err)
	}
}
