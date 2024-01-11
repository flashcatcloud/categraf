// mongodb_exporter
// Copyright (C) 2017 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type diagnosticDataCollector struct {
	ctx  context.Context
	base *baseCollector

	compatibleMode bool
	topologyInfo   labelsGetter
}

// newDiagnosticDataCollector creates a collector for diagnostic information.
func newDiagnosticDataCollector(ctx context.Context, client *mongo.Client, logger *logrus.Logger, compatible bool, topology labelsGetter) *diagnosticDataCollector {
	return &diagnosticDataCollector{
		ctx:  ctx,
		base: newBaseCollector(client, logger),

		compatibleMode: compatible,
		topologyInfo:   topology,
	}
}

func (d *diagnosticDataCollector) Describe(ch chan<- *prometheus.Desc) {
	d.base.Describe(d.ctx, ch, d.collect)
}

func (d *diagnosticDataCollector) Collect(ch chan<- prometheus.Metric) {
	d.base.Collect(ch, d.collect)
}

func (d *diagnosticDataCollector) collect(ch chan<- prometheus.Metric) {

	var m bson.M

	logger := d.base.logger
	client := d.base.client

	cmd := bson.D{{Key: "getDiagnosticData", Value: "1"}}
	res := client.Database("admin").RunCommand(d.ctx, cmd)
	if res.Err() != nil {
		if isArbiter, _ := isArbiter(d.ctx, client); isArbiter {
			return
		}
	}

	if err := res.Decode(&m); err != nil {
		logger.Errorf("cannot run getDiagnosticData: %s", err)
	}

	if m == nil || m["data"] == nil {
		logger.Error("cannot run getDiagnosticData: response is empty")
	}

	m, ok := m["data"].(bson.M)
	if !ok {
		err := errors.Wrapf(errUnexpectedDataType, "%T for data field", m["data"])
		logger.Errorf("cannot decode getDiagnosticData: %s", err)
	}

	logger.Debug("getDiagnosticData result")
	debugResult(logger, m)

	metrics := makeMetrics("", m, d.topologyInfo.baseLabels(), d.compatibleMode)
	metrics = append(metrics, locksMetrics(m)...)
	metrics = append(metrics, specialMetrics(d.ctx, client, m, logger)...)
	if cem, err := cacheEvictedTotalMetric(m); err == nil {
		metrics = append(metrics, cem)
	}

	if d.compatibleMode {
		nodeType, err := getNodeType(d.ctx, client)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"component": "diagnosticDataCollector",
			}).Errorf("Cannot get node type to check if this is a mongos: %s", err)
		} else if nodeType == typeMongos {
			metrics = append(metrics, mongosMetrics(d.ctx, client, logger)...)
		}
	}

	for _, metric := range metrics {
		ch <- metric
	}
}

// check interface.
var _ prometheus.Collector = (*diagnosticDataCollector)(nil)
