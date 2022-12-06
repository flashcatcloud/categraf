package writer

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
)

type Writers struct {
	writerMap map[string]WriterType
	queue     MetricQueue
}

var writers Writers

func InitWriters() error {
	writerMap := map[string]WriterType{}
	opts := config.Config.Writers
	for i := 0; i < len(opts); i++ {
		cli, err := api.NewClient(api.Config{
			Address: opts[i].Url,
			RoundTripper: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: time.Duration(opts[i].DialTimeout) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(opts[i].Timeout) * time.Millisecond,
				MaxIdleConnsPerHost:   opts[i].MaxIdleConnsPerHost,
			},
		})

		if err != nil {
			return err
		}

		writer := WriterType{
			Opts:   opts[i],
			Client: cli,
		}
		writerMap[opts[i].Url] = writer
	}

	writers = Writers{
		writerMap: writerMap,
		queue:     newMetricQueue(config.Config.WriterOpt.ChanSize, WriteTimeSeries),
	}

	go writers.queue.LoopRead()
	return nil
}

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
