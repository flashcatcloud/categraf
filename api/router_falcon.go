package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/writer"
)

type FalconMetric struct {
	Metric       string      `json:"metric"`
	Endpoint     string      `json:"endpoint"`
	Timestamp    int64       `json:"timestamp"`
	ValueUnTyped interface{} `json:"value"`
	Value        float64     `json:"-"`
	Tags         string      `json:"tags"`
}

func (m *FalconMetric) Clean(ts int64) error {
	if m.Metric == "" {
		return fmt.Errorf("metric is blank")
	}

	switch v := m.ValueUnTyped.(type) {
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.Value = f
		} else {
			return fmt.Errorf("unparseable value %v", v)
		}
	case float64:
		m.Value = v
	case uint64:
		m.Value = float64(v)
	case int64:
		m.Value = float64(v)
	case int:
		m.Value = float64(v)
	default:
		return fmt.Errorf("unparseable value %v", v)
	}

	// if timestamp bigger than 32 bits, likely in milliseconds
	if m.Timestamp > 0xffffffff {
		m.Timestamp /= 1000
	}

	// If the timestamp is greater than 5 minutes, the current time shall prevail
	diff := m.Timestamp - ts
	if diff > 300 {
		m.Timestamp = ts
	}
	return nil
}

func (m *FalconMetric) ToProm() (*prompb.TimeSeries, string, error) {
	pt := &prompb.TimeSeries{}
	pt.Samples = append(pt.Samples, prompb.Sample{
		// use ms
		Timestamp: m.Timestamp * 1000,
		Value:     m.Value,
	})

	if strings.IndexByte(m.Metric, '.') != -1 {
		m.Metric = strings.ReplaceAll(m.Metric, ".", "_")
	}

	if strings.IndexByte(m.Metric, '-') != -1 {
		m.Metric = strings.ReplaceAll(m.Metric, "-", "_")
	}

	if !model.MetricNameRE.MatchString(m.Metric) {
		return nil, "", fmt.Errorf("invalid metric name: %s", m.Metric)
	}

	pt.Labels = append(pt.Labels, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: m.Metric,
	})

	tagarr := strings.Split(m.Tags, ",")
	tagmap := make(map[string]string, len(tagarr)+1)

	for i := 0; i < len(tagarr); i++ {
		tmp := strings.SplitN(tagarr[i], "=", 2)
		if len(tmp) != 2 {
			continue
		}

		tagmap[tmp[0]] = tmp[1]
	}

	ident := ""

	if len(m.Endpoint) > 0 {
		ident = m.Endpoint
		if id, exists := tagmap["ident"]; exists {
			ident = id
			// 以tags中的ident作为唯一标识
			tagmap["endpoint"] = m.Endpoint
		} else {
			// 把endpoint塞到tags中，改key为ident
			tagmap["ident"] = m.Endpoint
		}
	}

	for key, value := range tagmap {
		if strings.IndexByte(key, '.') != -1 {
			key = strings.ReplaceAll(key, ".", "_")
		}

		if strings.IndexByte(key, '-') != -1 {
			key = strings.ReplaceAll(key, "-", "_")
		}

		if !model.LabelNameRE.MatchString(key) {
			return nil, "", fmt.Errorf("invalid tag name: %s", key)
		}

		pt.Labels = append(pt.Labels, prompb.Label{
			Name:  key,
			Value: value,
		})
	}

	return pt, ident, nil
}

func openFalcon(c *gin.Context) {
	var (
		err   error
		bytes []byte
	)

	bytes, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	var arr []FalconMetric
	if bytes[0] == '[' {
		err = json.Unmarshal(bytes, &arr)
	} else {
		var one FalconMetric
		err = json.Unmarshal(bytes, &one)
		arr = []FalconMetric{one}
	}
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	var (
		fail int
		succ int
		msg  = "data pushed to queue"
		ts   = time.Now().Unix()
	)

	ignoreHostname := config.Config.HTTP.IgnoreHostname || QueryBoolWithValues("ignore_hostname")(c)
	ignoreGlobalLabels := config.Config.HTTP.IgnoreGlobalLabels || QueryBoolWithValues("ignore_global_labels")(c)
	count := len(arr)
	series := make([]prompb.TimeSeries, 0, count)
	for i := 0; i < count; i++ {
		if err := arr[i].Clean(ts); err != nil {
			fail++
			continue
		}

		pt, _, err := arr[i].ToProm()
		if err != nil {
			fail++
			continue
		}

		tags := make(map[string]string)
		for _, label := range pt.Labels {
			tags[label.Name] = label.Value
		}
		// add global labels
		if !ignoreGlobalLabels {
			for k, v := range config.GlobalLabels() {
				if _, has := tags[k]; has {
					continue
				}
				pt.Labels = append(pt.Labels, prompb.Label{Name: k, Value: v})
			}
		}
		// add label: agent_hostname
		if _, has := tags[agentHostnameLabelKey]; !has && !ignoreHostname {
			pt.Labels = append(pt.Labels, prompb.Label{Name: agentHostnameLabelKey, Value: config.Config.GetHostname()})
		}

		series = append(series, *pt)
		succ++
	}

	if fail > 0 {
		log.Println("falcon forwarder error, message:", string(bytes))
	}

	writer.WriteTimeSeries(series)
	c.String(200, "succ:%d fail:%d message:%s", succ, fail, msg)
}
