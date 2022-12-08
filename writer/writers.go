package writer

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

// Writers manage all writers and metric queue
type Writers struct {
	writerMap map[string]WriterType
	queue     MetricQueue
}

var writers Writers

func InitWriters() error {
	writerMap := map[string]WriterType{}
	opts := config.Config.Writers
	for _, opt := range opts {
		writer, err := newWriteType(opt)
		if err != nil {
			return err
		}
		writerMap[opt.Url] = *writer
	}
	writers = Writers{
		writerMap: writerMap,
		queue:     newMetricQueue(config.Config.WriterOpt.ChanSize, WriteTimeSeries),
	}

	go writers.queue.LoopRead()
	return nil
}

// WriteSample convert sample to prompb.TimeSeries and write to queue
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
	writers.queue.Push(item)
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
