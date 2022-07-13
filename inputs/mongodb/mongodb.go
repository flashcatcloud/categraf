package mongodb

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/mongodb/exporter"
	"flashcat.cloud/categraf/types"
	"github.com/sirupsen/logrus"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "mongodb"

type MongoDB struct {
	config.Interval
	counter   uint64
	waitgrp   sync.WaitGroup
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MongoDB{}
	})
}

func (r *MongoDB) Prefix() string {
	return ""
}

func (r *MongoDB) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
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

func (r *MongoDB) Gather(slist *list.SafeList) {
	atomic.AddUint64(&r.counter, 1)

	for i := range r.Instances {
		ins := r.Instances[i]

		if len(ins.MongodbURI) == 0 {
			continue
		}

		r.waitgrp.Add(1)
		go func(slist *list.SafeList, ins *Instance) {
			defer r.waitgrp.Done()

			if ins.IntervalTimes > 0 {
				counter := atomic.LoadUint64(&r.counter)
				if counter%uint64(ins.IntervalTimes) != 0 {
					return
				}
			}

			ins.gatherOnce(slist)
		}(slist, ins)
	}

	r.waitgrp.Wait()
}

type Instance struct {
	Labels        map[string]string `toml:"labels"`
	IntervalTimes int64             `toml:"interval_times"`
	LogLevel      string            `toml:"log_level"`

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
		return nil
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

func (ins *Instance) gatherOnce(slist *list.SafeList) {
	err := inputs.Collect(ins.e, slist, ins.Labels)
	if err != nil {
		log.Println("E! failed to collect metrics:", err)
	}
}