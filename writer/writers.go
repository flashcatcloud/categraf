package writer

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

// Writers manage all writers and metric queue
type Writers struct {
	writerMap map[string]Writer
	queue     *types.SafeListLimited[*prompb.TimeSeries]
}

var writers Writers

func InitWriters() error {
	writerMap := map[string]Writer{}
	opts := config.Config.Writers
	for _, opt := range opts {
		writer, err := newWriter(opt)
		if err != nil {
			return err
		}
		writerMap[opt.Url] = writer
	}

	writers = Writers{
		writerMap: writerMap,
		queue:     types.NewSafeListLimited[*prompb.TimeSeries](config.Config.WriterOpt.ChanSize),
	}

	go writers.LoopRead()
	return nil
}

func (ws Writers) LoopRead() {
	for {
		series := ws.queue.PopBackN(config.Config.WriterOpt.Batch)
		if len(series) == 0 {
			time.Sleep(time.Millisecond * 400)
			continue
		}

		items := make([]prompb.TimeSeries, len(series))
		for i := 0; i < len(series); i++ {
			items[i] = *series[i]
		}

		WriteTimeSeries(items)
	}
}

// WriteSample convert sample to prompb.TimeSeries and write to queue
// Note: Use WriteSamples for batch write for better performance
func WriteSample(sample *types.Sample) {
	if sample == nil {
		return
	}
	if config.Config.TestMode {
		printTestMetric(sample)
		return
	}
	if config.Config.DebugMode {
		printTestMetric(sample)
	}

	item := sample.ConvertTimeSeries(config.Config.Global.Precision)
	if item == nil || len(item.Labels) == 0 {
		return
	}
	writers.queue.PushFront(item)
}

// WriteSamples convert samples to []prompb.TimeSeries and batch write to queue
func WriteSamples(samples []*types.Sample) {
	if len(samples) == 0 {
		return
	}
	if config.Config.TestMode {
		printTestMetrics(samples)
		return
	}
	if config.Config.DebugMode {
		printTestMetrics(samples)
	}

	items := make([]*prompb.TimeSeries, 0, len(samples))
	for _, sample := range samples {
		item := sample.ConvertTimeSeries(config.Config.Global.Precision)
		if item == nil || len(item.Labels) == 0 {
			continue
		}
		items = append(items, item)
	}
	writers.queue.PushFrontN(items)
}

// WriteTimeSeries write prompb.TimeSeries to all writers
func WriteTimeSeries(timeSeries []prompb.TimeSeries) {
	if len(timeSeries) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	for key := range writers.writerMap {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			writers.writerMap[key].Write(timeSeries)
		}(key)
	}
	wg.Wait()
}

func printTestMetrics(samples []*types.Sample) {
	for _, sample := range samples {
		printTestMetric(sample)
	}
}

// printTestMetric print metric to stdout, only used in debug/test mode
func printTestMetric(sample *types.Sample) {
	var sb strings.Builder

	sb.WriteString(sample.Timestamp.Format("15:04:05"))
	sb.WriteString(" ")
	sb.WriteString(sample.Metric)

	arr := make([]string, 0, len(sample.Labels))
	for key, val := range sample.Labels {
		arr = append(arr, fmt.Sprintf("%s=%v", key, val))
	}

	sort.Strings(arr)

	for _, pair := range arr {
		sb.WriteString(" ")
		sb.WriteString(pair)
	}

	sb.WriteString(" ")
	sb.WriteString(fmt.Sprint(sample.Value))

	fmt.Println(sb.String())
}
