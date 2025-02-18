package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
)

const (
	// Base64Suffix is appended to a label name in the request URL path to
	// mark the following label value as base64 encoded.
	Base64Suffix = "@base64"
)

func pushgateway(c *gin.Context) {
	var (
		err    error
		bs     []byte
		labels map[string]string
	)

	bs, err = readerGzipBody(c.GetHeader("Content-Encoding"), c.Request)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	// jobtype {"", "job", "job@base64"}
	jobType := c.Param("jobtype")
	job := c.Param("job")
	if jobType == "job"+Base64Suffix {
		var err error
		if job, err = decodeBase64(job); err != nil {
			c.String(http.StatusBadRequest, "invalid base64 encoding in job name %q: %v", job, err)
			return
		}
	}
	labelsString := c.Param("labels")
	if labels, err = splitLabels(labelsString); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if job != "" {
		labels["job"] = job
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

	ignoreHostname := config.Config.HTTP.IgnoreHostname || QueryBoolWithValues("ignore_hostname")(c)
	ignoreGlobalLabels := config.Config.HTTP.IgnoreGlobalLabels || QueryBoolWithValues("ignore_global_labels")(c)
	// 获取 AgentHostTag 的值
	agentHostTag := config.Config.HTTP.AgentHostTag
	if agentHostTag == "" {
		agentHostTag = c.GetString("agent_host_tag")
	}

	now := time.Now()

	for i := 0; i < count; i++ {
		// handle timestamp
		if samples[i].Timestamp.IsZero() {
			samples[i].Timestamp = now
		}

		// add global labels
		if !ignoreGlobalLabels {
			for k, v := range config.GlobalLabels() {
				if _, has := samples[i].Labels[k]; has {
					continue
				}
				samples[i].Labels[k] = v
			}
		}

		// add url labels
		for k, v := range labels {
			samples[i].Labels[k] = v

		}

		// add label: agent_hostname
		if !ignoreHostname {
			if agentHostTag == "" {
				if _, has := samples[i].Labels[agentHostnameLabelKey]; !has {
					samples[i].Labels[agentHostnameLabelKey] = config.Config.GetHostname()
				}
			} else {
				// 从当前现有的 Labels 中找到 key 等于 config.Config.HTTP.AgentHostTag 的值
				if value, exists := samples[i].Labels[agentHostTag]; exists {
					samples[i].Labels[agentHostnameLabelKey] = value
				}
			}
		}
	}
	writer.WriteSamples(samples)
	c.String(http.StatusOK, "forwarding...")
}

// fork prometheus/pushgateway handler/push.go

// decodeBase64 decodes the provided string using the “Base 64 Encoding with URL
// and Filename Safe Alphabet” (RFC 4648). Padding characters (i.e. trailing
// '=') are ignored.
func decodeBase64(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	return string(b), err
}

// splitLabels splits a labels string into a label map mapping names to values.
func splitLabels(labels string) (map[string]string, error) {
	result := map[string]string{}
	if len(labels) <= 1 {
		return result, nil
	}
	components := strings.Split(labels[1:], "/")
	if len(components)%2 != 0 {
		return nil, fmt.Errorf("odd number of components in label string %q", labels)
	}

	for i := 0; i < len(components)-1; i += 2 {
		name, value := components[i], components[i+1]
		trimmedName := strings.TrimSuffix(name, Base64Suffix)
		if !model.LabelNameRE.MatchString(trimmedName) ||
			strings.HasPrefix(trimmedName, model.ReservedLabelPrefix) {
			return nil, fmt.Errorf("improper label name %q", trimmedName)
		}
		if name == trimmedName {
			result[name] = value
			continue
		}
		decodedValue, err := decodeBase64(value)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 encoding for label %s=%q: %v", trimmedName, value, err)
		}
		result[trimmedName] = decodedValue
	}
	return result, nil
}
