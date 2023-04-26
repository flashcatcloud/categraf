package mtail

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/net/context"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
	"flashcat.cloud/categraf/inputs/mtail/internal/mtail"
	"flashcat.cloud/categraf/inputs/mtail/internal/waker"
	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

const inputName = `mtail`
const description = ` extract internal monitoring data from application logs`

// MTail holds the configuration for the plugin.
type MTail struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig

	NamePrefix           string        `toml:"name_prefix"`
	Progs                string        `toml:"progs"`
	Logs                 []string      `toml:"logs"`
	IgnoreFileRegPattern string        `toml:"ignore_filename_regex_pattern"`
	OverrideTimeZone     string        `toml:"override_timezone"`
	EmitProgLabel        string        `toml:"emit_prog_label"`
	emitProgLabel        bool          `toml:"-"`
	EmitMetricTimestamp  string        `toml:"emit_metric_timestamp"`
	emitMetricTimestamp  bool          `toml:"-"`
	PollInterval         time.Duration `toml:"poll_interval"`
	PollLogInterval      time.Duration `toml:"poll_log_interval"`
	MetricPushInterval   time.Duration `toml:"metric_push_interval"`
	MaxRegexpLen         int           `toml:"max_regexp_length"`
	MaxRecursionDepth    int           `toml:"max_recursion_depth"`

	SyslogUseCurrentYear string `toml:"syslog_use_current_year"` // true
	sysLogUseCurrentYear bool   `toml:"-"`
	LogRuntimeErrors     string `toml:"vm_logs_runtime_errors"` // true
	logRuntimeErrors     bool   `toml:"-"`
	//
	ctx    context.Context    `toml:"-"`
	cancel context.CancelFunc `toml:"-"`
	m      *mtail.Server
}

func (ins *Instance) Init() error {

	if len(ins.Progs) == 0 || len(ins.Logs) == 0 {
		return types.ErrInstancesEmpty
	}

	// set default value
	ins.sysLogUseCurrentYear = ins.SyslogUseCurrentYear == "true"
	ins.logRuntimeErrors = ins.LogRuntimeErrors == "true"
	ins.emitProgLabel = ins.EmitProgLabel == "true"
	ins.emitMetricTimestamp = ins.EmitMetricTimestamp == "true"

	if ins.PollLogInterval == 0 {
		ins.PollLogInterval = 250 * time.Millisecond
	}
	if ins.PollInterval == 0 {
		ins.PollInterval = 250 * time.Millisecond
	}
	if ins.MetricPushInterval == 0 {
		ins.MetricPushInterval = 1 * time.Minute
	}
	if ins.MaxRegexpLen == 0 {
		ins.MaxRegexpLen = 1024
	}
	if ins.MaxRecursionDepth == 0 {
		ins.MaxRecursionDepth = 100
	}
	buildInfo := mtail.BuildInfo{
		Version: config.Version,
	}
	loc, err := time.LoadLocation(ins.OverrideTimeZone)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't parse timezone %q: %s", ins.OverrideTimeZone, err)
		return err
	}

	opts := []mtail.Option{
		mtail.ProgramPath(ins.Progs),
		mtail.LogPathPatterns(ins.Logs...),
		mtail.IgnoreRegexPattern(ins.IgnoreFileRegPattern),
		mtail.SetBuildInfo(buildInfo),
		mtail.OverrideLocation(loc),
		mtail.MetricPushInterval(ins.MetricPushInterval), // keep it here ?
		mtail.MaxRegexpLength(ins.MaxRegexpLen),
		mtail.MaxRecursionDepth(ins.MaxRecursionDepth),
		mtail.LogRuntimeErrors,
	}
	if ins.cancel != nil {
		ins.cancel()
	} else {
		ins.ctx, ins.cancel = context.WithCancel(context.Background())
	}
	staleLogGcWaker := waker.NewTimed(ins.ctx, time.Hour)
	opts = append(opts, mtail.StaleLogGcWaker(staleLogGcWaker))

	if ins.PollInterval > 0 {
		logStreamPollWaker := waker.NewTimed(ins.ctx, ins.PollInterval)
		logPatternPollWaker := waker.NewTimed(ins.ctx, ins.PollLogInterval)
		opts = append(opts, mtail.LogPatternPollWaker(logPatternPollWaker), mtail.LogstreamPollWaker(logStreamPollWaker))
	}
	if ins.sysLogUseCurrentYear {
		opts = append(opts, mtail.SyslogUseCurrentYear)
	}
	if !ins.emitProgLabel {
		opts = append(opts, mtail.OmitProgLabel)
	}
	if ins.emitMetricTimestamp {
		opts = append(opts, mtail.EmitMetricTimestamp)
	}

	store := metrics.NewStore()
	store.StartGcLoop(ins.ctx, time.Hour)

	m, err := mtail.New(ins.ctx, store, opts...)
	if err != nil {
		log.Println(err)
		ins.cancel()
		return err
	}
	ins.m = m

	return nil
}

func (ins *Instance) Drop() {
	ins.cancel()
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &MTail{}
	})
}

func (s *MTail) Clone() inputs.Input {
	return &MTail{}
}

func (s *MTail) Name() string {
	return inputName
}

func (s *MTail) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

// Description returns a one-sentence description on the input.
func (s *MTail) Description() string {
	return description
}

// Gather retrieves all the configured fields and tables.
// Any error encountered does not halt the process. The errors are accumulated
// and returned at the end.
// func (s *Instance) Gather(acc telegraf.Accumulator) error {
func (ins *Instance) Gather(slist *types.SampleList) {
	reg := ins.m.GetRegistry()
	mfs, done, err := prometheus.ToTransactionalGatherer(reg).Gather()
	if err != nil {
		log.Println(err)
		return
	}
	defer done()
	for _, mf := range mfs {
		metricName := mf.GetName()
		for _, m := range mf.Metric {
			tags := util.MakeLabels(m, ins.GetLabels())

			if mf.GetType() == dto.MetricType_SUMMARY {
				util.HandleSummary(inputName, m, tags, metricName, ins.GetLogMetricTime, slist)
			} else if mf.GetType() == dto.MetricType_HISTOGRAM {
				util.HandleHistogram(inputName, m, tags, metricName, ins.GetLogMetricTime, slist)
			} else {
				util.HandleGaugeCounter(inputName, m, tags, metricName, ins.GetLogMetricTime, slist)
			}
		}
	}
}

func (p *Instance) GetLogMetricTime(ts int64) time.Time {
	var tm time.Time
	if ts <= 0 || !p.emitMetricTimestamp {
		return tm
	}
	sec := ts / 1000
	ms := ts % 1000 * 1e6
	tm = time.Unix(sec, ms)
	return tm
}
