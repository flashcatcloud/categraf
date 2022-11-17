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

var Writers = make(map[string]WriterType)

func InitWriters() error {
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

		Writers[opts[i].Url] = writer
	}

	initQueue()

	return nil
}

func postSeries(samples []*types.Sample) {
	if config.Config.TestMode {
		printTestMetrics(samples)
		return
	} else {
		if config.Config.DebugMode {
			printTestMetrics(samples)
		}
	}

	count := len(samples)
	series := make([]prompb.TimeSeries, 0, count)
	for i := 0; i < count; i++ {
		item := samples[i].ConvertTimeSeries(config.Config.Global.Precision)
		if item == nil || len(item.Labels) == 0 {
			continue
		}

		series = append(series, *item)
	}

	PostTimeSeries(series)
}

func PostTimeSeries(timeSeries []prompb.TimeSeries) {
	if len(timeSeries) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	for key := range Writers {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			Writers[key].Write(timeSeries)
		}(key)
	}
	wg.Wait()
}

func printTestMetrics(samples []*types.Sample) {
	for i := 0; i < len(samples); i++ {
		var sb strings.Builder

		sb.WriteString(samples[i].Timestamp.Format("15:04:05"))
		sb.WriteString(" ")
		sb.WriteString(samples[i].Metric)

		arr := make([]string, 0, len(samples[i].Labels))
		for key, val := range samples[i].Labels {
			arr = append(arr, fmt.Sprintf("%s=%v", key, val))
		}

		sort.Strings(arr)

		for _, pair := range arr {
			sb.WriteString(" ")
			sb.WriteString(pair)
		}

		sb.WriteString(" ")
		sb.WriteString(fmt.Sprint(samples[i].Value))

		fmt.Println(sb.String())
	}
}
