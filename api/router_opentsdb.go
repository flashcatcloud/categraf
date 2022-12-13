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

type Metric struct {
	Metric       string            `json:"metric"`
	Timestamp    int64             `json:"timestamp"`
	ValueUnTyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	Tags         map[string]string `json:"tags"`
}

func (m *Metric) Clean(ts int64) error {
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

func (m *Metric) ToProm() (*prompb.TimeSeries, error) {
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
		return nil, fmt.Errorf("invalid metric name: %s", m.Metric)
	}

	pt.Labels = append(pt.Labels, prompb.Label{
		Name:  model.MetricNameLabel,
		Value: m.Metric,
	})

	if _, exists := m.Tags["ident"]; !exists {
		// rename tag key
		host, has := m.Tags["host"]
		if has {
			delete(m.Tags, "host")
			m.Tags["ident"] = host
		}
	}

	for key, value := range m.Tags {
		if strings.IndexByte(key, '.') != -1 {
			key = strings.ReplaceAll(key, ".", "_")
		}

		if strings.IndexByte(key, '-') != -1 {
			key = strings.ReplaceAll(key, "-", "_")
		}

		if !model.LabelNameRE.MatchString(key) {
			return nil, fmt.Errorf("invalid tag name: %s", key)
		}

		pt.Labels = append(pt.Labels, prompb.Label{
			Name:  key,
			Value: value,
		})
	}

	return pt, nil
}

func openTSDB(c *gin.Context) {
	var (
		err   error
		bytes []byte
	)

	bytes, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	var list []Metric
	if bytes[0] == '[' {
		err = json.Unmarshal(bytes, &list)
	} else {
		var openTSDBMetric Metric
		err = json.Unmarshal(bytes, &openTSDBMetric)
		list = []Metric{openTSDBMetric}
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

	ignoreHostname := c.GetBool("ignore_hostname")
	ignoreGlobalLabels := c.GetBool("ignore_global_labels")
	count := len(list)
	series := make([]prompb.TimeSeries, 0, count)
	for i := 0; i < len(list); i++ {
		if err := list[i].Clean(ts); err != nil {
			log.Println("clean opentsdb sample:", err)
			if fail == 0 {
				msg = fmt.Sprintf("%s , Error clean: %s", msg, err.Error())
			}
			fail++
			continue
		}
		// add global labels
		if !ignoreGlobalLabels {
			for k, v := range config.Config.Global.Labels {
				if _, has := list[i].Tags[k]; has {
					continue
				}
				list[i].Tags[k] = v
			}
		}
		// add label: agent_hostname
		if _, has := list[i].Tags[agentHostnameLabelKey]; !has && !ignoreHostname {
			list[i].Tags[agentHostnameLabelKey] = config.Config.GetHostname()
		}

		pt, err := list[i].ToProm()
		if err != nil {
			log.Println("convert opentsdb sample:", err)
			if fail == 0 {
				msg = fmt.Sprintf("%s , Error toprom: %s", msg, err.Error())
			}
			fail++
			continue
		}

		series = append(series, *pt)
		succ++
	}

	if fail > 0 {
		log.Println("opentsdb forwarder error, message:", string(bytes))
	}

	writer.WriteTimeSeries(series)
	c.String(200, "succ:%d fail:%d message:%s", succ, fail, msg)
}
