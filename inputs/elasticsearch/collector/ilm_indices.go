// Copyright 2023 The Prometheus Authors
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

package collector

import (
	"encoding/json"
	"flashcat.cloud/categraf/pkg/filter"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type ilmMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(timeMillis float64) float64
	Labels []string
}

// Index Lifecycle Management information object
type IlmIndiciesCollector struct {
	client               *http.Client
	url                  *url.URL
	indicesIncluded      []string
	numMostRecentIndices int
	indexMatchers        map[string]filter.Filter
	ilmMetric            ilmMetric
}

type IlmResponse struct {
	Indices map[string]IlmIndexResponse `json:"indices"`
}

type IlmIndexResponse struct {
	Index          string  `json:"index"`
	Managed        bool    `json:"managed"`
	Phase          string  `json:"phase"`
	Action         string  `json:"action"`
	Step           string  `json:"step"`
	StepTimeMillis float64 `json:"step_time_millis"`
}

var (
	defaultIlmIndicesMappingsLabels = []string{"index", "phase", "action", "step"}
)

// NewIlmIndicies defines Index Lifecycle Management Prometheus metrics
func NewIlmIndicies(client *http.Client, url *url.URL, indicesIncluded []string, numMostRecentIndices int, indexMatchers map[string]filter.Filter) *IlmIndiciesCollector {
	subsystem := "ilm_index"

	return &IlmIndiciesCollector{
		client:               client,
		url:                  url,
		indicesIncluded:      indicesIncluded,
		numMostRecentIndices: numMostRecentIndices,
		indexMatchers:        indexMatchers,

		ilmMetric: ilmMetric{
			Type: prometheus.GaugeValue,
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "status"),
				"Status of ILM policy for index",
				defaultIlmIndicesMappingsLabels, nil),
			Value: func(timeMillis float64) float64 {
				return timeMillis
			},
		},
	}
}

// Describe adds metrics description
func (i *IlmIndiciesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- i.ilmMetric.Desc
}

func (i *IlmIndiciesCollector) fetchAndDecodeIlm() (IlmResponse, error) {
	var ir IlmResponse

	u := *i.url
	//add indices filter
	if len(i.indicesIncluded) == 0 {
		u.Path = path.Join(u.Path, "/_all/_ilm/explain")
	} else {
		u.Path = path.Join(u.Path, "/"+strings.Join(i.indicesIncluded, ",")+"/_ilm/explain")
	}

	res, err := i.client.Get(u.String())
	if err != nil {
		return ir, fmt.Errorf("failed to get index stats from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return ir, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return ir, err
	}

	if err := json.Unmarshal(bts, &ir); err != nil {
		return ir, err
	}

	return ir, nil
}

func bool2int(managed bool) float64 {
	if managed {
		return 1
	}
	return 0
}

// Collect pulls metric values from Elasticsearch
func (i *IlmIndiciesCollector) Collect(ch chan<- prometheus.Metric) {
	// indices
	ilmResp, err := i.fetchAndDecodeIlm()
	if err != nil {
		log.Println("failed to fetch and decode ILM stats, err: ", err)
		return
	}

	//add config i.numMostRecentIndices process code
	ilmResp = i.gatherIndividualIndicesStats(ilmResp)

	for indexName, indexIlm := range ilmResp.Indices {
		ch <- prometheus.MustNewConstMetric(
			i.ilmMetric.Desc,
			i.ilmMetric.Type,
			i.ilmMetric.Value(bool2int(indexIlm.Managed)),
			indexName, indexIlm.Phase, indexIlm.Action, indexIlm.Step,
		)
	}
}

func (i *IlmIndiciesCollector) gatherIndividualIndicesStats(resp IlmResponse) IlmResponse {
	newIndicesMappings := make(map[string]IlmIndexResponse)

	// Sort indices into buckets based on their configured prefix, if any matches.
	categorizedIndexNames := i.categorizeIndices(resp)
	for _, matchingIndices := range categorizedIndexNames {
		// Establish the number of each category of indices to use. User can configure to use only the latest 'X' amount.
		indicesCount := len(matchingIndices)
		indicesToTrackCount := indicesCount

		// Sort the indices if configured to do so.
		if i.numMostRecentIndices > 0 {
			if i.numMostRecentIndices < indicesToTrackCount {
				indicesToTrackCount = i.numMostRecentIndices
			}
			sort.Strings(matchingIndices)
		}

		// Gather only the number of indexes that have been configured, in descending order (most recent, if date-stamped).
		for i := indicesCount - 1; i >= indicesCount-indicesToTrackCount; i-- {
			indexName := matchingIndices[i]
			newIndicesMappings[indexName] = resp.Indices[indexName]
		}
	}
	//return new IlmResponse
	var iml IlmResponse
	iml.Indices = newIndicesMappings
	return iml
}

func (i *IlmIndiciesCollector) categorizeIndices(resp IlmResponse) map[string][]string {
	categorizedIndexNames := make(map[string][]string, len(resp.Indices))
	// If all indices are configured to be gathered, bucket them all together.
	if len(i.indicesIncluded) == 0 || i.indicesIncluded[0] == "_all" {
		for indexName := range resp.Indices {
			categorizedIndexNames["_all"] = append(categorizedIndexNames["_all"], indexName)
		}

		return categorizedIndexNames
	}

	// Bucket each returned index with its associated configured index (if any match).
	for indexName := range resp.Indices {
		match := indexName
		for name, matcher := range i.indexMatchers {
			// If a configured index matches one of the returned indexes, mark it as a match.
			if matcher.Match(match) {
				match = name
				break
			}
		}

		// Bucket all matching indices together for sorting.
		categorizedIndexNames[match] = append(categorizedIndexNames[match], indexName)
	}

	return categorizedIndexNames
}
