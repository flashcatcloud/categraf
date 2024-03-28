package collector

// ClusterStatsResponse defines node stats information structure for nodes
type ClusterStatsResponse struct {
	NodeName    string                      `json:"node_name"`
	ClusterName string                      `json:"cluster_name"`
	Status      string                      `json:"status"`
	Indices     ClusterStatsIndicesResponse `json:"indices"`
	Nodes       ClusterStatsNodesResponse   `json:"nodes"`
}

// ClusterStatsIndicesResponse is a representation of a indices stats (size, document count, indexing and deletion times, search times, field cache size, merges and flushes)
type ClusterStatsIndicesResponse struct {
	Count        int64 `json:"count"`
	Docs         ClusterStatsIndicesDocsResponse
	Indexing     ClusterStatsIndicesIndexingResponse
	FieldData    ClusterStatsIndicesCacheResponse `json:"fielddata"`
	FilterCache  ClusterStatsIndicesCacheResponse `json:"filter_cache"`
	QueryCache   ClusterStatsIndicesCacheResponse `json:"query_cache"`
	RequestCache ClusterStatsIndicesCacheResponse `json:"request_cache"`
	Segments     ClusterStatsIndicesSegmentsResponse
	Completion   ClusterStatsIndicesCompletionResponse
	Shards       ClusterStatsIndicesShardsResponse
	Store        ClusterStatsIndicesStoreResponse
}

// ClusterStatsIndicesDocsResponse defines node stats docs information structure for indices
type ClusterStatsIndicesDocsResponse struct {
	Count   int64 `json:"count"`
	Deleted int64 `json:"deleted"`
}

// ClusterStatsIndicesIndexingResponse defines node stats indexing information structure for indices
type ClusterStatsIndicesIndexingResponse struct {
	IndexTotal    int64 `json:"index_total"`
	IndexTime     int64 `json:"index_time_in_millis"`
	IndexCurrent  int64 `json:"index_current"`
	DeleteTotal   int64 `json:"delete_total"`
	DeleteTime    int64 `json:"delete_time_in_millis"`
	DeleteCurrent int64 `json:"delete_current"`
	IsThrottled   bool  `json:"is_throttled"`
	ThrottleTime  int64 `json:"throttle_time_in_millis"`
}

// ClusterStatsIndicesCacheResponse defines node stats cache information structure for indices
type ClusterStatsIndicesCacheResponse struct {
	Evictions  int64 `json:"evictions"`
	MemorySize int64 `json:"memory_size_in_bytes"`
	CacheCount int64 `json:"cache_count"`
	CacheSize  int64 `json:"cache_size"`
	HitCount   int64 `json:"hit_count"`
	MissCount  int64 `json:"miss_count"`
	TotalCount int64 `json:"total_count"`
}

// ClusterStatsIndicesSegmentsResponse defines node stats segments information structure for indices
type ClusterStatsIndicesSegmentsResponse struct {
	Count                    int64 `json:"count"`
	Memory                   int64 `json:"memory_in_bytes"`
	TermsMemory              int64 `json:"terms_memory_in_bytes"`
	IndexWriterMemory        int64 `json:"index_writer_memory_in_bytes"`
	NormsMemory              int64 `json:"norms_memory_in_bytes"`
	StoredFieldsMemory       int64 `json:"stored_fields_memory_in_bytes"`
	FixedBitSet              int64 `json:"fixed_bit_set_memory_in_bytes"`
	DocValuesMemory          int64 `json:"doc_values_memory_in_bytes"`
	TermVectorsMemory        int64 `json:"term_vectors_memory_in_bytes"`
	PointsMemory             int64 `json:"points_memory_in_bytes"`
	VersionMapMemory         int64 `json:"version_map_memory_in_bytes"`
	MaxUnsafeAutoIDTimestamp int64 `json:"max_unsafe_auto_id_timestamp"`
}

// ClusterStatsIndicesCompletionResponse defines node stats completion information structure for indices
type ClusterStatsIndicesCompletionResponse struct {
	Size int64 `json:"size_in_bytes"`
}

// ClusterStatsIndicesShardsResponse defines node stats shards information structure for indices
type ClusterStatsIndicesShardsResponse struct {
	Total       float64                                `json:"total"`
	Primaries   float64                                `json:"primaries"`
	Replication float64                                `json:"replication"`
	Index       ClusterStatsIndicesShardsIndexResponse `json:"index"`
}

// ClusterStatsIndicesShardsIndexResponse defines node stats shards index information structure for indices
type ClusterStatsIndicesShardsIndexResponse struct {
	Shards    ClusterStatsValueResponse `json:"shards"`
	Primaries ClusterStatsValueResponse `json:"primaries"`
	Replicas  ClusterStatsValueResponse `json:"replicas"`
}

// ClusterStatsValueResponse defines node stats shards index value information structure for indices
type ClusterStatsValueResponse struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// ClusterStatsIndicesStoreResponse defines node stats store information structure for indices
type ClusterStatsIndicesStoreResponse struct {
	Size             int64 `json:"size_in_bytes"`
	TotalDataSetSize int64 `json:"total_data_set_size_in_bytes"`
	Reserved         int64 `json:"reserved_in_bytes"`
}

// ClusterStatsNodesResponse defines node stats information structure for nodes
type ClusterStatsNodesResponse struct {
	Count        ClusterStatsNodesCountResponse        `json:"count"`
	JVM          ClusterStatsNodesJVMResponse          `json:"jvm"`
	FS           ClusterStatsNodesFSResponse           `json:"fs"`
	OS           ClusterStatsNodesOSResponse           `json:"os"`
	Process      ClusterStatsNodesProcessResponse      `json:"process"`
	NetWorkTypes ClusterStatsNodesNetworkTypesResponse `json:"network_types"`
	Versions     []string                              `json:"versions"`
}

// ClusterStatsNodesCountResponse defines node stats count information structure for nodes
type ClusterStatsNodesCountResponse struct {
	Total               int64 `json:"total"`
	CoordinatingOnly    int64 `json:"coordinating_only"`
	Data                int64 `json:"data"`
	DataCold            int64 `json:"data_cold"`
	DataContent         int64 `json:"data_content"`
	DataFrozen          int64 `json:"data_frozen"`
	DataHot             int64 `json:"data_hot"`
	DataWarm            int64 `json:"data_warm"`
	Ingest              int64 `json:"ingest"`
	Master              int64 `json:"master"`
	Ml                  int64 `json:"ml"`
	RemoteClusterClient int64 `json:"remote_cluster_client"`
	Transform           int64 `json:"transform"`
	VotingOnly          int64 `json:"voting_only"`
}

// ClusterStatsNodesJVMResponse defines node stats JVM information structure for nodes
type ClusterStatsNodesJVMResponse struct {
	MaxUptimeInMillis int64                                  `json:"max_uptime_in_millis"`
	Mem               ClusterStatsNodesJVMMemResponse        `json:"mem"`
	Versions          []ClusterStatsNodesJVMVersionsResponse `json:"versions"`
	Threads           int64                                  `json:"threads"`
}

// ClusterStatsNodesJVMMemResponse defines node stats JVM memory information structure for nodes
type ClusterStatsNodesJVMMemResponse struct {
	HeapUsedInBytes int64 `json:"heap_used_in_bytes"`
	HeapMaxInBytes  int64 `json:"heap_max_in_bytes"`
}

// ClusterStatsNodesJVMVersionsResponse defines node stats JVM versions information structure for nodes
type ClusterStatsNodesJVMVersionsResponse struct {
	Version   string `json:"version"`
	VMName    string `json:"vm_name"`
	VMVersion string `json:"vm_version"`
	VMVendor  string `json:"vm_vendor"`
	Count     int64  `json:"count"`
}

// ClusterStatsNodesFSResponse defines node stats FS information structure for nodes
type ClusterStatsNodesFSResponse struct {
	TotalInBytes     int64 `json:"total_in_bytes"`
	FreeInBytes      int64 `json:"free_in_bytes"`
	AvailableInBytes int64 `json:"available_in_bytes"`
}

// ClusterStatsNodesOSResponse defines node stats OS information structure for nodes
type ClusterStatsNodesOSResponse struct {
	AvailableProcessors int64                              `json:"available_processors"`
	AllocatedProcessors int64                              `json:"allocated_processors"`
	Mem                 ClusterStatsNodesOSMemResponse     `json:"mem"`
	Names               []ClusterStatsNodesOSNamesResponse `json:"names"`
}

// ClusterStatsNodesOSMemResponse defines node stats OS memory information structure for nodes
type ClusterStatsNodesOSMemResponse struct {
	TotalInBytes int64 `json:"total_in_bytes"`
	FreeInBytes  int64 `json:"free_in_bytes"`
	UsedInBytes  int64 `json:"used_in_bytes"`
	FreePercent  int64 `json:"free_percent"`
	UsedPercent  int64 `json:"used_percent"`
}

// ClusterStatsNodesOSNamesResponse defines node stats OS names information structure for nodes
type ClusterStatsNodesOSNamesResponse struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ClusterStatsNodesProcessResponse defines node stats process information structure for nodes
type ClusterStatsNodesProcessResponse struct {
	OpenFileDescriptors ClusterStatsValueResponse           `json:"open_file_descriptors"`
	CPU                 ClusterStatsNodesProcessCPUResponse `json:"cpu"`
}

// ClusterStatsNodesProcessCPUResponse defines node stats process CPU information structure for nodes
type ClusterStatsNodesProcessCPUResponse struct {
	Percent int64 `json:"percent"`
}

// ClusterStatsNodesNetworkTypesResponse defines node stats network types information structure for nodes
type ClusterStatsNodesNetworkTypesResponse struct {
	TransportTypes ClusterStatsNodesNetworkTypeResponse `json:"transport_types"`
	HTTPTypes      ClusterStatsNodesNetworkTypeResponse `json:"http_types"`
}

// ClusterStatsNodesNetworkTypeResponse defines node stats network type information structure for nodes
type ClusterStatsNodesNetworkTypeResponse struct {
	Security int64 `json:"security4"`
}
