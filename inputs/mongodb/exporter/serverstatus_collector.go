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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type serverStatusCollector struct {
	ctx  context.Context
	base *baseCollector

	compatibleMode bool
	topologyInfo   labelsGetter
}

// newServerStatusCollector creates a collector for statistics on server status.
func newServerStatusCollector(ctx context.Context, client *mongo.Client, logger *logrus.Logger, compatible bool, topology labelsGetter) *serverStatusCollector {
	return &serverStatusCollector{
		ctx:            ctx,
		base:           newBaseCollector(client, logger),
		compatibleMode: compatible,
		topologyInfo:   topology,
	}
}

func (d *serverStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	d.base.Describe(d.ctx, ch, d.collect)
}

func (d *serverStatusCollector) Collect(ch chan<- prometheus.Metric) {
	d.base.Collect(ch, d.collect)
}

func (d *serverStatusCollector) collect(ch chan<- prometheus.Metric) {

	logger := d.base.logger
	client := d.base.client

	cmd := bson.D{{Key: "serverStatus", Value: "1"}}
	res := client.Database("admin").RunCommand(d.ctx, cmd)

	var m bson.M
	if err := res.Decode(&m); err != nil {
		ch <- prometheus.NewInvalidMetric(prometheus.NewInvalidDesc(err), err)
		return
	}

	logrus.Debug("serverStatus result:")
	debugResult(logger, m)

	for _, metric := range makeMetrics("", m, d.topologyInfo.baseLabels(), d.compatibleMode) {
		ch <- metric
	}
}
