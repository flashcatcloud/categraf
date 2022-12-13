package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
)

func pushgateway(c *gin.Context) {
	var (
		err error
		bs  []byte
	)

	bs, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	parser := prometheus.EmptyParser()
	slist := types.NewSampleList()
	if err = parser.Parse(bs, slist); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	samples := slist.PopBackAll()
	count := len(samples)
	if count == 0 {
		c.String(http.StatusBadRequest, "no valid samples")
		return
	}

	ignoreHostname := c.GetBool("ignore_hostname")
	ignoreGlobalLabels := c.GetBool("ignore_global_labels")

	now := time.Now()

	for i := 0; i < count; i++ {
		// handle timestamp
		if samples[i].Timestamp.IsZero() {
			samples[i].Timestamp = now
		}

		// add global labels
		if !ignoreGlobalLabels {
			for k, v := range config.Config.Global.Labels {
				if _, has := samples[i].Labels[k]; has {
					continue
				}
				samples[i].Labels[k] = v
			}
		}

		// add label: agent_hostname
		if _, has := samples[i].Labels[agentHostnameLabelKey]; !has && !ignoreHostname {
			samples[i].Labels[agentHostnameLabelKey] = config.Config.GetHostname()
		}

	}
	writer.WriteSamples(samples)
	c.String(200, "forwarding...")
}
