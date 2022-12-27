// Copyright 2015 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package exporter

import (
	"expvar"
	"fmt"
	"os"
	"time"

	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
)

var (
	statsdHostPort = os.Getenv("STATSD_HOSTPORT")
	statsdPrefix   = os.Getenv("STATSD_PREFIX")

	statsdExportTotal   = expvar.NewInt("statsd_export_total")
	statsdExportSuccess = expvar.NewInt("statsd_export_success")
)

// metricToStatsd encodes a metric in the statsd text protocol format.  The
// metric lock is held before entering this function.
func metricToStatsd(hostname string, m *metrics.Metric, l *metrics.LabelSet, _ time.Duration) string {
	var t string
	switch m.Kind {
	case metrics.Counter:
		t = "c" // StatsD Counter
	case metrics.Gauge:
		t = "g" // StatsD Gauge
	case metrics.Timer:
		t = "ms" // StatsD Timer
	}
	return fmt.Sprintf("%s%s.%s:%s|%s",
		statsdPrefix,
		m.Program,
		formatLabels(m.Name, l.Labels, ".", ".", "_"),
		l.Datum.ValueString(), t)
}
