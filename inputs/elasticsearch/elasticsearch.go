package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/elasticsearch/collector"
	"flashcat.cloud/categraf/inputs/elasticsearch/pkg/clusterinfo"
	"flashcat.cloud/categraf/inputs/elasticsearch/pkg/roundtripper"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"

	"github.com/prometheus/common/version"
	"k8s.io/klog/v2"
)

const inputName = "elasticsearch"

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(Elasticsearch)
var _ inputs.InstancesGetter = new(Elasticsearch)

type (
	Elasticsearch struct {
		config.PluginConfig

		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		Local                     bool            `toml:"local"`
		Servers                   []string        `toml:"servers"`
		UserName                  string          `toml:"username"`
		Password                  string          `toml:"password"`
		ApiKey                    string          `toml:"api_key"`
		HTTPTimeout               config.Duration `toml:"http_timeout"`
		AllNodes                  bool            `toml:"all_nodes"`
		Node                      string          `toml:"node"`
		NodeStats                 []string        `toml:"node_stats"`
		ClusterHealth             bool            `toml:"cluster_health"`
		ClusterHealthLevel        string          `toml:"cluster_health_level"`
		ClusterStats              bool            `toml:"cluster_stats"`
		IndicesInclude            []string        `toml:"indices_include"`
		ExportIndices             bool            `toml:"export_indices"`
		ExportIndicesSettings     bool            `toml:"export_indices_settings"`
		ExportIndicesMappings     bool            `toml:"export_indices_mappings"`
		ExportIndicesAliases      bool            `toml:"export_indices_aliases"`
		ExportIndexAliases        bool            `toml:"export_index_aliases"`
		ExportILM                 bool            `toml:"export_ilm"`
		ExportShards              bool            `toml:"export_shards"`
		ExportSLM                 bool            `toml:"export_slm"`
		ExportDataStream          bool            `toml:"export_data_stream"`
		ExportSnapshots           bool            `toml:"export_snapshots"`
		ExportClusterSettings     bool            `toml:"export_cluster_settings"`
		ExportClusterInfo         bool            `toml:"export_cluster_info"`
		ClusterInfoInterval       config.Duration `toml:"cluster_info_interval"`
		AwsRegion                 string          `toml:"aws_region"`
		AwsRoleArn                string          `toml:"aws_role_arn"`
		NumMostRecentIndices      int             `toml:"num_most_recent_indices"`
		DynamicIndexMatcherRegexp []string        `toml:"dynamic_index_matcher_regexp"`
		MaxIndicesIncludeCount    int             `toml:"max_indices_include_count"`
		NewIndicesInclude         []string

		EsURL *url.URL
		*http.Client
		tls.ClientConfig
		indexMatchers   map[string]filter.Filter
		serverInfo      map[string]serverInfo
		hasRunBefore    bool
		serverInfoMutex sync.Mutex
	}

	transportWithAPIKey struct {
		underlyingTransport http.RoundTripper
		apiKey              string
	}

	serverInfo struct {
		nodeID      string
		masterID    string
		clusterName string
	}

	IndicesInfo struct {
		Index string `json:"index"` //index name
	}
)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Elasticsearch{}
	})
}

func (e *Elasticsearch) Clone() inputs.Input {
	return &Elasticsearch{}
}

func (e *Elasticsearch) Name() string {
	return inputName
}

func (e *Elasticsearch) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(e.Instances))
	for i := 0; i < len(e.Instances); i++ {
		ret[i] = e.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}
	if ins.HTTPTimeout <= 0 {
		ins.HTTPTimeout = config.Duration(5 * time.Second)
	}
	if ins.ClusterInfoInterval == 0 {
		ins.ClusterInfoInterval = config.Duration(5 * time.Minute)
	}
	if ins.UserName == "" {
		ins.UserName = os.Getenv("ES_USERNAME")
	}
	if ins.Password == "" {
		ins.Password = os.Getenv("ES_PASSWORD")
	}
	if ins.ApiKey == "" {
		ins.ApiKey = os.Getenv("ES_API_KEY")
	}
	ins.hasRunBefore = false

	// Compile the configured indexes to match for sorting.
	indexMatchers, err := ins.compileIndexMatchers()
	if err != nil {
		return err
	}
	ins.indexMatchers = indexMatchers

	ins.Client, err = ins.createHTTPClient()
	if err != nil {
		return err
	}
	if ins.ExportIndexAliases {
		klog.Warning("export_index_aliases is deprecated, use export_indices_aliases instead")
		ins.ExportIndicesAliases = true
	}

	if ins.MaxIndicesIncludeCount == 0 {
		//set default value
		//Prevent getting requests from becoming too long and failing due to excessively long indices_include values
		ins.MaxIndicesIncludeCount = 80
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	// version metric
	if err := inputs.Collect(NewCollector(inputName), slist); err != nil {
		klog.ErrorS(err, "failed to collect elasticsearch version metric")
	}
	if ins.ClusterStats || len(ins.IndicesInclude) > 0 {
		var wgC sync.WaitGroup
		wgC.Add(len(ins.Servers))

		ins.serverInfo = make(map[string]serverInfo)
		for _, serv := range ins.Servers {
			go func(s string, slist *types.SampleList) {
				defer wgC.Done()
				info := serverInfo{}
				var err error

				// Gather node ID
				if info.nodeID, err = collector.GetNodeID(ins.Client, ins.UserName, ins.Password, s); err != nil {
					slist.PushSample("elasticsearch", "up", 0, map[string]string{"address": s})
					klog.ErrorS(err, "failed to gather elasticsearch node id", "address", s)
					return
				}

				// get cat/master information here so NodeStats can determine
				// whether this node is the Master
				if info.masterID, err = collector.GetCatMaster(ins.Client, ins.UserName, ins.Password, s); err != nil {
					slist.PushSample("elasticsearch", "up", 0, map[string]string{"address": s})
					klog.ErrorS(err, "failed to get elasticsearch cat master", "address", s)
					return
				}

				if info.clusterName, err = collector.GetClusterName(ins.Client, ins.UserName, ins.Password, s); err != nil {
					slist.PushSample("elasticsearch", "up", 0, map[string]string{"address": s})
					klog.ErrorS(err, "failed to get elasticsearch cluster name", "address", s)
					return
				}

				slist.PushSample("elasticsearch", "up", 1, map[string]string{
					"address": s,
					"cluster": info.clusterName,
				})
				ins.serverInfoMutex.Lock()
				ins.serverInfo[s] = info
				ins.serverInfoMutex.Unlock()
			}(serv, slist)
		}
		wgC.Wait()
	}

	var wg sync.WaitGroup
	wg.Add(len(ins.Servers))

	// create the exporter
	for _, serv := range ins.Servers {
		go func(s string, slist *types.SampleList) {
			defer wg.Done()
			EsUrl, err := url.Parse(s)
			if err != nil {
				klog.ErrorS(err, "failed to parse elasticsearch url", "address", s)
				return
			}
			if ins.UserName != "" && ins.Password != "" {
				EsUrl.User = url.UserPassword(ins.UserName, ins.Password)
			}
			exporter, err := collector.NewElasticsearchCollector(
				[]string{},
				collector.WithElasticsearchURL(EsUrl),
				collector.WithHTTPClient(ins.Client),
				collector.EnableExportDataStream(ins.ExportDataStream),
				collector.EnableExportILM(ins.ExportILM),
				collector.EnableExportSLM(ins.ExportSLM),
				collector.EnableExportSnapshots(ins.ExportSnapshots),
				collector.EnableExportClusterSettings(ins.ExportClusterSettings),
			)
			if err != nil {
				klog.ErrorS(err, "failed to create Elasticsearch collector", "address", s)
				return
			}
			if err := inputs.Collect(exporter, slist); err != nil {
				klog.ErrorS(err, "failed to collect elasticsearch exporter metrics", "address", s)
			}

			if ins.NumMostRecentIndices != 0 {
				//match Dynamic indexing
				//query all indices
				uu := *EsUrl
				//url: /_cat/indices?format=json&h=index
				if len(ins.IndicesInclude) > 0 {
					uu.Path = path.Join(uu.Path, "/_cat/indices/"+strings.Join(ins.IndicesInclude, ","))
				} else {
					uu.Path = path.Join(uu.Path, "/_cat/indices")
				}
				uu.RawQuery = "format=json&s=index:desc&h=index"
				indices_bts, err := ins.queryURL(&uu)
				if err != nil {
					klog.ErrorS(err, "failed to query all elasticsearch indices", "address", s)
				}
				var indices []IndicesInfo
				if err := json.Unmarshal(indices_bts, &indices); err != nil {
					klog.ErrorS(err, "failed to unmarshal elasticsearch indices", "address", s)
				}

				var indexList []string
				//match Dynamic indexing，exchange index name
				indexList = ins.classifyDynamicIndexes(indices)
				//must use NewIndicesInclude,cannot recover IndicesInclude
				ins.NewIndicesInclude = indexList

			} else {
				ins.NewIndicesInclude = ins.IndicesInclude
			}

			// Always gather node stats
			if err := inputs.Collect(collector.NewNodes(ins.Client, EsUrl, ins.AllNodes, ins.Node, ins.Local, ins.NodeStats), slist); err != nil {
				klog.ErrorS(err, "failed to collect elasticsearch nodes metrics", "address", s)
			}

			clusterInfoRetriever := clusterinfo.New(ins.Client, EsUrl, time.Duration(ins.ClusterInfoInterval))

			if ins.ClusterHealth {
				if ins.ClusterHealthLevel == "indices" {
					if err := inputs.Collect(collector.NewClusterHealthIndices(ins.Client, EsUrl), slist); err != nil {
						klog.ErrorS(err, "failed to collect elasticsearch cluster health indices metrics", "address", s)
					}
				} else {
					if err := inputs.Collect(collector.NewClusterHealth(ins.Client, EsUrl), slist); err != nil {
						klog.ErrorS(err, "failed to collect elasticsearch cluster health metrics", "address", s)
					}
				}
			}

			if ins.ClusterStats && (ins.serverInfo[s].isMaster() || !ins.Local) {
				if err := inputs.Collect(collector.NewClusterStats(ins.Client, EsUrl), slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch cluster stats metrics", "address", s)
				}
			}

			if (ins.ExportIndices || ins.ExportShards) && (ins.serverInfo[s].isMaster() || !ins.Local) {
				sC := collector.NewShards(ins.Client, EsUrl)
				if err := inputs.Collect(sC, slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch shards metrics", "address", s)
				}
				iC := collector.NewIndices(ins.Client, EsUrl, ins.ExportShards, ins.ExportIndicesAliases, ins.NewIndicesInclude, ins.MaxIndicesIncludeCount)
				if err := inputs.Collect(iC, slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch indices metrics", "address", s)
				}
				if registerErr := clusterInfoRetriever.RegisterConsumer(iC); registerErr != nil {
					klog.ErrorS(registerErr, "failed to register indices collector in cluster info", "address", s)
				}
				if registerErr := clusterInfoRetriever.RegisterConsumer(sC); registerErr != nil {
					klog.ErrorS(registerErr, "failed to register shards collector in cluster info", "address", s)
				}
			}

			if ins.ExportIndicesSettings {
				if err := inputs.Collect(collector.NewIndicesSettings(ins.Client, EsUrl, ins.NewIndicesInclude, ins.MaxIndicesIncludeCount), slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch indices settings metrics", "address", s)
				}
			}

			if ins.ExportIndicesMappings {
				if err := inputs.Collect(collector.NewIndicesMappings(ins.Client, EsUrl, ins.NewIndicesInclude, ins.MaxIndicesIncludeCount), slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch indices mappings metrics", "address", s)
				}
			}

			if ins.ExportClusterInfo && !ins.hasRunBefore {
				// Create a context that is cancelled on SIGKILL or SIGINT.
				ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)

				// start the cluster info retriever
				switch runErr := clusterInfoRetriever.Run(ctx); {
				case runErr == nil:
					if ins.DebugMod {
						klog.V(1).InfoS("started cluster info retriever", "interval", ins.ClusterInfoInterval)
					}
				case errors.Is(runErr, clusterinfo.ErrInitialCallTimeout):
					if ins.DebugMod {
						klog.V(1).InfoS("initial cluster info call timed out")
					}
				default:
					klog.ErrorS(runErr, "failed to run cluster info retriever", "address", s)
					return
				}

				// register cluster info retriever as prometheus collector
				if err := inputs.Collect(clusterInfoRetriever, slist); err != nil {
					klog.ErrorS(err, "failed to collect elasticsearch cluster info metrics", "address", s)
				}
				ins.serverInfoMutex.Lock()
				ins.hasRunBefore = true
				ins.serverInfoMutex.Unlock()
			}

		}(serv, slist)
	}

	wg.Wait()
	return
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	var httpTransport http.RoundTripper
	var err error
	httpTransport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 1,
	}
	if ins.ApiKey != "" {
		httpTransport = &transportWithAPIKey{
			underlyingTransport: httpTransport,
			apiKey:              ins.ApiKey,
		}
	}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return nil, err
		}
		httpTransport = &http.Transport{
			TLSClientConfig:     tlsConfig,
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: 1,
		}
	}

	client := &http.Client{
		Timeout:   time.Duration(ins.HTTPTimeout),
		Transport: httpTransport,
	}
	if ins.AwsRegion != "" {
		ins.Client.Transport, err = roundtripper.NewAWSSigningTransport(httpTransport, ins.AwsRegion, ins.AwsRoleArn)
		if err != nil {
			klog.ErrorS(err, "failed to create AWS transport")
		}
	}

	return client, nil
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

func (ins *Instance) queryURL(u *url.URL) ([]byte, error) {
	res, err := ins.Client.Get(u.String())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get resource from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}
	defer func() {
		err := res.Body.Close()
		if err != nil {
			klog.ErrorS(err, "failed to close elasticsearch response body")
		}
	}()

	if res.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("HTTP Request failed with status code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}
	return bts, nil
}

// match Dynamic Indexes
func (ins *Instance) classifyDynamicIndexes(indicesInfo []IndicesInfo) []string {

	if len(ins.DynamicIndexMatcherRegexp) == 0 {
		//default matcher
		ins.DynamicIndexMatcherRegexp = append(ins.DynamicIndexMatcherRegexp, `(?P<date>(?:\\d{4}|\\d{2})[.-]?(?:\\d{2})[.-]?(?:\\d{2})?[.-]?(?:\\d{2})?)$`)
		ins.DynamicIndexMatcherRegexp = append(ins.DynamicIndexMatcherRegexp, `[\\.-._]\\d+(\\.\\d+){0,2}$`)
	}

	var patterns []*regexp.Regexp

	for _, patternStr := range ins.DynamicIndexMatcherRegexp {
		re := regexp.MustCompile(patternStr)
		patterns = append(patterns, re)
	}

	groups := make(map[string][]string)

	for _, index := range indicesInfo {
		matched := false

		// Attempt to match known dynamic patterns
		for _, pattern := range patterns {
			if loc := pattern.FindStringIndex(index.Index); loc != nil {
				// Construct group patterns (replace dynamic parts with *)
				groupKey := index.Index[:loc[0]] + "*" + index.Index[loc[1]:]
				groups[groupKey] = append(groups[groupKey], index.Index)
				matched = true
				break
			}
		}

		// Indexes not matching known patterns are grouped separately
		if !matched {
			groups[index.Index] = []string{index.Index}

		}
	}

	if ins.DebugMod {
		for pattern, indexes := range groups {
			fmt.Printf("[%s] (%d  index total \n)", pattern, len(indexes))
			if len(indexes) > 5 {
				fmt.Printf(" result: %v ... \n", indexes[:5])
			} else {
				fmt.Printf("result:  %v \n", indexes)
			}
		}
	}

	var new_indices []string

	//Retrieve the first n indexes
	for pattern, indexes := range groups {
		if ins.DebugMod {
			fmt.Printf("[%s] (%d index total) \n", pattern, len(indexes))
		}
		if len(indexes) > ins.NumMostRecentIndices {
			if ins.DebugMod {
				fmt.Printf(" result: %v \n", indexes[:ins.NumMostRecentIndices])
			}
			new_indices = append(new_indices, indexes[:ins.NumMostRecentIndices]...)
		} else {
			if ins.DebugMod {
				fmt.Printf("result: %v \n", indexes)
			}
			new_indices = append(new_indices, indexes[:]...)
		}
	}

	return new_indices
}

func (t *transportWithAPIKey) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s", t.apiKey))
	return t.underlyingTransport.RoundTrip(req)
}

func (i serverInfo) isMaster() bool {
	return i.nodeID == i.masterID
}

func NewCollector(program string) prometheus.Collector {
	return prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: program,
			Name:      "build_info",
			Help: fmt.Sprintf(
				"A metric with a constant '1' value labeled by version, revision, branch, goversion from which %s was built, and the goos and goarch for the build.",
				program,
			),
			ConstLabels: prometheus.Labels{
				"version":   version.Version,
				"revision":  version.Revision,
				"branch":    version.Branch,
				"goversion": runtime.Version(),
				"goos":      runtime.GOOS,
				"goarch":    runtime.GOARCH,
				"tags":      version.GetTags(),
			},
		},
		func() float64 { return 1 },
	)
}
