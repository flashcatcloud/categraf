package api

import (
	"net/http"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/house"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
	"github.com/gin-gonic/gin"
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

	parser := prometheus.NewParser("", map[string]string{}, nil, nil, nil)
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

	for i := 0; i < count; i++ {
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

		writer.PushQueue(samples[i])
		house.MetricsHouse.Push(samples[i])
	}

	c.String(200, "forwarding...")
}
