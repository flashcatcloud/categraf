// mongodb_exporter
// Copyright (C) 2017 Percona LLC
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Package exporter implements the collectors and metrics handlers.
package exporter

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ prometheus.Collector = (*Exporter)(nil)

// Exporter holds Exporter methods and attributes.
type Exporter struct {
	client                *mongo.Client
	clientMu              sync.Mutex
	logger                *logrus.Logger
	opts                  *Opts
	lock                  *sync.Mutex
	totalCollectionsCount int

	cs []prometheus.Collector
}

// Opts holds new exporter options.
type Opts struct {
	URI      string
	Username string
	Password string

	// Only get stats for the collections matching this list of namespaces.
	// Example: db1.col1,db.col1
	CollStatsNamespaces           []string
	IndexStatsCollections         []string
	CollStatsLimit                int
	CompatibleMode                bool
	DirectConnect                 bool
	DiscoveringMode               bool
	CollectAll                    bool
	EnableDBStats                 bool
	EnableDiagnosticData          bool
	EnableReplicasetStatus        bool
	EnableTopMetrics              bool
	EnableIndexStats              bool
	EnableCollStats               bool
	EnableOverrideDescendingIndex bool

	Logger *logrus.Logger
}

var (
	errCannotHandleType   = fmt.Errorf("don't know how to handle data type")
	errUnexpectedDataType = fmt.Errorf("unexpected data type")
)

const (
	defaultCacheSize = 1000
)

// New connects to the database and returns a new Exporter instance.
func New(opts *Opts) (*Exporter, error) {
	if opts == nil {
		opts = new(Opts)
	}

	if opts.Logger == nil {
		opts.Logger = logrus.New()
	}

	exp := &Exporter{
		logger:                opts.Logger,
		opts:                  opts,
		lock:                  &sync.Mutex{},
		totalCollectionsCount: -1, // Not calculated yet. waiting the db connection.
	}

	ctx := context.Background()
	_, err := exp.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to mongo: %v", err)
	}

	return exp, exp.initCollectors(ctx, exp.client)
}

func (e *Exporter) Close() {
	if e.client != nil {
		e.client.Disconnect(context.Background())
	}
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	wg := new(sync.WaitGroup)

	for idx := range e.cs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			e.cs[i].Collect(ch)
		}(idx)
	}

	wg.Wait()
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	wg := new(sync.WaitGroup)
	wg.Add(len(e.cs))

	for idx := range e.cs {
		go func(i int) {
			defer wg.Done()

			e.cs[i].Describe(ch)
		}(idx)
	}

	wg.Wait()
}

func (e *Exporter) initCollectors(ctx context.Context, client *mongo.Client) error {
	gc := newGeneralCollector(ctx, client, e.opts.Logger)
	e.cs = append(e.cs, gc)

	// Enable collectors like collstats and indexstats depending on the number of collections
	// present in the database.
	limitsOk := false
	if e.opts.CollStatsLimit <= 0 || // Unlimited
		e.getTotalCollectionsCount() <= e.opts.CollStatsLimit {
		limitsOk = true
	}

	if e.opts.CollectAll {
		if len(e.opts.CollStatsNamespaces) == 0 {
			e.opts.DiscoveringMode = true
		}
		e.opts.EnableDiagnosticData = true
		e.opts.EnableDBStats = true
		e.opts.EnableCollStats = true
		e.opts.EnableTopMetrics = true
		e.opts.EnableReplicasetStatus = true
		e.opts.EnableIndexStats = true
	}

	topologyInfo := newTopologyInfo(ctx, client)
	if e.opts.EnableDiagnosticData {
		ddc := newDiagnosticDataCollector(ctx, client, e.opts.Logger,
			e.opts.CompatibleMode, topologyInfo)
		e.cs = append(e.cs, ddc)
	}

	// If we manually set the collection names we want or auto discovery is set.
	if (len(e.opts.CollStatsNamespaces) > 0 || e.opts.DiscoveringMode) && e.opts.EnableCollStats && limitsOk {
		cc := newCollectionStatsCollector(ctx, client, e.opts.Logger,
			e.opts.CompatibleMode, e.opts.DiscoveringMode,
			topologyInfo, e.opts.CollStatsNamespaces)
		e.cs = append(e.cs, cc)
	}

	// If we manually set the collection names we want or auto discovery is set.
	if (len(e.opts.IndexStatsCollections) > 0 || e.opts.DiscoveringMode) && e.opts.EnableIndexStats && limitsOk {
		ic := newIndexStatsCollector(ctx, client, e.opts.Logger,
			e.opts.DiscoveringMode, e.opts.EnableOverrideDescendingIndex,
			topologyInfo, e.opts.IndexStatsCollections)
		e.cs = append(e.cs, ic)
	}

	if e.opts.EnableDBStats && limitsOk {
		cc := newDBStatsCollector(ctx, client, e.opts.Logger,
			e.opts.CompatibleMode, topologyInfo, nil)
		e.cs = append(e.cs, cc)
	}

	nodeType, err := getNodeType(ctx, client)
	if err != nil {
		return fmt.Errorf("cannot get node type to check if this is a mongos : %s", err)
	}

	if e.opts.EnableTopMetrics && nodeType != typeMongos && limitsOk {
		tc := newTopCollector(ctx, client, e.opts.Logger,
			e.opts.CompatibleMode, topologyInfo)
		e.cs = append(e.cs, tc)
	}

	// replSetGetStatus is not supported through mongos.
	if e.opts.EnableReplicasetStatus && nodeType != typeMongos {
		rsgsc := newReplicationSetStatusCollector(ctx, client, e.opts.Logger,
			e.opts.CompatibleMode, topologyInfo)
		e.cs = append(e.cs, rsgsc)
	}

	return nil
}

func (e *Exporter) getTotalCollectionsCount() int {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.totalCollectionsCount
}

func (e *Exporter) getClient(ctx context.Context) (*mongo.Client, error) {
	// Get global client. Maybe it must be initialized first.
	// Initialization is retried with every scrape until it succeeds once.
	e.clientMu.Lock()
	defer e.clientMu.Unlock()

	// If client is already initialized, return it.
	if e.client != nil {
		return e.client, nil
	}

	client, err := connect(context.Background(), e.opts.URI, e.opts.Username, e.opts.Password, e.opts.DirectConnect)
	if err != nil {
		return nil, err
	}

	e.client = client
	return client, nil
}

func connect(ctx context.Context, dsn, username, password string, directConnect bool) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(dsn)
	opts.SetDirect(directConnect)
	opts.SetAppName("mongodb_exporter")

	if len(username) > 0 || len(password) > 0 {
		opts.SetAuth(options.Credential{
			Username: username,
			Password: password,
		})
	}

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err = client.Ping(ctx, nil); err != nil {
		// Ping failed. Close background connections. Error is ignored since the ping error is more relevant.
		_ = client.Disconnect(ctx)

		return nil, fmt.Errorf("cannot connect to MongoDB: %w", err)
	}

	return client, nil
}
