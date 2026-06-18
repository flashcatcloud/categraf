//go:build !no_logs

package categraf

import (
	"strconv"
	"time"

	config "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/logs/auditor"
	"flashcat.cloud/categraf/types"
)

// reserved label keys that must not be overridden by user tags
var reservedLabels = map[string]bool{
	"path": true, "source_type": true, "tailing_mode": true,
	"source": true, "service": true,
}

func gatherLogMetrics(slist *types.SampleList) {
	stat := auditor.GetCollectorStatus()

	running := 0
	if stat.Running {
		running = 1
	}

	pipelines := config.NumberOfPipelines()

	slist.PushSample(defaultPrefix, "log_agent_running", running)
	slist.PushSample(defaultPrefix, "log_agent_pipelines", pipelines)
	slist.PushSample(defaultPrefix, "log_files_tracked_total", stat.TrackedFiles)

	for _, e := range stat.Entries {
		tags := map[string]string{
			"path":         e.Path,
			"source_type":  e.SourceType,
			"tailing_mode": e.TailingMode,
		}
		if e.Source != "" {
			tags["source"] = e.Source
		}
		if e.Service != "" {
			tags["service"] = e.Service
		}

		// merge user-defined item tags (skip reserved keys)
		for k, v := range e.Tags {
			if !reservedLabels[k] {
				tags[k] = v
			}
		}

		if offsetVal, err := strconv.ParseInt(e.Offset, 10, 64); err == nil {
			slist.PushSample(defaultPrefix, "log_source_offset", offsetVal, tags)
		}
		ageSec := time.Since(e.LastUpdated).Seconds()
		slist.PushSample(defaultPrefix, "log_source_last_updated_seconds", ageSec, tags)
	}
}
