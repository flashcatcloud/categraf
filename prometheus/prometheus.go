//go:build !no_prometheus

package prometheus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/logging"
	"github.com/prometheus/prometheus/util/notifications"

	"github.com/alecthomas/units"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	promlog "github.com/prometheus/common/promslog"
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

	coreconfig "flashcat.cloud/categraf/config"
	keyset "flashcat.cloud/categraf/set/key"
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
func (s *readyStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	if x := s.get(); x != nil {
		return x.Querier(mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

// ChunkQuerier implements the Storage interface.
func (s *readyStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	if x := s.get(); x != nil {
		return x.ChunkQuerier(mint, maxt)
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

func (n notReadyAppender) Append(_ storage.SeriesRef, _ labels.Labels, _ int64, _ float64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendExemplar(_ storage.SeriesRef, _ labels.Labels, _ exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendHistogram(_ storage.SeriesRef, _ labels.Labels, _ int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) UpdateMetadata(_ storage.SeriesRef, _ labels.Labels, _ metadata.Metadata) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendHistogramCTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, _ int64, _ *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendCTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (n notReadyAppender) Rollback() error { return tsdb.ErrNotReady }

func (n notReadyAppender) SetOptions(_ *storage.AppendOptions) {}

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
func (s *readyStorage) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Delete(ctx, mint, maxt, ms...)
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
func (s *readyStorage) Stats(statsByLabelName string, limit int) (*tsdb.Stats, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Head().Stats(statsByLabelName, limit), nil
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

type safePromQLNoStepSubqueryInterval struct {
	value atomic.Int64
}

func (i *safePromQLNoStepSubqueryInterval) Set(ev model.Duration) {
	i.value.Store(durationToInt64Millis(time.Duration(ev)))
}

func (i *safePromQLNoStepSubqueryInterval) Get(int64) int64 {
	return i.value.Load()
}

func reloadConfig(filename string, enableExemplarStorage bool, logger *slog.Logger, noStepSuqueryInterval *safePromQLNoStepSubqueryInterval, callback func(bool), rls ...reloader) (err error) {
	start := time.Now()
	timings := logger
	logger.Info("msg", "Loading configuration file", "filename", filename)

	conf, err := config.LoadFile(filename, true, logger)
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
			logger.Error("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
		timings.With((rl.name), time.Since(rstart))
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	noStepSuqueryInterval.Set(conf.GlobalConfig.EvaluationInterval)
	timings.Info("msg", "Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start))
	return nil
}

type flagConfig struct {
	configFile string

	agentStoragePath            string
	serverStoragePath           string
	notifier                    notifier.Options
	forGracePeriod              model.Duration
	outageTolerance             model.Duration
	resendDelay                 model.Duration
	web                         web.Options
	scrape                      scrape.Options
	tsdb                        tsdbOptions
	agent                       agentOptions
	lookbackDelta               model.Duration
	webTimeout                  model.Duration
	queryTimeout                model.Duration
	queryConcurrency            int
	queryMaxSamples             int
	RemoteFlushDeadline         model.Duration
	maxNotificationsSubscribers int

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
func (c *flagConfig) setFeatureListOptions(logger *slog.Logger) error {
	for _, f := range c.featureList {
		opts := strings.Split(f, ",")
		for _, o := range opts {
			switch o {
			case "expand-external-labels":
				c.enableExpandExternalLabels = true
				logger.Info("msg", "Experimental expand-external-labels enabled")
			case "exemplar-storage":
				c.tsdb.EnableExemplarStorage = true
				logger.Info("msg", "Experimental in-memory exemplar storage enabled")
			case "memory-snapshot-on-shutdown":
				c.tsdb.EnableMemorySnapshotOnShutdown = true
				logger.Info("msg", "Experimental memory snapshot on shutdown enabled")
			case "extra-scrape-metrics":
				c.scrape.ExtraMetrics = true
				logger.Info("msg", "Experimental additional scrape metrics")
			case "new-service-discovery-manager":
				c.enableNewSDManager = true
				logger.Info("msg", "Experimental service discovery manager")
			case "agent":
				logger.Info("msg", "Experimental agent mode enabled.")
			case "promql-per-step-stats":
				c.enablePerStepStats = true
				logger.Info("msg", "Experimental per-step statistics reporting")
			case "auto-gomaxprocs":
				c.enableAutoGOMAXPROCS = true
				logger.Info("msg", "Automatically set GOMAXPROCS to match Linux container CPU quota")
			case "":
				continue
			case "promql-at-modifier", "promql-negative-offset":
				logger.Warn("msg", "This option for --enable-feature is now permanently enabled and therefore a no-op.", "option", o)
			default:
				logger.Info("msg", "Unknown option for --enable-feature", "option", o)
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
	WALCompressionType             string
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
		WALSegmentSize:           int(opts.WALSegmentSize),
		MaxBlockChunkSegmentSize: int64(opts.MaxBlockChunkSegmentSize),
		RetentionDuration:        int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:                 int64(opts.MaxBytes),
		NoLockfile:               opts.NoLockfile,
		//AllowOverlappingCompaction:     true,
		WALCompression:                 wlog.ParseCompressionType(opts.WALCompression, opts.WALCompressionType),
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
	WALCompressionType     string
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
		WALCompression:    wlog.ParseCompressionType(opts.WALCompression, opts.WALCompressionType),
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

func debug() bool {
	return coreconfig.Config.DebugMode && strings.Contains(coreconfig.Config.InputFilters, keyset.PrometheusAgent)
}

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

	if debug() {
		cfg.promlogConfig.Level.Set("debug")
	} else {
		cfg.promlogConfig.Level.Set(coreconfig.Config.Prometheus.LogLevel)
	}

	if cfg.promlogConfig.Level.String() == "" {
		cfg.promlogConfig.Level.Set("info")
	}

	notifs := notifications.NewNotifications(cfg.maxNotificationsSubscribers, prometheus.DefaultRegisterer)
	cfg.web.NotificationsSub = notifs.Sub
	cfg.web.NotificationsGetter = notifs.Get
	notifs.AddNotification(notifications.StartingUp)

	logger := promlog.New(&cfg.promlogConfig)
	slog.SetDefault(logger)
	if err = cfg.setFeatureListOptions(logger); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("%s Error parsing feature list", err))
		os.Exit(1)
	}

	notifierManager := notifier.NewManager(&cfg.notifier, logger.With(logger, "component", "notifier"))

	ctxScrape, cancelScrape := context.WithCancel(context.Background())
	ctxNotify, cancelNotify := context.WithCancel(context.Background())

	sdMetrics, err := discovery.CreateAndRegisterSDMetrics(prometheus.DefaultRegisterer)
	if err != nil {
		logger.Error("failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	discoveryManagerScrape := discovery.NewManager(ctxScrape, logger.With("component", "discovery manager scrape"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("scrape"))
	discoveryManagerNotify := discovery.NewManager(ctxNotify, logger.With("component", "discovery manager notify"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("notify"))

	if cfg.scrape.ExtraMetrics {
		// Experimental additional scrape metrics
		// TODO scrapeopts configurable
	}

	noStepSubqueryInterval := &safePromQLNoStepSubqueryInterval{}
	noStepSubqueryInterval.Set(config.DefaultGlobalConfig.EvaluationInterval)

	localStorage := &readyStorage{stats: tsdb.NewDBStats()}

	cfg.agentStoragePath = coreconfig.Config.Prometheus.StoragePath
	if len(cfg.agentStoragePath) == 0 {
		cfg.agentStoragePath = "./data-agent"
	}
	if coreconfig.Config.Prometheus.MinBlockDuration == coreconfig.Duration(0) {
		// keep data in memory for 10min by default
		cfg.tsdb.MinBlockDuration = model.Duration(10 * time.Minute)
	} else {
		cfg.tsdb.MinBlockDuration = model.Duration(coreconfig.Config.Prometheus.MinBlockDuration)
	}
	if coreconfig.Config.Prometheus.MaxBlockDuration == coreconfig.Duration(0) {
		cfg.tsdb.MaxBlockDuration = model.Duration(20 * time.Minute)
	} else {
		cfg.tsdb.MaxBlockDuration = model.Duration(coreconfig.Config.Prometheus.MaxBlockDuration)
	}
	if coreconfig.Config.Prometheus.RetentionDuration == coreconfig.Duration(0) {
		// reserve data for 24h by default
		cfg.tsdb.RetentionDuration = model.Duration(24 * time.Hour)
	} else {
		cfg.tsdb.RetentionDuration = model.Duration(coreconfig.Config.Prometheus.RetentionDuration)
	}
	if len(coreconfig.Config.Prometheus.RetentionSize) == 0 {
		// max size is 1GB by default
		cfg.tsdb.MaxBytes = 1024 * 1024 * 1024
	} else {
		cfg.tsdb.MaxBytes, err = units.ParseBase2Bytes(coreconfig.Config.Prometheus.RetentionSize)
		if err != nil {
			panic(fmt.Sprintf("retention_size:%s format error %s", coreconfig.Config.Prometheus.RetentionSize, err))
		}
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
	remoteStorage := remote.NewStorage(logger.With(logger, "component", "remote"), prometheus.DefaultRegisterer, localStorage.StartTime, localStoragePath, time.Duration(remoteFlushDeadline), scraper, cfg.scrape.AppendMetadata)
	fanoutStorage := storage.NewFanout(logger, localStorage, remoteStorage)

	scrapeManager, err := scrape.NewManager(
		&cfg.scrape,
		logger.With(logger, "component", "scrape manager"),
		logging.NewJSONFileLogger,
		fanoutStorage,
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		logger.Error("failed to create a scrape manager", "err", err)
		os.Exit(1)
	}
	scraper.Set(scrapeManager)

	if len(coreconfig.Config.Prometheus.WebAddress) != 0 {
		cfg.web.ListenAddresses = append(cfg.web.ListenAddresses, coreconfig.Config.Prometheus.WebAddress)
	} else {
		cfg.web.ListenAddresses = append(cfg.web.ListenAddresses, "127.0.0.1:0")
	}

	cfg.web.ExternalURL, err = computeExternalURL(cfg.prometheusURL, cfg.web.ListenAddresses[0])
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

	webHandler := web.New(logger.With(logger, "component", "web"), &cfg.web)
	listener, err := webHandler.Listeners()
	if err != nil {
		logger.Info("msg", "Unable to start web listener", "err", err)
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
		signal.Notify(term, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				// Don't forget to release the reloadReady channel so that waiting blocks can exit normally.
				select {
				case sig := <-term:
					logger.Warn("msg", "Received "+sig.String()+" exiting gracefully...")
					reloadReady.Close()
				case <-webHandler.Quit():
					logger.Warn("msg", "Received termination request via web service, exiting gracefully...")
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
				webHandler.SetReady(web.Stopping)
				notifs.AddNotification(notifications.ShuttingDown)
			},
		)
	}
	{
		// Scrape discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerScrape.Run()
				logger.Info("msg", "Scrape discovery manager stopped")
				return err
			},
			func(err error) {
				logger.Info("msg", "Stopping scrape discovery manager...")
				cancelScrape()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerNotify.Run()
				logger.Info("msg", "Notify discovery manager stopped")
				return err
			},
			func(err error) {
				logger.Info("msg", "Stopping notify discovery manager...")
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
				logger.Info("msg", "Scrape manager stopped")
				return err
			},
			func(err error) {
				// Scrape manager needs to be stopped before closing the local TSDB
				// so that it doesn't try to write samples to a closed storage.
				logger.Info("msg", "Stopping scrape manager...")
				scrapeManager.Stop()
			},
		)
	}
	{
		// Reload handler.
		callback := func(success bool) {
			if success {
				notifs.DeleteNotification(notifications.ConfigurationUnsuccessful)
				return
			}
			notifs.AddNotification(notifications.ConfigurationUnsuccessful)
		}

		// Make sure that sighup handler is registered with a redirect to the channel before the potentially
		// long and synchronous tsdb init.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP, syscall.SIGPIPE)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				<-reloadReady.C

				for {
					select {
					case ch := <-hup:
						if ch == syscall.SIGPIPE {
							// broken pipe , do nothing
							continue
						}
						if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, callback, reloaders...); err != nil {
							logger.Error("msg", "Error reloading config", "err", err)
						}
					case rc := <-webHandler.Reload():
						if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, callback, reloaders...); err != nil {
							logger.Error("msg", "Error reloading config", "err", err)
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

				if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, func(bool) {}, reloaders...); err != nil {
					return fmt.Errorf("%s error loading config from %q", err, cfg.configFile)
				}

				reloadReady.Close()

				webHandler.SetReady(web.Ready)
				notifs.DeleteNotification(notifications.StartingUp)
				logger.Info("msg", "server is ready.")
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
				logger.Info("msg", "Starting WAL storage ...")
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
					logger.Warn("fs_type", fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
				default:
					logger.Info("fs_type", fsType)
				}

				logger.Info("msg", "Agent WAL storage started")
				logger.Info("msg", "Agent WAL storage options",
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
					logger.Error("msg", "Error stopping storage", "err", err)
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
				logger.Info("msg", "Notifier manager stopped")
				return nil
			},
			func(err error) {
				notifierManager.Stop()
			},
		)
	}
	atomic.StoreInt32(&isRunning, 1)
	if err := g.Run(); err != nil {
		logger.Error("err", err)
		os.Exit(1)
	}
	logger.Info("msg", "See you next time!")
}

func Stop() {
	// if stop != nil {
	// 	close(stop)
	// }
}
