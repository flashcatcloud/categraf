// mongodb_exporter
// Copyright (C) 2017 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package exporter implements the collectors and metrics handlers.
package exporter

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"flashcat.cloud/categraf/pkg/netx"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ prometheus.Collector = (*Exporter)(nil)

// Exporter holds Exporter methods and attributes.
type Exporter struct {
	client                *mongo.Client
	httpClient            *http.Client // owned policy client; never the SDK's shared default
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

	if err := exp.initCollectors(ctx, exp.client); err != nil {
		exp.Close()
		return nil, err
	}
	return exp, nil
}

func (e *Exporter) Close() {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()
	if e.client != nil {
		e.client.Disconnect(context.Background())
		e.client = nil
	}
	if e.httpClient != nil {
		e.httpClient.CloseIdleConnections()
		e.httpClient = nil
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

	client, httpClient, err := connect(ctx, e.opts.URI, e.opts.Username, e.opts.Password, e.opts.DirectConnect)
	if err != nil {
		return nil, err
	}

	e.client = client
	e.httpClient = httpClient
	return client, nil
}

func connect(ctx context.Context, dsn, username, password string, directConnect bool) (*mongo.Client, *http.Client, error) {
	opts := options.Client().ApplyURI(dsn)
	opts.SetDirect(directConnect)
	opts.SetAppName("mongodb_exporter")

	if len(username) > 0 || len(password) > 0 {
		opts.SetAuth(options.Credential{
			Username: username,
			Password: password,
		})
	}

	httpClient, err := configureIPv4(opts)
	if err != nil {
		return nil, nil, err
	}
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		if httpClient != nil {
			httpClient.CloseIdleConnections()
		}
		return nil, nil, err
	}

	if err = client.Ping(ctx, nil); err != nil {
		// Ping failed. Close background connections. Error is ignored since the ping error is more relevant.
		_ = client.Disconnect(ctx)
		if httpClient != nil {
			httpClient.CloseIdleConnections()
		}

		return nil, nil, fmt.Errorf("cannot connect to MongoDB: %w", err)
	}

	return client, httpClient, nil
}

// configureIPv4 runs before mongo.Connect constructs authenticators and topology.
// The returned HTTP client is private and must be closed after Disconnect.
func configureIPv4(opts *options.ClientOptions) (*http.Client, error) {
	if !netx.PreferIPv4() {
		return nil, nil
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = options.Client().HTTPClient
	}
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if _, ok := transport.(*http.Transport); !ok {
		return nil, fmt.Errorf("MongoDB IPv4 policy requires an adapted HTTP transport, got %T", transport)
	}
	httpClient = netx.HTTPClientFromEnv(httpClient)
	opts.SetHTTPClient(httpClient)
	if opts.Dialer == nil {
		opts.SetDialer(netx.NewDialer(&net.Dialer{}))
	} else {
		opts.SetDialer(contextDialer(netx.DialFuncFromEnv(opts.Dialer.DialContext)))
	}
	return httpClient, nil
}

type contextDialer func(context.Context, string, string) (net.Conn, error)

func (dial contextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dial(ctx, network, addr)
}
