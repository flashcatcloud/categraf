package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/writer"
)

func remoteWrite(c *gin.Context) {
	req, err := DecodeWriteRequest(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	count := len(req.Timeseries)
	if count == 0 {
		c.String(http.StatusBadRequest, "payload empty")
		return
	}

	ignoreHostname := config.Config.HTTP.IgnoreHostname || QueryBoolWithValues("ignore_hostname")(c)
	ignoreGlobalLabels := config.Config.HTTP.IgnoreGlobalLabels || QueryBoolWithValues("ignore_global_labels")(c)
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
			for k, v := range config.GlobalLabels() {
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

	writer.WriteTimeSeries(req.Timeseries)
	c.String(200, "forwarding...")
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
