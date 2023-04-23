package elasticsearch

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/jsonx"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "elasticsearch"

// Nodestats are always generated, so simply define a constant for these endpoints
const statsPath = "/_nodes/stats"
const statsPathLocal = "/_nodes/_local/stats"

type nodeStat struct {
	Host       string            `json:"host"`
	Name       string            `json:"name"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
	Indices    interface{}       `json:"indices"`
	OS         interface{}       `json:"os"`
	Process    interface{}       `json:"process"`
	JVM        interface{}       `json:"jvm"`
	ThreadPool interface{}       `json:"thread_pool"`
	FS         interface{}       `json:"fs"`
	Transport  interface{}       `json:"transport"`
	HTTP       interface{}       `json:"http"`
	Breakers   interface{}       `json:"breakers"`
}

type clusterHealth struct {
	ActivePrimaryShards         int                    `json:"active_primary_shards"`
	ActiveShards                int                    `json:"active_shards"`
	ActiveShardsPercentAsNumber float64                `json:"active_shards_percent_as_number"`
	ClusterName                 string                 `json:"cluster_name"`
	DelayedUnassignedShards     int                    `json:"delayed_unassigned_shards"`
	InitializingShards          int                    `json:"initializing_shards"`
	NumberOfDataNodes           int                    `json:"number_of_data_nodes"`
	NumberOfInFlightFetch       int                    `json:"number_of_in_flight_fetch"`
	NumberOfNodes               int                    `json:"number_of_nodes"`
	NumberOfPendingTasks        int                    `json:"number_of_pending_tasks"`
	RelocatingShards            int                    `json:"relocating_shards"`
	Status                      string                 `json:"status"`
	TaskMaxWaitingInQueueMillis int                    `json:"task_max_waiting_in_queue_millis"`
	TimedOut                    bool                   `json:"timed_out"`
	UnassignedShards            int                    `json:"unassigned_shards"`
	Indices                     map[string]indexHealth `json:"indices"`
}

type indexHealth struct {
	ActivePrimaryShards int    `json:"active_primary_shards"`
	ActiveShards        int    `json:"active_shards"`
	InitializingShards  int    `json:"initializing_shards"`
	NumberOfReplicas    int    `json:"number_of_replicas"`
	NumberOfShards      int    `json:"number_of_shards"`
	RelocatingShards    int    `json:"relocating_shards"`
	Status              string `json:"status"`
	UnassignedShards    int    `json:"unassigned_shards"`
}

type clusterStats struct {
	NodeName    string      `json:"node_name"`
	ClusterName string      `json:"cluster_name"`
	Status      string      `json:"status"`
	Indices     interface{} `json:"indices"`
	Nodes       interface{} `json:"nodes"`
}

type indexStat struct {
	Primaries interface{}              `json:"primaries"`
	Total     interface{}              `json:"total"`
	Shards    map[string][]interface{} `json:"shards"`
}

type Elasticsearch struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Elasticsearch{}
	})
}

func (r *Elasticsearch) Clone() inputs.Input {
	return &Elasticsearch{}
}

func (c *Elasticsearch) Name() string {
	return inputName
}

func (r *Elasticsearch) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Local                bool            `toml:"local"`
	Servers              []string        `toml:"servers"`
	HTTPTimeout          config.Duration `toml:"http_timeout"`
	ClusterHealth        bool            `toml:"cluster_health"`
	ClusterHealthLevel   string          `toml:"cluster_health_level"`
	ClusterStats         bool            `toml:"cluster_stats"`
	IndicesInclude       []string        `toml:"indices_include"`
	IndicesLevel         string          `toml:"indices_level"`
	NodeStats            []string        `toml:"node_stats"`
	Username             string          `toml:"username"`
	Password             string          `toml:"password"`
	NumMostRecentIndices int             `toml:"num_most_recent_indices"`

	tls.ClientConfig
	client          *http.Client
	indexMatchers   map[string]filter.Filter
	serverInfo      map[string]serverInfo
	serverInfoMutex sync.Mutex
}

type serverInfo struct {
	nodeID   string
	masterID string
}

func (i serverInfo) isMaster() bool {
	return i.nodeID == i.masterID
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.HTTPTimeout <= 0 {
		ins.HTTPTimeout = config.Duration(time.Second * 5)
	}

	if ins.ClusterHealthLevel == "" {
		ins.ClusterHealthLevel = "indices"
	}

	// Compile the configured indexes to match for sorting.
	indexMatchers, err := ins.compileIndexMatchers()
	if err != nil {
		return err
	}

	ins.indexMatchers = indexMatchers
	ins.client, err = ins.createHTTPClient()
	return err
}

func (ins *Instance) compileIndexMatchers() (map[string]filter.Filter, error) {
	indexMatchers := map[string]filter.Filter{}
	var err error

	// Compile each configured index into a glob matcher.
	for _, configuredIndex := range ins.IndicesInclude {
		if _, exists := indexMatchers[configuredIndex]; !exists {
			indexMatchers[configuredIndex], err = filter.Compile([]string{configuredIndex})
			if err != nil {
				return nil, err
			}
		}
	}

	return indexMatchers, nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if ins.ClusterStats || len(ins.IndicesInclude) > 0 || len(ins.IndicesLevel) > 0 {
		var wgC sync.WaitGroup
		wgC.Add(len(ins.Servers))

		ins.serverInfo = make(map[string]serverInfo)
		for _, serv := range ins.Servers {
			go func(s string, slist *types.SampleList) {
				defer wgC.Done()
				info := serverInfo{}

				var err error

				// Gather node ID
				if info.nodeID, err = ins.gatherNodeID(s + "/_nodes/_local/name"); err != nil {
					slist.PushSample("elasticsearch", "up", 0, map[string]string{"address": s})
					log.Println("E! failed to gather node id:", err)
					return
				}

				// get cat/master information here so NodeStats can determine
				// whether this node is the Master
				if info.masterID, err = ins.getCatMaster(s + "/_cat/master"); err != nil {
					slist.PushSample("elasticsearch", "up", 0, map[string]string{"address": s})
					log.Println("E! failed to get cat master:", err)
					return
				}

				slist.PushSample("elasticsearch", "up", 1, map[string]string{"address": s})
				ins.serverInfoMutex.Lock()
				ins.serverInfo[s] = info
				ins.serverInfoMutex.Unlock()
			}(serv, slist)
		}
		wgC.Wait()
	}

	var wg sync.WaitGroup
	wg.Add(len(ins.Servers))

	for _, serv := range ins.Servers {
		go func(s string, slist *types.SampleList) {
			defer wg.Done()
			url := ins.nodeStatsURL(s)

			// Always gather node stats
			if err := ins.gatherNodeStats(url, s, slist); err != nil {
				log.Println("E! failed to gather node stats:", err)
				return
			}

			if ins.ClusterHealth {
				url = s + "/_cluster/health"
				if ins.ClusterHealthLevel != "" {
					url = url + "?level=" + ins.ClusterHealthLevel
				}
				if err := ins.gatherClusterHealth(url, s, slist); err != nil {
					log.Println("E! failed to gather cluster health:", err)
					return
				}
			}

			if ins.ClusterStats && (ins.serverInfo[s].isMaster() || !ins.Local) {
				if err := ins.gatherClusterStats(s+"/_cluster/stats", s, slist); err != nil {
					log.Println("E! failed to gather cluster stats:", err)
					return
				}
			}

			if len(ins.IndicesInclude) > 0 && (ins.serverInfo[s].isMaster() || !ins.Local) {
				if ins.IndicesLevel != "shards" {
					if err := ins.gatherIndicesStats(s+"/"+strings.Join(ins.IndicesInclude, ",")+"/_stats", s, slist); err != nil {
						log.Println("E! failed to gather indices stats:", err)
						return
					}
				} else {
					if err := ins.gatherIndicesStats(s+"/"+strings.Join(ins.IndicesInclude, ",")+"/_stats?level=shards", s, slist); err != nil {
						log.Println("E! failed to gather indices stats:", err)
						return
					}
				}
			}
		}(serv, slist)
	}

	wg.Wait()
}

func (ins *Instance) gatherIndicesStats(url string, address string, slist *types.SampleList) error {
	indicesStats := &struct {
		Shards  map[string]interface{} `json:"_shards"`
		All     map[string]interface{} `json:"_all"`
		Indices map[string]indexStat   `json:"indices"`
	}{}

	if err := ins.gatherJSONData(url, indicesStats); err != nil {
		return err
	}

	addrTag := map[string]string{"address": address}

	// Total Shards Stats
	slist.PushSamples("elasticsearch_indices_stats_shards_total", indicesStats.Shards, addrTag)

	// All Stats
	for m, s := range indicesStats.All {
		// parse Json, ignoring bools and excluding strings
		jsonParser := jsonx.JSONFlattener{}
		err := jsonParser.FullFlattenJSON("_", s, false, true)
		if err != nil {
			return err
		}
		for key, val := range jsonParser.Fields {
			slist.PushSample("elasticsearch", "indices_stats_"+m+"_"+key, val, map[string]string{"index_name": "_all"}, addrTag)
		}
	}

	// Gather stats for each index.
	return ins.gatherIndividualIndicesStats(indicesStats.Indices, addrTag, slist)
}

// gatherSortedIndicesStats gathers stats for all indices in no particular order.
func (ins *Instance) gatherIndividualIndicesStats(indices map[string]indexStat, addrTag map[string]string, slist *types.SampleList) error {
	// Sort indices into buckets based on their configured prefix, if any matches.
	categorizedIndexNames := ins.categorizeIndices(indices)
	for _, matchingIndices := range categorizedIndexNames {
		// Establish the number of each category of indices to use. User can configure to use only the latest 'X' amount.
		indicesCount := len(matchingIndices)
		indicesToTrackCount := indicesCount

		// Sort the indices if configured to do so.
		if ins.NumMostRecentIndices > 0 {
			if ins.NumMostRecentIndices < indicesToTrackCount {
				indicesToTrackCount = ins.NumMostRecentIndices
			}
			sort.Strings(matchingIndices)
		}

		// Gather only the number of indexes that have been configured, in descending order (most recent, if date-stamped).
		for i := indicesCount - 1; i >= indicesCount-indicesToTrackCount; i-- {
			indexName := matchingIndices[i]

			err := ins.gatherSingleIndexStats(indexName, indices[indexName], addrTag, slist)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (ins *Instance) gatherSingleIndexStats(name string, index indexStat, addrTag map[string]string, slist *types.SampleList) error {
	indexTag := map[string]string{"index_name": name}
	stats := map[string]interface{}{
		"primaries": index.Primaries,
		"total":     index.Total,
	}
	for m, s := range stats {
		f := jsonx.JSONFlattener{}
		// parse Json, getting strings and bools
		err := f.FullFlattenJSON("", s, true, true)
		if err != nil {
			return err
		}
		for key, val := range f.Fields {
			slist.PushSample("elasticsearch", "indices_stats_"+m+"_"+key, val, indexTag, addrTag)
		}
	}

	if ins.IndicesLevel == "shards" {
		for shardNumber, shards := range index.Shards {
			for _, shard := range shards {
				// Get Shard Stats
				flattened := jsonx.JSONFlattener{}
				err := flattened.FullFlattenJSON("", shard, true, true)
				if err != nil {
					return err
				}

				// determine shard tag and primary/replica designation
				shardType := "replica"
				routingPrimary, _ := flattened.Fields["routing_primary"].(bool)
				if routingPrimary {
					shardType = "primary"
				}
				delete(flattened.Fields, "routing_primary")

				routingState, ok := flattened.Fields["routing_state"].(string)
				if ok {
					flattened.Fields["routing_state"] = mapShardStatusToCode(routingState)
				}

				routingNode, _ := flattened.Fields["routing_node"].(string)
				shardTags := map[string]string{
					"index_name": name,
					"node_id":    routingNode,
					"shard_name": shardNumber,
					"type":       shardType,
				}

				for key, field := range flattened.Fields {
					switch field.(type) {
					case string, bool:
						delete(flattened.Fields, key)
					}
				}

				slist.PushSamples("elasticsearch_indices_stats_shards", flattened.Fields, shardTags, addrTag)
			}
		}
	}

	return nil
}

func (ins *Instance) categorizeIndices(indices map[string]indexStat) map[string][]string {
	categorizedIndexNames := map[string][]string{}

	// If all indices are configured to be gathered, bucket them all together.
	if len(ins.IndicesInclude) == 0 || ins.IndicesInclude[0] == "_all" {
		for indexName := range indices {
			categorizedIndexNames["_all"] = append(categorizedIndexNames["_all"], indexName)
		}

		return categorizedIndexNames
	}

	// Bucket each returned index with its associated configured index (if any match).
	for indexName := range indices {
		match := indexName
		for name, matcher := range ins.indexMatchers {
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

func (ins *Instance) gatherClusterStats(url string, address string, slist *types.SampleList) error {
	clusterStats := &clusterStats{}
	if err := ins.gatherJSONData(url, clusterStats); err != nil {
		return err
	}

	tags := map[string]string{
		// "node_name":    clusterStats.NodeName,
		// "status":       clusterStats.Status,
		"cluster_name": clusterStats.ClusterName,
		"address":      address,
	}

	stats := map[string]interface{}{
		"nodes":   clusterStats.Nodes,
		"indices": clusterStats.Indices,
	}

	for p, s := range stats {
		f := jsonx.JSONFlattener{}
		// parse json, including bools and excluding strings
		err := f.FullFlattenJSON("", s, false, true)
		if err != nil {
			return err
		}

		for key, val := range f.Fields {
			slist.PushSample("elasticsearch", "clusterstats_"+p+"_"+key, val, tags)
		}
	}

	return nil
}

func (ins *Instance) gatherClusterHealth(url string, address string, slist *types.SampleList) error {
	healthStats := &clusterHealth{}
	if err := ins.gatherJSONData(url, healthStats); err != nil {
		return err
	}

	addrTag := map[string]string{"address": address}

	clusterFields := map[string]interface{}{
		"cluster_health_active_primary_shards":            healthStats.ActivePrimaryShards,
		"cluster_health_active_shards":                    healthStats.ActiveShards,
		"cluster_health_active_shards_percent_as_number":  healthStats.ActiveShardsPercentAsNumber,
		"cluster_health_delayed_unassigned_shards":        healthStats.DelayedUnassignedShards,
		"cluster_health_initializing_shards":              healthStats.InitializingShards,
		"cluster_health_number_of_data_nodes":             healthStats.NumberOfDataNodes,
		"cluster_health_number_of_in_flight_fetch":        healthStats.NumberOfInFlightFetch,
		"cluster_health_number_of_nodes":                  healthStats.NumberOfNodes,
		"cluster_health_number_of_pending_tasks":          healthStats.NumberOfPendingTasks,
		"cluster_health_relocating_shards":                healthStats.RelocatingShards,
		"cluster_health_status_code":                      mapHealthStatusToCode(healthStats.Status),
		"cluster_health_task_max_waiting_in_queue_millis": healthStats.TaskMaxWaitingInQueueMillis,
		"cluster_health_timed_out":                        healthStats.TimedOut,
		"cluster_health_unassigned_shards":                healthStats.UnassignedShards,
	}

	slist.PushSamples("elasticsearch", clusterFields, map[string]string{"cluster_name": healthStats.ClusterName}, addrTag)

	for name, health := range healthStats.Indices {
		indexFields := map[string]interface{}{
			"cluster_health_indices_active_primary_shards": health.ActivePrimaryShards,
			"cluster_health_indices_active_shards":         health.ActiveShards,
			"cluster_health_indices_initializing_shards":   health.InitializingShards,
			"cluster_health_indices_number_of_replicas":    health.NumberOfReplicas,
			"cluster_health_indices_number_of_shards":      health.NumberOfShards,
			"cluster_health_indices_relocating_shards":     health.RelocatingShards,
			"cluster_health_indices_status_code":           mapHealthStatusToCode(health.Status),
			"cluster_health_indices_unassigned_shards":     health.UnassignedShards,
		}
		slist.PushSamples("elasticsearch", indexFields, map[string]string{"index": name, "name": healthStats.ClusterName}, addrTag)
	}

	return nil
}

func (ins *Instance) gatherNodeStats(url string, address string, slist *types.SampleList) error {
	nodeStats := &struct {
		ClusterName string               `json:"cluster_name"`
		Nodes       map[string]*nodeStat `json:"nodes"`
	}{}

	if err := ins.gatherJSONData(url, nodeStats); err != nil {
		return err
	}

	addrTag := map[string]string{"address": address}

	for id, n := range nodeStats.Nodes {
		// sort.Strings(n.Roles)
		tags := map[string]string{
			"node_id":      id,
			"node_host":    n.Host,
			"node_name":    n.Name,
			"cluster_name": nodeStats.ClusterName,
			// "node_roles":   strings.Join(n.Roles, ","),
		}

		for k, v := range n.Attributes {
			slist.PushSample("elasticsearch", "node_attribute_"+k, v, tags, addrTag)
		}

		stats := map[string]interface{}{
			"indices":     n.Indices,
			"os":          n.OS,
			"process":     n.Process,
			"jvm":         n.JVM,
			"thread_pool": n.ThreadPool,
			"fs":          n.FS,
			"transport":   n.Transport,
			"http":        n.HTTP,
			"breakers":    n.Breakers,
		}

		for p, s := range stats {
			// if one of the individual node stats is not even in the
			// original result
			if s == nil {
				continue
			}
			f := jsonx.JSONFlattener{}
			// parse Json, ignoring strings and bools
			err := f.FlattenJSON("", s)
			if err != nil {
				return err
			}

			for key, val := range f.Fields {
				slist.PushSample("elasticsearch", p+"_"+key, val, tags, addrTag)
			}
		}
	}

	return nil
}

func (ins *Instance) nodeStatsURL(baseURL string) string {
	var url string

	if ins.Local {
		url = baseURL + statsPathLocal
	} else {
		url = baseURL + statsPath
	}

	if len(ins.NodeStats) == 0 {
		return url
	}

	return fmt.Sprintf("%s/%s", url, strings.Join(ins.NodeStats, ","))
}

func (ins *Instance) getCatMaster(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	if ins.Username != "" || ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	r, err := ins.client.Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		// NOTE: we are not going to read/discard r.Body under the assumption we'd prefer
		// to let the underlying transport close the connection and re-establish a new one for
		// future calls.
		return "", fmt.Errorf("elasticsearch: Unable to retrieve master node information. API responded with status-code %d, expected %d", r.StatusCode, http.StatusOK)
	}
	response, err := io.ReadAll(r.Body)

	if err != nil {
		return "", err
	}

	masterID := strings.Split(string(response), " ")[0]

	return masterID, nil
}

func (ins *Instance) gatherNodeID(url string) (string, error) {
	nodeStats := &struct {
		ClusterName string               `json:"cluster_name"`
		Nodes       map[string]*nodeStat `json:"nodes"`
	}{}
	if err := ins.gatherJSONData(url, nodeStats); err != nil {
		return "", err
	}

	// Only 1 should be returned
	for id := range nodeStats.Nodes {
		return id, nil
	}
	return "", nil
}

func (ins *Instance) gatherJSONData(url string, v interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if ins.Username != "" || ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	r, err := ins.client.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		// NOTE: we are not going to read/discard r.Body under the assumption we'd prefer
		// to let the underlying transport close the connection and re-establish a new one for
		// future calls.
		return fmt.Errorf("elasticsearch: API responded with status-code %d, expected %d",
			r.StatusCode, http.StatusOK)
	}

	return json.NewDecoder(r.Body).Decode(v)
}

// perform status mapping
func mapHealthStatusToCode(s string) int {
	switch strings.ToLower(s) {
	case "green":
		return 1
	case "yellow":
		return 2
	case "red":
		return 3
	}
	return 0
}

// perform shard status mapping
func mapShardStatusToCode(s string) int {
	switch strings.ToUpper(s) {
	case "UNASSIGNED":
		return 1
	case "INITIALIZING":
		return 2
	case "STARTED":
		return 3
	case "RELOCATING":
		return 4
	}
	return 0
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		ResponseHeaderTimeout: time.Duration(ins.HTTPTimeout),
		TLSClientConfig:       tlsCfg,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(ins.HTTPTimeout),
	}

	return client, nil
}
