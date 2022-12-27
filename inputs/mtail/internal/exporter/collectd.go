// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package exporter

import (
	"expvar"
	"fmt"
	"os"
	"strings"
	"time"

	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
)

const (
	// See https://collectd.org/wiki/index.php/Plain_text_protocol#PUTVAL
	collectdFormat = "PUTVAL \"%s/%smtail-%s/%s-%s\" interval=%d %s:%s\n"
)

var (
	collectdSocketPath = os.Getenv("COLLECTD_SOCKETPATH")
	collectdPrefix     = os.Getenv("COLLECTD_PREFIX")

	collectdExportTotal   = expvar.NewInt("collectd_export_total")
	collectdExportSuccess = expvar.NewInt("collectd_export_success")
)

// metricToCollectd encodes the metric data in the collectd text protocol format.  The
// metric lock is held before entering this function.
func metricToCollectd(hostname string, m *metrics.Metric, l *metrics.LabelSet, interval time.Duration) string {
	return fmt.Sprintf(collectdFormat,
		hostname,
		collectdPrefix,
		m.Program,
		kindToCollectdType(m.Kind),
		formatLabels(m.Name, l.Labels, "-", "-", "_"),
		int64(interval.Seconds()),
		l.Datum.TimeString(),
		l.Datum.ValueString())
}

func kindToCollectdType(kind metrics.Kind) string {
	if kind != metrics.Timer {
		return strings.ToLower(kind.String())
	}
	return "gauge"
}
