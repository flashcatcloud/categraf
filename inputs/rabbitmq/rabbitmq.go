package rabbitmq

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/filter"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "rabbitmq"

type RabbitMQ struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &RabbitMQ{}
	})
}

func (r *RabbitMQ) Clone() inputs.Input {
	return &RabbitMQ{}
}

func (r *RabbitMQ) Name() string {
	return inputName
}

func (r *RabbitMQ) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URL      string `toml:"url"`
	Username string `toml:"username"`
	Password string `toml:"password"`

	HeaderTimeout config.Duration `toml:"header_timeout"`
	ClientTimeout config.Duration `toml:"client_timeout"`

	Nodes     []string `toml:"nodes"`
	Exchanges []string `toml:"exchanges"`

	MetricInclude             []string `toml:"metric_include"`
	MetricExclude             []string `toml:"metric_exclude"`
	QueueInclude              []string `toml:"queue_name_include"`
	QueueExclude              []string `toml:"queue_name_exclude"`
	FederationUpstreamInclude []string `toml:"federation_upstream_include"`
	FederationUpstreamExclude []string `toml:"federation_upstream_exclude"`

	tls.ClientConfig
	client *http.Client

	metricFilter   filter.Filter
	queueFilter    filter.Filter
	upstreamFilter filter.Filter

	excludeEveryQueue bool
}

func (ins *Instance) Init() error {
	if ins.URL == "" {
		return types.ErrInstancesEmpty
	}

	var err error

	if err := ins.createQueueFilter(); err != nil {
		return err
	}

	if ins.upstreamFilter, err = filter.NewIncludeExcludeFilter(ins.FederationUpstreamInclude, ins.FederationUpstreamExclude); err != nil {
		return err
	}

	if ins.metricFilter, err = filter.NewIncludeExcludeFilter(ins.MetricInclude, ins.MetricExclude); err != nil {
		return err
	}

	ins.client, err = ins.createHTTPClient()
	return err
}

func (ins *Instance) createQueueFilter() error {
	queueFilter, err := filter.NewIncludeExcludeFilter(ins.QueueInclude, ins.QueueExclude)
	if err != nil {
		return err
	}
	ins.queueFilter = queueFilter

	for _, q := range ins.QueueExclude {
		if q == "*" {
			ins.excludeEveryQueue = true
		}
	}

	return nil
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	if ins.HeaderTimeout <= 0 {
		ins.HeaderTimeout = config.Duration(time.Second * 3)
	}

	if ins.ClientTimeout <= 0 {
		ins.ClientTimeout = config.Duration(time.Second * 4)
	}

	trans := &http.Transport{
		ResponseHeaderTimeout: time.Duration(ins.HeaderTimeout),
	}

	if ins.UseTLS {
		tlsConfig, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return nil, err
		}
		trans.TLSClientConfig = tlsConfig
	}

	client := &http.Client{
		Transport: trans,
		Timeout:   time.Duration(ins.ClientTimeout),
	}

	return client, nil
}

// OverviewResponse ...
type OverviewResponse struct {
	MessageStats *MessageStats `json:"message_stats"`
	ObjectTotals *ObjectTotals `json:"object_totals"`
	QueueTotals  *QueueTotals  `json:"queue_totals"`
	Listeners    []Listeners   `json:"listeners"`
}

// Listeners ...
type Listeners struct {
	Protocol string `json:"protocol"`
}

// Details ...
type Details struct {
	Rate float64 `json:"rate"`
}

// MessageStats ...
type MessageStats struct {
	Ack                     int64
	AckDetails              Details `json:"ack_details"`
	Deliver                 int64
	DeliverDetails          Details `json:"deliver_details"`
	DeliverGet              int64   `json:"deliver_get"`
	DeliverGetDetails       Details `json:"deliver_get_details"`
	Publish                 int64
	PublishDetails          Details `json:"publish_details"`
	Redeliver               int64
	RedeliverDetails        Details `json:"redeliver_details"`
	PublishIn               int64   `json:"publish_in"`
	PublishInDetails        Details `json:"publish_in_details"`
	PublishOut              int64   `json:"publish_out"`
	PublishOutDetails       Details `json:"publish_out_details"`
	ReturnUnroutable        int64   `json:"return_unroutable"`
	ReturnUnroutableDetails Details `json:"return_unroutable_details"`
}

// ObjectTotals ...
type ObjectTotals struct {
	Channels    int64
	Connections int64
	Consumers   int64
	Exchanges   int64
	Queues      int64
}

// QueueTotals ...
type QueueTotals struct {
	Messages                   int64
	MessagesReady              int64 `json:"messages_ready"`
	MessagesUnacknowledged     int64 `json:"messages_unacknowledged"`
	MessageBytes               int64 `json:"message_bytes"`
	MessageBytesReady          int64 `json:"message_bytes_ready"`
	MessageBytesUnacknowledged int64 `json:"message_bytes_unacknowledged"`
	MessageRAM                 int64 `json:"message_bytes_ram"`
	MessagePersistent          int64 `json:"message_bytes_persistent"`
}

// Queue ...
type Queue struct {
	QueueTotals            // just to not repeat the same code
	MessageStats           `json:"message_stats"`
	Memory                 int64
	Consumers              int64
	ConsumerUtilisation    float64 `json:"consumer_utilisation"`
	Name                   string
	Node                   string
	Vhost                  string
	Durable                bool
	AutoDelete             bool     `json:"auto_delete"`
	IdleSince              string   `json:"idle_since"`
	SlaveNodes             []string `json:"slave_nodes"`
	SynchronisedSlaveNodes []string `json:"synchronised_slave_nodes"`
}

// Node ...
type Node struct {
	Name string

	DiskFree                 int64   `json:"disk_free"`
	DiskFreeLimit            int64   `json:"disk_free_limit"`
	DiskFreeAlarm            bool    `json:"disk_free_alarm"`
	FdTotal                  int64   `json:"fd_total"`
	FdUsed                   int64   `json:"fd_used"`
	MemLimit                 int64   `json:"mem_limit"`
	MemUsed                  int64   `json:"mem_used"`
	MemAlarm                 bool    `json:"mem_alarm"`
	ProcTotal                int64   `json:"proc_total"`
	ProcUsed                 int64   `json:"proc_used"`
	RunQueue                 int64   `json:"run_queue"`
	SocketsTotal             int64   `json:"sockets_total"`
	SocketsUsed              int64   `json:"sockets_used"`
	Running                  bool    `json:"running"`
	Uptime                   int64   `json:"uptime"`
	MnesiaDiskTxCount        int64   `json:"mnesia_disk_tx_count"`
	MnesiaDiskTxCountDetails Details `json:"mnesia_disk_tx_count_details"`
	MnesiaRAMTxCount         int64   `json:"mnesia_ram_tx_count"`
	MnesiaRAMTxCountDetails  Details `json:"mnesia_ram_tx_count_details"`
	GcNum                    int64   `json:"gc_num"`
	GcNumDetails             Details `json:"gc_num_details"`
	GcBytesReclaimed         int64   `json:"gc_bytes_reclaimed"`
	GcBytesReclaimedDetails  Details `json:"gc_bytes_reclaimed_details"`
	IoReadAvgTime            float64 `json:"io_read_avg_time"`
	IoReadAvgTimeDetails     Details `json:"io_read_avg_time_details"`
	IoReadBytes              int64   `json:"io_read_bytes"`
	IoReadBytesDetails       Details `json:"io_read_bytes_details"`
	IoWriteAvgTime           float64 `json:"io_write_avg_time"`
	IoWriteAvgTimeDetails    Details `json:"io_write_avg_time_details"`
	IoWriteBytes             int64   `json:"io_write_bytes"`
	IoWriteBytesDetails      Details `json:"io_write_bytes_details"`
}

type Exchange struct {
	Name         string
	MessageStats `json:"message_stats"`
	Type         string
	Internal     bool
	Vhost        string
	Durable      bool
	AutoDelete   bool `json:"auto_delete"`
}

// FederationLinkChannelMessageStats ...
type FederationLinkChannelMessageStats struct {
	Confirm                 int64   `json:"confirm"`
	ConfirmDetails          Details `json:"confirm_details"`
	Publish                 int64   `json:"publish"`
	PublishDetails          Details `json:"publish_details"`
	ReturnUnroutable        int64   `json:"return_unroutable"`
	ReturnUnroutableDetails Details `json:"return_unroutable_details"`
}

// FederationLinkChannel ...
type FederationLinkChannel struct {
	AcksUncommitted        int64                             `json:"acks_uncommitted"`
	ConsumerCount          int64                             `json:"consumer_count"`
	MessagesUnacknowledged int64                             `json:"messages_unacknowledged"`
	MessagesUncommitted    int64                             `json:"messages_uncommitted"`
	MessagesUnconfirmed    int64                             `json:"messages_unconfirmed"`
	MessageStats           FederationLinkChannelMessageStats `json:"message_stats"`
}

// FederationLink ...
type FederationLink struct {
	Type             string                `json:"type"`
	Queue            string                `json:"queue"`
	UpstreamQueue    string                `json:"upstream_queue"`
	Exchange         string                `json:"exchange"`
	UpstreamExchange string                `json:"upstream_exchange"`
	Vhost            string                `json:"vhost"`
	Upstream         string                `json:"upstream"`
	LocalChannel     FederationLinkChannel `json:"local_channel"`
}

type HealthCheck struct {
	Status string `json:"status"`
}

// MemoryResponse ...
type MemoryResponse struct {
	Memory *Memory `json:"memory"`
}

// Memory details
type Memory struct {
	ConnectionReaders   int64       `json:"connection_readers"`
	ConnectionWriters   int64       `json:"connection_writers"`
	ConnectionChannels  int64       `json:"connection_channels"`
	ConnectionOther     int64       `json:"connection_other"`
	QueueProcs          int64       `json:"queue_procs"`
	QueueSlaveProcs     int64       `json:"queue_slave_procs"`
	Plugins             int64       `json:"plugins"`
	OtherProc           int64       `json:"other_proc"`
	Metrics             int64       `json:"metrics"`
	MgmtDb              int64       `json:"mgmt_db"`
	Mnesia              int64       `json:"mnesia"`
	OtherEts            int64       `json:"other_ets"`
	Binary              int64       `json:"binary"`
	MsgIndex            int64       `json:"msg_index"`
	Code                int64       `json:"code"`
	Atom                int64       `json:"atom"`
	OtherSystem         int64       `json:"other_system"`
	AllocatedUnused     int64       `json:"allocated_unused"`
	ReservedUnallocated int64       `json:"reserved_unallocated"`
	Total               interface{} `json:"total"`
}

// Error response
type ErrorResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

// gatherFunc ...
type gatherFunc func(ins *Instance, slist *types.SampleList)

var gatherFunctions = map[string]gatherFunc{
	"exchange":   gatherExchanges,
	"federation": gatherFederationLinks,
	"node":       gatherNodes,
	"overview":   gatherOverview,
	"queue":      gatherQueues,
}

func (ins *Instance) Gather(slist *types.SampleList) {
	tags := map[string]string{"url": ins.URL}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use, tags))
	}(begun)

	var wg sync.WaitGroup
	for name, f := range gatherFunctions {
		// Query only metrics that are supported
		if !ins.metricFilter.Match(name) {
			continue
		}
		wg.Add(1)
		go func(gf gatherFunc) {
			defer wg.Done()
			gf(ins, slist)
		}(f)
	}
	wg.Wait()

}

func (ins *Instance) requestEndpoint(u string) ([]byte, error) {
	endpoint := ins.URL + u

	if config.Config.DebugMode {
		log.Println("D! requesting:", endpoint)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(ins.Username, ins.Password)

	resp, err := ins.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("getting %q failed: %v %v", u, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return io.ReadAll(resp.Body)
}

func (ins *Instance) requestJSON(u string, target interface{}) error {
	buf, err := ins.requestEndpoint(u)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(buf, target); err != nil {
		if _, ok := err.(*json.UnmarshalTypeError); ok {
			// Try to get the error reason from the response
			var errResponse ErrorResponse
			if json.Unmarshal(buf, &errResponse) == nil && errResponse.Error != "" {
				// Return the error reason in the response
				return fmt.Errorf("error response trying to get %q: %q (reason: %q)", u, errResponse.Error, errResponse.Reason)
			}
		}

		return fmt.Errorf("decoding answer from %q failed: %v", u, err)
	}

	return nil
}

func gatherOverview(ins *Instance, slist *types.SampleList) {
	overview := OverviewResponse{}

	err := ins.requestJSON("/api/overview", &overview)
	if err != nil {
		log.Println("E! failed to query rabbitmq /api/overview:", err)
		return
	}

	if overview.QueueTotals == nil || overview.ObjectTotals == nil || overview.MessageStats == nil || overview.Listeners == nil {
		log.Println("E! wrong answer from rabbitmq. probably auth issue")
		return
	}

	var clusteringListeners, amqpListeners int64 = 0, 0
	for _, listener := range overview.Listeners {
		if listener.Protocol == "clustering" {
			clusteringListeners++
		} else if listener.Protocol == "amqp" {
			amqpListeners++
		}
	}

	tags := map[string]string{"url": ins.URL}

	fields := map[string]interface{}{
		"overview_messages":                  overview.QueueTotals.Messages,
		"overview_messages_ready":            overview.QueueTotals.MessagesReady,
		"overview_messages_unacked":          overview.QueueTotals.MessagesUnacknowledged,
		"overview_channels":                  overview.ObjectTotals.Channels,
		"overview_connections":               overview.ObjectTotals.Connections,
		"overview_consumers":                 overview.ObjectTotals.Consumers,
		"overview_exchanges":                 overview.ObjectTotals.Exchanges,
		"overview_queues":                    overview.ObjectTotals.Queues,
		"overview_messages_acked":            overview.MessageStats.Ack,
		"overview_messages_acked_rate":       overview.MessageStats.AckDetails.Rate,
		"overview_messages_delivered":        overview.MessageStats.Deliver,
		"overview_messages_delivered_rate":   overview.MessageStats.DeliverDetails.Rate,
		"overview_messages_redelivered":      overview.MessageStats.Redeliver,
		"overview_messages_redelivered_rate": overview.MessageStats.RedeliverDetails.Rate,
		"overview_messages_delivered_get":    overview.MessageStats.DeliverGet,
		"overview_messages_published":        overview.MessageStats.Publish,
		"overview_clustering_listeners":      clusteringListeners,
		"overview_amqp_listeners":            amqpListeners,
		"overview_return_unroutable":         overview.MessageStats.ReturnUnroutable,
		"overview_return_unroutable_rate":    overview.MessageStats.ReturnUnroutableDetails.Rate,
	}

	slist.PushSamples(inputName, fields, tags)
}

func gatherExchanges(ins *Instance, slist *types.SampleList) {
	// Gather information about exchanges
	exchanges := make([]Exchange, 0)
	err := ins.requestJSON("/api/exchanges", &exchanges)
	if err != nil {
		log.Println("E! failed to query rabbitmq /api/exchanges:", err)
		return
	}

	for _, exchange := range exchanges {
		if !ins.shouldGatherExchange(exchange.Name) {
			continue
		}
		tags := map[string]string{
			"url":      ins.URL,
			"exchange": exchange.Name,
			"type":     exchange.Type,
			"vhost":    exchange.Vhost,
			// "internal":    strconv.FormatBool(exchange.Internal),
			// "durable":     strconv.FormatBool(exchange.Durable),
			// "auto_delete": strconv.FormatBool(exchange.AutoDelete),
		}

		fields := map[string]interface{}{
			"exchange_messages_publish_in":       exchange.MessageStats.PublishIn,
			"exchange_messages_publish_in_rate":  exchange.MessageStats.PublishInDetails.Rate,
			"exchange_messages_publish_out":      exchange.MessageStats.PublishOut,
			"exchange_messages_publish_out_rate": exchange.MessageStats.PublishOutDetails.Rate,
		}

		slist.PushSamples(inputName, fields, tags)
	}
}

func (ins *Instance) shouldGatherExchange(exchangeName string) bool {
	if len(ins.Exchanges) == 0 {
		return true
	}

	for _, name := range ins.Exchanges {
		if name == exchangeName {
			return true
		}
	}

	return false
}

func gatherFederationLinks(ins *Instance, slist *types.SampleList) {
	// Gather information about federation links
	federationLinks := make([]FederationLink, 0)
	err := ins.requestJSON("/api/federation-links", &federationLinks)
	if err != nil {
		log.Println("E! failed to query rabbitmq /api/federation-links:", err)
		return
	}

	for _, link := range federationLinks {
		if !ins.shouldGatherFederationLink(link) {
			continue
		}

		tags := map[string]string{
			"url":      ins.URL,
			"type":     link.Type,
			"vhost":    link.Vhost,
			"upstream": link.Upstream,
		}

		if link.Type == "exchange" {
			tags["exchange"] = link.Exchange
			tags["upstream_exchange"] = link.UpstreamExchange
		} else {
			tags["queue"] = link.Queue
			tags["upstream_queue"] = link.UpstreamQueue
		}

		fields := map[string]interface{}{
			"federation_acks_uncommitted":           link.LocalChannel.AcksUncommitted,
			"federation_consumers":                  link.LocalChannel.ConsumerCount,
			"federation_messages_unacknowledged":    link.LocalChannel.MessagesUnacknowledged,
			"federation_messages_uncommitted":       link.LocalChannel.MessagesUncommitted,
			"federation_messages_unconfirmed":       link.LocalChannel.MessagesUnconfirmed,
			"federation_messages_confirm":           link.LocalChannel.MessageStats.Confirm,
			"federation_messages_publish":           link.LocalChannel.MessageStats.Publish,
			"federation_messages_return_unroutable": link.LocalChannel.MessageStats.ReturnUnroutable,
		}

		slist.PushSamples(inputName, fields, tags)
	}
}

func (ins *Instance) shouldGatherFederationLink(link FederationLink) bool {
	if !ins.upstreamFilter.Match(link.Upstream) {
		return false
	}

	switch link.Type {
	case "exchange":
		return ins.shouldGatherExchange(link.Exchange)
	case "queue":
		return ins.queueFilter.Match(link.Queue)
	default:
		return false
	}
}

func gatherNodes(ins *Instance, slist *types.SampleList) {
	allNodes := make([]*Node, 0)

	err := ins.requestJSON("/api/nodes", &allNodes)
	if err != nil {
		log.Println("E! failed to query rabbitmq /api/nodes:", err)
		return
	}

	nodes := allNodes[:0]
	for _, node := range allNodes {
		if ins.shouldGatherNode(node) {
			nodes = append(nodes, node)
		}
	}

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(node *Node) {
			defer wg.Done()

			tags := map[string]string{"url": ins.URL}
			tags["node"] = node.Name

			fields := map[string]interface{}{
				"node_disk_free":                 node.DiskFree,
				"node_disk_free_limit":           node.DiskFreeLimit,
				"node_disk_free_alarm":           boolToInt(node.DiskFreeAlarm),
				"node_fd_total":                  node.FdTotal,
				"node_fd_used":                   node.FdUsed,
				"node_mem_limit":                 node.MemLimit,
				"node_mem_used":                  node.MemUsed,
				"node_mem_alarm":                 boolToInt(node.MemAlarm),
				"node_proc_total":                node.ProcTotal,
				"node_proc_used":                 node.ProcUsed,
				"node_run_queue":                 node.RunQueue,
				"node_sockets_total":             node.SocketsTotal,
				"node_sockets_used":              node.SocketsUsed,
				"node_uptime":                    node.Uptime,
				"node_mnesia_disk_tx_count":      node.MnesiaDiskTxCount,
				"node_mnesia_disk_tx_count_rate": node.MnesiaDiskTxCountDetails.Rate,
				"node_mnesia_ram_tx_count":       node.MnesiaRAMTxCount,
				"node_mnesia_ram_tx_count_rate":  node.MnesiaRAMTxCountDetails.Rate,
				"node_gc_num":                    node.GcNum,
				"node_gc_num_rate":               node.GcNumDetails.Rate,
				"node_gc_bytes_reclaimed":        node.GcBytesReclaimed,
				"node_gc_bytes_reclaimed_rate":   node.GcBytesReclaimedDetails.Rate,
				"node_io_read_avg_time":          node.IoReadAvgTime,
				"node_io_read_avg_time_rate":     node.IoReadAvgTimeDetails.Rate,
				"node_io_read_bytes":             node.IoReadBytes,
				"node_io_read_bytes_rate":        node.IoReadBytesDetails.Rate,
				"node_io_write_avg_time":         node.IoWriteAvgTime,
				"node_io_write_avg_time_rate":    node.IoWriteAvgTimeDetails.Rate,
				"node_io_write_bytes":            node.IoWriteBytes,
				"node_io_write_bytes_rate":       node.IoWriteBytesDetails.Rate,
				"node_running":                   boolToInt(node.Running),
			}

			var memory MemoryResponse
			err = ins.requestJSON("/api/nodes/"+node.Name+"/memory", &memory)
			if err != nil {
				log.Println("E! failed to query rabbitmq /api/nodes/"+node.Name+"/memory:", err)
				return
			}

			if memory.Memory != nil {
				fields["node_mem_connection_readers"] = memory.Memory.ConnectionReaders
				fields["node_mem_connection_writers"] = memory.Memory.ConnectionWriters
				fields["node_mem_connection_channels"] = memory.Memory.ConnectionChannels
				fields["node_mem_connection_other"] = memory.Memory.ConnectionOther
				fields["node_mem_queue_procs"] = memory.Memory.QueueProcs
				fields["node_mem_queue_slave_procs"] = memory.Memory.QueueSlaveProcs
				fields["node_mem_plugins"] = memory.Memory.Plugins
				fields["node_mem_other_proc"] = memory.Memory.OtherProc
				fields["node_mem_metrics"] = memory.Memory.Metrics
				fields["node_mem_mgmt_db"] = memory.Memory.MgmtDb
				fields["node_mem_mnesia"] = memory.Memory.Mnesia
				fields["node_mem_other_ets"] = memory.Memory.OtherEts
				fields["node_mem_binary"] = memory.Memory.Binary
				fields["node_mem_msg_index"] = memory.Memory.MsgIndex
				fields["node_mem_code"] = memory.Memory.Code
				fields["node_mem_atom"] = memory.Memory.Atom
				fields["node_mem_other_system"] = memory.Memory.OtherSystem
				fields["node_mem_allocated_unused"] = memory.Memory.AllocatedUnused
				fields["node_mem_reserved_unallocated"] = memory.Memory.ReservedUnallocated
				switch v := memory.Memory.Total.(type) {
				case float64:
					fields["node_mem_total"] = int64(v)
				case map[string]interface{}:
					var foundEstimator bool
					for _, estimator := range []string{"rss", "allocated", "erlang"} {
						if x, found := v[estimator]; found {
							if total, ok := x.(float64); ok {
								fields["node_mem_total"] = int64(total)
								foundEstimator = true
								break
							}

							msg := fmt.Sprintf("unknown type %T for %q total memory", x, estimator)
							log.Println("E!", msg)
						}
					}
					if !foundEstimator {
						log.Println("E! no known memory estimation in", v)
					}
				default:
					log.Println("E! unknown type", memory.Memory.Total, "for total memory")
				}
			}

			slist.PushSamples(inputName, fields, tags)
		}(node)
	}

	wg.Wait()
}

func (ins *Instance) shouldGatherNode(node *Node) bool {
	if len(ins.Nodes) == 0 {
		return true
	}

	for _, name := range ins.Nodes {
		if name == node.Name {
			return true
		}
	}

	return false
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func gatherQueues(ins *Instance, slist *types.SampleList) {
	if ins.excludeEveryQueue {
		return
	}

	// Gather information about queues
	queues := make([]Queue, 0)
	err := ins.requestJSON("/api/queues", &queues)
	if err != nil {
		log.Println("E! failed to query rabbitmq /api/queues:", err)
		return
	}

	for _, queue := range queues {
		if !ins.queueFilter.Match(queue.Name) {
			continue
		}

		tags := map[string]string{
			"url":   ins.URL,
			"queue": queue.Name,
			"vhost": queue.Vhost,
			"node":  queue.Node,
			// "durable":     strconv.FormatBool(queue.Durable),
			// "auto_delete": strconv.FormatBool(queue.AutoDelete),
		}

		fields := map[string]interface{}{
			// common information
			"queue_consumers":                queue.Consumers,
			"queue_consumer_utilisation":     queue.ConsumerUtilisation,
			"queue_idle_since":               queue.IdleSince,
			"queue_slave_nodes":              len(queue.SlaveNodes),
			"queue_synchronised_slave_nodes": len(queue.SynchronisedSlaveNodes),
			"queue_memory":                   queue.Memory,
			// messages information
			"queue_message_bytes":             queue.MessageBytes,
			"queue_message_bytes_ready":       queue.MessageBytesReady,
			"queue_message_bytes_unacked":     queue.MessageBytesUnacknowledged,
			"queue_message_bytes_ram":         queue.MessageRAM,
			"queue_message_bytes_persist":     queue.MessagePersistent,
			"queue_messages":                  queue.Messages,
			"queue_messages_ready":            queue.MessagesReady,
			"queue_messages_unack":            queue.MessagesUnacknowledged,
			"queue_messages_ack":              queue.MessageStats.Ack,
			"queue_messages_ack_rate":         queue.MessageStats.AckDetails.Rate,
			"queue_messages_deliver":          queue.MessageStats.Deliver,
			"queue_messages_deliver_rate":     queue.MessageStats.DeliverDetails.Rate,
			"queue_messages_deliver_get":      queue.MessageStats.DeliverGet,
			"queue_messages_deliver_get_rate": queue.MessageStats.DeliverGetDetails.Rate,
			"queue_messages_publish":          queue.MessageStats.Publish,
			"queue_messages_publish_rate":     queue.MessageStats.PublishDetails.Rate,
			"queue_messages_redeliver":        queue.MessageStats.Redeliver,
			"queue_messages_redeliver_rate":   queue.MessageStats.RedeliverDetails.Rate,
		}

		slist.PushSamples(inputName, fields, tags)
	}
}
