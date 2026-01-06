package inputs

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	util "flashcat.cloud/categraf/pkg/metrics"
	"flashcat.cloud/categraf/types"
)

const capMetricChan = 1000

func Collect(e prometheus.Collector, slist *types.SampleList, constLabels ...map[string]string) error {
	if e == nil {
		return errors.New("exporter must not be nil")
	}

	metricChan := make(chan prometheus.Metric, capMetricChan)
	go func() {
		e.Collect(metricChan)
		close(metricChan)
	}()

	for metric := range metricChan {
		if metric == nil {
			continue
		}

		desc := metric.Desc().String()
		descName, err := DescName(desc)
		if err != nil {
			log.Printf("error getting metric name: %s", err)
			continue
		}
		ls, err := DescConstLabels(desc)
		if err != nil {
			log.Println("E! failed to read labels:", desc)
			continue
		}

		dtoMetric := &dto.Metric{}
		err = metric.Write(dtoMetric)
		if err != nil {
			log.Println("E! failed to write metric:", desc)
			continue
		}

		labels := map[string]string{}
		for k, v := range ls {
			labels[k] = v
		}

		for _, kv := range dtoMetric.Label {
			labels[*kv.Name] = *kv.Value
		}

		for _, kvs := range constLabels {
			for k, v := range kvs {
				labels[k] = v
			}
		}

		switch {
		case dtoMetric.Counter != nil:
			slist.PushSample("", descName, *dtoMetric.Counter.Value, labels)
		case dtoMetric.Gauge != nil:
			slist.PushSample("", descName, *dtoMetric.Gauge.Value, labels)
		case dtoMetric.Summary != nil:
			util.HandleSummary("", dtoMetric, nil, descName, nil, slist)
		case dtoMetric.Histogram != nil:
			util.HandleHistogram("", dtoMetric, nil, descName, nil, slist)
		default:
			slist.PushSample("", descName, *dtoMetric.Untyped.Value, labels)
		}
	}

	return nil
}

func DescName(descStr string) (string, error) {
	// 使用正则表达式匹配fqName部分
	re := regexp.MustCompile(`fqName: "([^"]*)"`)
	matches := re.FindStringSubmatch(descStr)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to extract fqName from desc string")
	}
	return matches[1], nil
}

func DescConstLabels(descStr string) (map[string]string, error) {
	// 使用正则表达式匹配constLabels部分
	re := regexp.MustCompile(`constLabels: {([^}]*)}`)
	matches := re.FindStringSubmatch(descStr)
	if len(matches) < 2 {
		return nil, fmt.Errorf("failed to extract constLabels from desc string")
	}

	labels := make(map[string]string)
	// 如果constLabels为空，直接返回空map
	if matches[1] == "" {
		return labels, nil
	}

	// 分割多个label对
	pairs := strings.Split(matches[1], ",")
	for _, pair := range pairs {
		// 使用正则表达式匹配每个label对中的name和value
		pairRe := regexp.MustCompile(`(\w+)="([^"]*)"`)
		pairMatches := pairRe.FindStringSubmatch(pair)
		if len(pairMatches) < 3 {
			return nil, fmt.Errorf("invalid label pair format: %s", pair)
		}
		labels[pairMatches[1]] = pairMatches[2]
	}

	return labels, nil
}
