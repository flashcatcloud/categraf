// Copyright 2016 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package golden

import (
	"bufio"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
	"flashcat.cloud/categraf/inputs/mtail/internal/metrics/datum"
)

var varRe = regexp.MustCompile(`^(counter|gauge|timer|text|histogram) ([^ ]+)(?: {([^}]+)})?(?: (\S+))?(?: (.+))?`)

// ReadTestData loads a "golden" test data file from a programfile and returns as a slice of Metrics.
func ReadTestData(file io.Reader, programfile string) metrics.MetricSlice {
	store := metrics.NewStore()
	prog := filepath.Base(programfile)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := varRe.FindStringSubmatch(scanner.Text())
		if len(match) == 0 {
			continue
		}
		keys := make([]string, 0)
		vals := make([]string, 0)
		if match[3] != "" {
			for _, pair := range strings.Split(match[3], ",") {
				kv := strings.Split(pair, "=")
				keys = append(keys, kv[0])
				if kv[1] != "" {
					if kv[1] == `""` {
						vals = append(vals, "")
					} else {
						vals = append(vals, kv[1])
					}
				}
			}
		}
		var kind metrics.Kind
		switch match[1] {
		case "counter":
			kind = metrics.Counter
		case "gauge":
			kind = metrics.Gauge
		case "timer":
			kind = metrics.Timer
		case "text":
			kind = metrics.Text
		case "histogram":
			kind = metrics.Histogram
		}
		typ := metrics.Int
		var (
			ival int64
			fval float64
			sval string
			err  error
		)
		if match[4] != "" {
			ival, err = strconv.ParseInt(match[4], 10, 64)
			if err != nil {
				fval, err = strconv.ParseFloat(match[4], 64)
				typ = metrics.Float
				if err != nil || fval == 0.0 {
					sval = match[4]
					typ = metrics.String
				}
			}
		}
		var timestamp time.Time
		if match[5] != "" {
			timestamp, err = time.Parse(time.RFC3339, match[5])
			if err != nil {
				j, err := strconv.ParseInt(match[5], 10, 64)
				if err == nil {
					timestamp = time.Unix(j/1000000000, j%1000000000)
				} else {
					log.Println(err)
				}
			}
		}

		// Now we have enough information to get or create a metric.
		m := store.FindMetricOrNil(match[2], prog)
		if m != nil {
			if m.Type != typ {
				log.Printf("The type of the fetched metric is not %s: %s", typ, m)
				continue
			}
		} else {
			m = metrics.NewMetric(match[2], prog, kind, typ, keys...)
			if kind == metrics.Counter && len(keys) == 0 {
				d, err := m.GetDatum()
				if err != nil {
					log.Fatal(err)
				}
				// Initialize to zero at the zero time.
				switch typ {
				case metrics.Int:
					datum.SetInt(d, 0, time.Unix(0, 0))
				case metrics.Float:
					datum.SetFloat(d, 0, time.Unix(0, 0))
				}
			}
			if err := store.Add(m); err != nil {
				log.Printf("Failed to add metric %v to store: %s", m, err)
			}
		}

		if match[4] != "" {
			d, err := m.GetDatum(vals...)
			if err != nil {
				log.Printf("Failed to get datum: %s", err)
				continue
			}

			switch typ {
			case metrics.Int:
				datum.SetInt(d, ival, timestamp)
			case metrics.Float:
				datum.SetFloat(d, fval, timestamp)
			case metrics.String:
				datum.SetString(d, sval, timestamp)
			}
		}
	}

	storeList := make([]*metrics.Metric, 0)
	/* #nosec G104 -- Always returns nil. nolint:errcheck */
	store.Range(func(m *metrics.Metric) error {
		storeList = append(storeList, m)
		return nil
	})
	return storeList
}
