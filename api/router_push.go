package api

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

const agentHostnameLabelKey = "agent_hostname"

type push struct {
}

type Context struct {
	*gin.Context
}

func NewContext(c *gin.Context) *Context {
	return &Context{c}
}

func (c *Context) Failed(code int, err error) {
	c.JSON(code, Response{
		Message: err.Error(),
	})
}

func (c *Context) Success(v ...any) {
	r := Response{
		Message: "success",
	}
	if len(v) > 0 {
		r.Data = v[0]
	}
	c.JSON(http.StatusOK, r)
}

func (push *push) OpenTSDB(c *gin.Context) {
	var (
		err   error
		bytes []byte
	)

	cc := NewContext(c)
	bytes, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		cc.Failed(http.StatusBadRequest, err)
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
		cc.Failed(http.StatusBadRequest, err)
		return
	}

	var (
		fail    int
		success int
		msg     = "data pushed to queue"
		ts      = time.Now().Unix()
	)

	ignoreHostname := c.GetBool("ignore_hostname")
	ignoreGlobalLabels := c.GetBool("ignore_global_labels")
	count := len(list)
	series := make([]prompb.TimeSeries, 0, count)
	for i := 0; i < len(list); i++ {
		if err := list[i].Clean(ts); err != nil {
			log.Printf("opentsdb msg clean error: %s\n", err.Error())
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
			log.Printf("opentsdb msg to tsdb error: %s\n", err.Error())
			if fail == 0 {
				msg = fmt.Sprintf("%s , Error toprom: %s", msg, err.Error())
			}
			fail++
			continue
		}

		series = append(series, *pt)
		success++
	}

	if fail > 0 {
		log.Printf("opentsdb msg process error , msg is : %s\n", string(bytes))
	}

	writer.PostTimeSeries(series)
	cc.Success(map[string]interface{}{
		"success": success,
		"fail":    fail,
		"msg":     msg,
	})
}

func (push *push) falcon(c *gin.Context) {
	var (
		err   error
		bytes []byte
	)

	cc := NewContext(c)
	bytes, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		cc.Failed(http.StatusBadRequest, err)
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
		cc.Failed(http.StatusBadRequest, err)
		return
	}

	var (
		fail    int
		success int
		msg     = "data pushed to queue"
		ts      = time.Now().Unix()
	)

	ignoreHostname := c.GetBool("ignore_hostname")
	ignoreGlobalLabels := c.GetBool("ignore_global_labels")
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
			for k, v := range config.Config.Global.Labels {
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

		writer.PushQueue(types.TimeSeriesConvertSample(pt))
		series = append(series, *pt)
		success++
	}

	if fail > 0 {
		log.Printf("falconmetric msg process error , msg is : %s\n", string(bytes))
	}

	writer.PostTimeSeries(series)

	cc.Success(map[string]interface{}{
		"success": success,
		"fail":    fail,
		"msg":     msg,
	})
}

func (push *push) remoteWrite(c *gin.Context) {
	cc := NewContext(c)
	req, err := DecodeWriteRequest(c.Request.Body)
	if err != nil {
		cc.Failed(http.StatusBadRequest, err)
		return
	}

	count := len(req.Timeseries)
	if count == 0 {
		cc.Success()
		return
	}

	ignoreHostname := c.GetBool("ignore_hostname")
	ignoreGlobalLabels := c.GetBool("ignore_global_labels")
	for i := 0; i < count; i++ {
		// 去除重复的数据
		if duplicateLabelKey(req.Timeseries[i]) {
			continue
		}

		tags := make(map[string]string)
		for _, label := range req.Timeseries[i].Labels {
			tags[label.Name] = label.Value
		}
		// add global labels
		if !ignoreGlobalLabels {
			for k, v := range config.Config.Global.Labels {
				if _, has := tags[k]; has {
					continue
				}
				req.Timeseries[i].Labels = append(req.Timeseries[i].Labels, prompb.Label{Name: k, Value: v})
			}
		}
		// add label: agent_hostname
		if _, has := tags[agentHostnameLabelKey]; !has && !ignoreHostname {
			req.Timeseries[i].Labels = append(req.Timeseries[i].Labels, prompb.Label{Name: agentHostnameLabelKey, Value: config.Config.GetHostname()})
		}
	}

	writer.PostTimeSeries(req.Timeseries)

	cc.Success()
}

func duplicateLabelKey(series prompb.TimeSeries) bool {
	labelKeys := make(map[string]struct{})

	for j := 0; j < len(series.Labels); j++ {
		if _, has := labelKeys[series.Labels[j].Name]; has {
			return true
		} else {
			labelKeys[series.Labels[j].Name] = struct{}{}
		}
	}

	return false
}

func readerGzipBody(contentEncoding string, request *http.Request) (bytes []byte, err error) {
	if contentEncoding == "gzip" {
		var (
			r *gzip.Reader
		)
		r, err = gzip.NewReader(request.Body)
		if err != nil {
			return nil, err
		}

		defer r.Close()
		bytes, err = ioutil.ReadAll(r)
	} else {
		defer request.Body.Close()
		bytes, err = ioutil.ReadAll(request.Body)
	}
	if err != nil || len(bytes) == 0 {
		return nil, errors.New("request parameter error")
	}

	return bytes, nil
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}
