//go:build !no_prometheus

package prometheus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	coreconfig "flashcat.cloud/categraf/config"
	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/install"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/agent"
	prom_runtime "github.com/prometheus/prometheus/util/runtime"
	"github.com/prometheus/prometheus/web"
)

// config toml/yaml
type readyScrapeManager struct {
	mtx sync.RWMutex
	m   *scrape.Manager
}

// Set the scrape manager.
func (rm *readyScrapeManager) Set(m *scrape.Manager) {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()

	rm.m = m
}

func (rm *readyScrapeManager) Get() (*scrape.Manager, error) {
	rm.mtx.RLock()
	defer rm.mtx.RUnlock()

	if rm.m != nil {
		return rm.m, nil
	}

	return nil, ErrNotReady
}

// readyStorage implements the Storage interface while allowing to set the actual
// storage at a later point in time.
type readyStorage struct {
	mtx             sync.RWMutex
	db              storage.Storage
	startTimeMargin int64
	stats           *tsdb.DBStats
}

func (s *readyStorage) ApplyConfig(conf *config.Config) error {
	db := s.get()
	if db, ok := db.(*tsdb.DB); ok {
		return db.ApplyConfig(conf)
	}
	return nil
}

// Set the storage.
func (s *readyStorage) Set(db storage.Storage, startTimeMargin int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.db = db
	s.startTimeMargin = startTimeMargin
}

func (s *readyStorage) get() storage.Storage {
	s.mtx.RLock()
	x := s.db
	s.mtx.RUnlock()
	return x
}

func (s *readyStorage) getStats() *tsdb.DBStats {
	s.mtx.RLock()
	x := s.stats
	s.mtx.RUnlock()
	return x
}

// StartTime implements the Storage interface.
func (s *readyStorage) StartTime() (int64, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			var startTime int64
			if len(db.Blocks()) > 0 {
				startTime = db.Blocks()[0].Meta().MinTime
			} else {
				startTime = time.Now().Unix() * 1000
			}
			// Add a safety margin as it may take a few minutes for everything to spin up.
			return startTime + s.startTimeMargin, nil
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}

	return math.MaxInt64, tsdb.ErrNotReady
}

// Querier implements the Storage interface.
func (s *readyStorage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	if x := s.get(); x != nil {
		return x.Querier(ctx, mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

// ChunkQuerier implements the Storage interface.
func (s *readyStorage) ChunkQuerier(ctx context.Context, mint, maxt int64) (storage.ChunkQuerier, error) {
	if x := s.get(); x != nil {
		return x.ChunkQuerier(ctx, mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

func (s *readyStorage) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.ExemplarQuerier(ctx)
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
}

// Appender implements the Storage interface.
func (s *readyStorage) Appender(ctx context.Context) storage.Appender {
	if x := s.get(); x != nil {
		return x.Appender(ctx)
	}
	return notReadyAppender{}
}

type notReadyAppender struct{}

func (n notReadyAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (n notReadyAppender) Rollback() error { return tsdb.ErrNotReady }

// Close implements the Storage interface.
func (s *readyStorage) Close() error {
	if x := s.get(); x != nil {
		return x.Close()
	}
	return nil
}

// CleanTombstones implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) CleanTombstones() error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.CleanTombstones()
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Delete implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) Delete(mint, maxt int64, ms ...*labels.Matcher) error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Delete(mint, maxt, ms...)
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Snapshot implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) Snapshot(dir string, withHead bool) error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Snapshot(dir, withHead)
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Stats implements the api_v1.TSDBAdminStats interface.
func (s *readyStorage) Stats(statsByLabelName string) (*tsdb.Stats, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Head().Stats(statsByLabelName), nil
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
}

// WALReplayStatus implements the api_v1.TSDBStats interface.
func (s *readyStorage) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	if x := s.getStats(); x != nil {
		return x.Head.WALReplayStatus.GetWALReplayStatus(), nil
	}
	return tsdb.WALReplayStatus{}, tsdb.ErrNotReady
}

// ErrNotReady is returned if the underlying scrape manager is not ready yet.
var ErrNotReady = errors.New("Scrape manager not ready")

type reloader struct {
	name     string
	reloader func(*config.Config) error
}

func reloadConfig(filename string, expandExternalLabels, enableExemplarStorage bool, logger log.Logger, rls ...reloader) (err error) {
	start := time.Now()
	timings := []interface{}{}
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	conf, err := config.LoadFile(filename, true, false, logger)
	if err != nil {
		return fmt.Errorf("%s couldn't load configuration (--config.file=%q)", err, filename)
	}

	if enableExemplarStorage {
		if conf.StorageConfig.ExemplarsConfig == nil {
			conf.StorageConfig.ExemplarsConfig = &config.DefaultExemplarsConfig
		}
	}

	failed := false
	for _, rl := range rls {
		rstart := time.Now()
		if err := rl.reloader(conf); err != nil {
			level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
		timings = append(timings, rl.name, time.Since(rstart))
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	l := []interface{}{"msg", "Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start)}
	level.Info(logger).Log(append(l, timings...)...)
	return nil
}

type flagConfig struct {
	configFile string

	agentStoragePath    string
	serverStoragePath   string
	notifier            notifier.Options
	forGracePeriod      model.Duration
	outageTolerance     model.Duration
	resendDelay         model.Duration
	web                 web.Options
	scrape              scrape.Options
	tsdb                tsdbOptions
	agent               agentOptions
	lookbackDelta       model.Duration
	webTimeout          model.Duration
	queryTimeout        model.Duration
	queryConcurrency    int
	queryMaxSamples     int
	RemoteFlushDeadline model.Duration

	featureList []string
	// These options are extracted from featureList
	// for ease of use.
	enableExpandExternalLabels bool
	enableNewSDManager         bool
	enablePerStepStats         bool
	enableAutoGOMAXPROCS       bool

	prometheusURL   string
	corsRegexString string

	promlogConfig promlog.Config
}

// setFeatureListOptions sets the corresponding options from the featureList.
func (c *flagConfig) setFeatureListOptions(logger log.Logger) error {
	for _, f := range c.featureList {
		opts := strings.Split(f, ",")
		for _, o := range opts {
			switch o {
			case "expand-external-labels":
				c.enableExpandExternalLabels = true
				level.Info(logger).Log("msg", "Experimental expand-external-labels enabled")
			case "exemplar-storage":
				c.tsdb.EnableExemplarStorage = true
				level.Info(logger).Log("msg", "Experimental in-memory exemplar storage enabled")
			case "memory-snapshot-on-shutdown":
				c.tsdb.EnableMemorySnapshotOnShutdown = true
				level.Info(logger).Log("msg", "Experimental memory snapshot on shutdown enabled")
			case "extra-scrape-metrics":
				c.scrape.ExtraMetrics = true
				level.Info(logger).Log("msg", "Experimental additional scrape metrics")
			case "new-service-discovery-manager":
				c.enableNewSDManager = true
				level.Info(logger).Log("msg", "Experimental service discovery manager")
			case "agent":
				level.Info(logger).Log("msg", "Experimental agent mode enabled.")
			case "promql-per-step-stats":
				c.enablePerStepStats = true
				level.Info(logger).Log("msg", "Experimental per-step statistics reporting")
			case "auto-gomaxprocs":
				c.enableAutoGOMAXPROCS = true
				level.Info(logger).Log("msg", "Automatically set GOMAXPROCS to match Linux container CPU quota")
			case "":
				continue
			case "promql-at-modifier", "promql-negative-offset":
				level.Warn(logger).Log("msg", "This option for --enable-feature is now permanently enabled and therefore a no-op.", "option", o)
			default:
				level.Warn(logger).Log("msg", "Unknown option for --enable-feature", "option", o)
			}
		}
	}
	return nil
}

type tsdbOptions struct {
	WALSegmentSize                 units.Base2Bytes
	MaxBlockChunkSegmentSize       units.Base2Bytes
	RetentionDuration              model.Duration
	MaxBytes                       units.Base2Bytes
	NoLockfile                     bool
	AllowOverlappingBlocks         bool
	WALCompression                 bool
	HeadChunksWriteQueueSize       int
	StripeSize                     int
	MinBlockDuration               model.Duration
	MaxBlockDuration               model.Duration
	EnableExemplarStorage          bool
	MaxExemplars                   int64
	EnableMemorySnapshotOnShutdown bool
}

func (opts tsdbOptions) ToTSDBOptions() tsdb.Options {
	return tsdb.Options{
		WALSegmentSize:                 int(opts.WALSegmentSize),
		MaxBlockChunkSegmentSize:       int64(opts.MaxBlockChunkSegmentSize),
		RetentionDuration:              int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:                       int64(opts.MaxBytes),
		NoLockfile:                     opts.NoLockfile,
		AllowOverlappingBlocks:         opts.AllowOverlappingBlocks,
		WALCompression:                 opts.WALCompression,
		HeadChunksWriteQueueSize:       opts.HeadChunksWriteQueueSize,
		StripeSize:                     opts.StripeSize,
		MinBlockDuration:               int64(time.Duration(opts.MinBlockDuration) / time.Millisecond),
		MaxBlockDuration:               int64(time.Duration(opts.MaxBlockDuration) / time.Millisecond),
		EnableExemplarStorage:          opts.EnableExemplarStorage,
		MaxExemplars:                   opts.MaxExemplars,
		EnableMemorySnapshotOnShutdown: opts.EnableMemorySnapshotOnShutdown,
	}
}

// agentOptions is a version of agent.Options with defined units. This is required
// as agent.Option fields are unit agnostic (time).
type agentOptions struct {
	WALSegmentSize         units.Base2Bytes
	WALCompression         bool
	StripeSize             int
	TruncateFrequency      model.Duration
	MinWALTime, MaxWALTime model.Duration
	NoLockfile             bool
}

func durationToInt64Millis(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
}

func computeExternalURL(u, listenAddr string) (*url.URL, error) {
	if u == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return nil, err
		}
		u = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if startsOrEndsWithQuote(u) {
		return nil, errors.New("URL must not begin or end with quotes")
	}

	eu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(eu.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	eu.Path = ppref

	return eu, nil
}

func (opts agentOptions) ToAgentOptions() agent.Options {
	return agent.Options{
		WALSegmentSize:    int(opts.WALSegmentSize),
		WALCompression:    opts.WALCompression,
		StripeSize:        opts.StripeSize,
		TruncateFrequency: time.Duration(opts.TruncateFrequency),
		MinWALTime:        durationToInt64Millis(time.Duration(opts.MinWALTime)),
		MaxWALTime:        durationToInt64Millis(time.Duration(opts.MaxWALTime)),
		NoLockfile:        opts.NoLockfile,
	}
}

var (
	stop      = make(chan struct{})
	isRunning int32
)

func Start() {
	var (
		err error
	)
	if atomic.LoadInt32(&isRunning) > 0 {
		return
	}
	cfg := flagConfig{
		notifier: notifier.Options{
			Registerer: prometheus.DefaultRegisterer,
		},
		web: web.Options{
			Registerer: prometheus.DefaultRegisterer,
			Gatherer:   prometheus.DefaultGatherer,
		},
		promlogConfig: promlog.Config{
			Level: &promlog.AllowedLevel{},
		},
	}

	if coreconfig.Config.DebugMode || coreconfig.Config.TestMode {
		cfg.promlogConfig.Level.Set("debug")
	} else {
		cfg.promlogConfig.Level.Set(coreconfig.Config.Prometheus.LogLevel)
	}

	if cfg.promlogConfig.Level.String() == "" {
		cfg.promlogConfig.Level.Set("info")
	}

	logger := promlog.New(&cfg.promlogConfig)
	if err = cfg.setFeatureListOptions(logger); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("%s Error parsing feature list", err))
		os.Exit(1)
	}

	notifierManager := notifier.NewManager(&cfg.notifier, log.With(logger, "component", "notifier"))

	ctxScrape, cancelScrape := context.WithCancel(context.Background())
	ctxNotify, cancelNotify := context.WithCancel(context.Background())

	discoveryManagerScrape := discovery.NewManager(ctxScrape, log.With(logger, "component", "discovery manager scrape"), discovery.Name("scrape"))
	discoveryManagerNotify := discovery.NewManager(ctxNotify, log.With(logger, "component", "discovery manager notify"), discovery.Name("notify"))

	if cfg.scrape.ExtraMetrics {
		// Experimental additional scrape metrics
		// TODO scrapeopts configurable
	}

	localStorage := &readyStorage{stats: tsdb.NewDBStats()}

	cfg.agentStoragePath = coreconfig.Config.Prometheus.StoragePath
	if len(cfg.agentStoragePath) == 0 {
		cfg.agentStoragePath = "./data-agent"
	}
	if cfg.tsdb.MinBlockDuration == model.Duration(0) {
		cfg.tsdb.MinBlockDuration = model.Duration(2 * time.Hour)
	}
	if cfg.webTimeout == model.Duration(0) {
		cfg.webTimeout = model.Duration(time.Minute * 5)
	}
	cfg.web.ReadTimeout = time.Duration(cfg.webTimeout)
	if cfg.web.MaxConnections == 0 {
		cfg.web.MaxConnections = 512
	}
	cfg.web.EnableAdminAPI = false
	cfg.web.EnableRemoteWriteReceiver = false

	if cfg.web.RemoteReadSampleLimit == 0 {
		cfg.web.RemoteReadSampleLimit = 5e7
	}
	if cfg.web.RemoteReadConcurrencyLimit == 0 {
		cfg.web.RemoteReadConcurrencyLimit = 10
	}
	if cfg.web.RemoteReadBytesInFrame == 0 {
		cfg.web.RemoteReadBytesInFrame = 1048576
	}
	if len(cfg.configFile) == 0 {
		cfg.configFile = coreconfig.Config.Prometheus.ScrapeConfigFile
	}

	scraper := &readyScrapeManager{}

	remoteFlushDeadline := time.Duration(1 * time.Minute)
	localStoragePath := cfg.agentStoragePath
	remoteStorage := remote.NewStorage(log.With(logger, "component", "remote"), prometheus.DefaultRegisterer, localStorage.StartTime, localStoragePath, time.Duration(remoteFlushDeadline), scraper)
	fanoutStorage := storage.NewFanout(logger, localStorage, remoteStorage)

	scrapeManager := scrape.NewManager(&cfg.scrape, log.With(logger, "component", "scrape manager"), fanoutStorage)
	scraper.Set(scrapeManager)

	cfg.web.ListenAddress = coreconfig.Config.Prometheus.WebAddress
	if len(cfg.web.ListenAddress) == 0 {
		cfg.web.ListenAddress = "127.0.0.1:0"
	}

	cfg.web.ExternalURL, err = computeExternalURL(cfg.prometheusURL, cfg.web.ListenAddress)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("%s parse external URL %q", err, cfg.prometheusURL))
		os.Exit(2)
	}

	if cfg.web.RoutePrefix == "" {
		cfg.web.RoutePrefix = cfg.web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.web.RoutePrefix = "/" + strings.Trim(cfg.web.RoutePrefix, "/")

	ctxWeb, cancelWeb := context.WithCancel(context.Background())
	cfg.web.Context = ctxWeb
	cfg.web.TSDBRetentionDuration = cfg.tsdb.RetentionDuration
	cfg.web.TSDBMaxBytes = cfg.tsdb.MaxBytes
	cfg.web.TSDBDir = localStoragePath
	cfg.web.LocalStorage = localStorage
	cfg.web.Storage = fanoutStorage
	cfg.web.ExemplarStorage = localStorage
	cfg.web.ScrapeManager = scrapeManager
	cfg.web.QueryEngine = nil
	cfg.web.RuleManager = nil
	cfg.web.Notifier = notifierManager
	cfg.web.LookbackDelta = time.Duration(cfg.lookbackDelta)
	cfg.web.IsAgent = true

	webHandler := web.New(log.With(logger, "component", "web"), &cfg.web)
	listener, err := webHandler.Listener()
	if err != nil {
		level.Error(logger).Log("msg", "Unable to start web listener", "err", err)
		os.Exit(1)
	}

	reloaders := []reloader{
		{
			name:     "remote_storage",
			reloader: remoteStorage.ApplyConfig,
		}, {
			name:     "web_handler",
			reloader: webHandler.ApplyConfig,
		}, {
			// The Scrape and notifier managers need to reload before the Discovery manager as
			// they need to read the most updated config when receiving the new targets list.
			name:     "scrape",
			reloader: scrapeManager.ApplyConfig,
		}, {
			name: "scrape_sd",
			reloader: func(cfg *config.Config) error {
				c := make(map[string]discovery.Configs)
				for _, v := range cfg.ScrapeConfigs {
					c[v.JobName] = v.ServiceDiscoveryConfigs
				}
				return discoveryManagerScrape.ApplyConfig(c)
			},
		}, {
			name:     "notify",
			reloader: notifierManager.ApplyConfig,
		}, {
			name: "notify_sd",
			reloader: func(cfg *config.Config) error {
				c := make(map[string]discovery.Configs)
				for k, v := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
					c[k] = v.ServiceDiscoveryConfigs
				}
				return discoveryManagerNotify.ApplyConfig(c)
			},
		},
	}

	dbOpen := make(chan struct{})
	type closeOnce struct {
		C     chan struct{}
		once  sync.Once
		Close func()
	}
	// Wait until the server is ready to handle reloading.
	reloadReady := &closeOnce{
		C: make(chan struct{}),
	}
	reloadReady.Close = func() {
		reloadReady.once.Do(func() {
			close(reloadReady.C)
		})
	}

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGPIPE)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				// Don't forget to release the reloadReady channel so that waiting blocks can exit normally.
				select {
				case sig := <-term:
					level.Warn(logger).Log("msg", "Received "+sig.String())
					if sig != syscall.SIGPIPE {
						level.Warn(logger).Log("msg", "exiting gracefully...")
						reloadReady.Close()
					}
				case <-webHandler.Quit():
					level.Warn(logger).Log("msg", "Received termination request via web service, exiting gracefully...")
				case <-cancel:
					reloadReady.Close()
				case <-stop:
					reloadReady.Close()
				}
				atomic.StoreInt32(&isRunning, 0)
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		// Scrape discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerScrape.Run()
				level.Info(logger).Log("msg", "Scrape discovery manager stopped")
				return err
			},
			func(err error) {
				level.Info(logger).Log("msg", "Stopping scrape discovery manager...")
				cancelScrape()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerNotify.Run()
				level.Info(logger).Log("msg", "Notify discovery manager stopped")
				return err
			},
			func(err error) {
				level.Info(logger).Log("msg", "Stopping notify discovery manager...")
				cancelNotify()
			},
		)
	}
	{
		// Scrape manager.
		g.Add(
			func() error {
				// When the scrape manager receives a new targets list
				// it needs to read a valid config for each job.
				// It depends on the config being in sync with the discovery manager so
				// we wait until the config is fully loaded.
				<-reloadReady.C

				err := scrapeManager.Run(discoveryManagerScrape.SyncCh())
				level.Info(logger).Log("msg", "Scrape manager stopped")
				return err
			},
			func(err error) {
				// Scrape manager needs to be stopped before closing the local TSDB
				// so that it doesn't try to write samples to a closed storage.
				level.Info(logger).Log("msg", "Stopping scrape manager...")
				scrapeManager.Stop()
			},
		)
	}
	{
		// Reload handler.

		// Make sure that sighup handler is registered with a redirect to the channel before the potentially
		// long and synchronous tsdb init.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				<-reloadReady.C

				for {
					select {
					case <-hup:
						if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-webHandler.Reload():
						if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
							rc <- err
						} else {
							rc <- nil
						}
					case <-cancel:
						return nil
					}
				}
			},
			func(err error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}
	{
		// Initial configuration loading.
		cancel := make(chan struct{})
		g.Add(
			func() error {
				select {
				case <-dbOpen:
				// In case a shutdown is initiated before the dbOpen is released
				case <-cancel:
					reloadReady.Close()
					return nil
				}

				if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, reloaders...); err != nil {
					return fmt.Errorf("%s error loading config from %q", err, cfg.configFile)
				}

				reloadReady.Close()

				webHandler.SetReady(true)
				level.Info(logger).Log("msg", "Server is ready to receive web requests.")
				<-cancel
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		// WAL storage.
		opts := cfg.agent.ToAgentOptions()
		cancel := make(chan struct{})
		g.Add(
			func() error {
				level.Info(logger).Log("msg", "Starting WAL storage ...")
				if cfg.agent.WALSegmentSize != 0 {
					if cfg.agent.WALSegmentSize < 10*1024*1024 || cfg.agent.WALSegmentSize > 256*1024*1024 {
						return errors.New("flag 'storage.agent.wal-segment-size' must be set between 10MB and 256MB")
					}
				}
				db, err := agent.Open(
					logger,
					prometheus.DefaultRegisterer,
					remoteStorage,
					localStoragePath,
					&opts,
				)
				if err != nil {
					return fmt.Errorf("opening storage failed %s", err)
				}

				switch fsType := prom_runtime.Statfs(localStoragePath); fsType {
				case "NFS_SUPER_MAGIC":
					level.Warn(logger).Log("fs_type", fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
				default:
					level.Info(logger).Log("fs_type", fsType)
				}

				level.Info(logger).Log("msg", "Agent WAL storage started")
				level.Debug(logger).Log("msg", "Agent WAL storage options",
					"WALSegmentSize", cfg.agent.WALSegmentSize,
					"WALCompression", cfg.agent.WALCompression,
					"StripeSize", cfg.agent.StripeSize,
					"TruncateFrequency", cfg.agent.TruncateFrequency,
					"MinWALTime", cfg.agent.MinWALTime,
					"MaxWALTime", cfg.agent.MaxWALTime,
				)

				localStorage.Set(db, 0)
				close(dbOpen)
				<-cancel
				return nil
			},
			func(e error) {
				if err := fanoutStorage.Close(); err != nil {
					level.Error(logger).Log("msg", "Error stopping storage", "err", err)
				}
				close(cancel)
			},
		)
	}
	{
		// Web handler.
		g.Add(
			func() error {
				if err := webHandler.Run(ctxWeb, listener, ""); err != nil {
					return fmt.Errorf("%s error starting web server", err)
				}
				return nil
			},
			func(err error) {
				cancelWeb()
			},
		)
	}
	{
		// Notifier.

		// Calling notifier.Stop() before ruleManager.Stop() will cause a panic if the ruleManager isn't running,
		// so keep this interrupt after the ruleManager.Stop().
		g.Add(
			func() error {
				// When the notifier manager receives a new targets list
				// it needs to read a valid config for each job.
				// It depends on the config being in sync with the discovery manager
				// so we wait until the config is fully loaded.
				<-reloadReady.C

				notifierManager.Run(discoveryManagerNotify.SyncCh())
				level.Info(logger).Log("msg", "Notifier manager stopped")
				return nil
			},
			func(err error) {
				notifierManager.Stop()
			},
		)
	}
	atomic.StoreInt32(&isRunning, 1)
	if err := g.Run(); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "See you next time!")
}

func Stop() {
	// if stop != nil {
	// 	close(stop)
	// }
}
