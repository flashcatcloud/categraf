package exporter

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/krallistic/kazoo-go"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "kafka"
	clientID  = "kafka_exporter"
)

type PromDesc struct {
	up                                       *prometheus.Desc
	clusterBrokers                           *prometheus.Desc
	clusterBrokerInfo                        *prometheus.Desc
	topicPartitions                          *prometheus.Desc
	topicCurrentOffset                       *prometheus.Desc
	topicOldestOffset                        *prometheus.Desc
	topicPartitionLeader                     *prometheus.Desc
	topicPartitionReplicas                   *prometheus.Desc
	topicPartitionInSyncReplicas             *prometheus.Desc
	topicPartitionUsesPreferredReplica       *prometheus.Desc
	topicUnderReplicatedPartition            *prometheus.Desc
	consumergroupCurrentOffset               *prometheus.Desc
	consumergroupCurrentOffsetSum            *prometheus.Desc
	consumergroupUncomittedOffsets           *prometheus.Desc
	consumergroupUncommittedOffsetsSum       *prometheus.Desc
	consumergroupUncommittedOffsetsZookeeper *prometheus.Desc
	consumergroupMembers                     *prometheus.Desc
	topicPartitionLagMillis                  *prometheus.Desc
	lagDatapointUsedInterpolation            *prometheus.Desc
	lagDatapointUsedExtrapolation            *prometheus.Desc
}

// Exporter collects Kafka stats from the given server and exports them using
// the prometheus metrics package.
type Exporter struct {
	client                     sarama.Client
	topicFilter                *regexp.Regexp
	topicExclude               *regexp.Regexp
	groupFilter                *regexp.Regexp
	groupExclude               *regexp.Regexp
	mu                         sync.Mutex
	useZooKeeperLag            bool
	zookeeperClient            *kazoo.Kazoo
	nextMetadataRefresh        time.Time
	metadataRefreshInterval    time.Duration
	offsetShowAll              bool
	allowConcurrent            bool
	sgMutex                    sync.Mutex
	sgWaitCh                   chan struct{}
	sgChans                    []chan<- prometheus.Metric
	consumerGroupFetchAll      bool
	consumerGroupLagTable      interpolationMap
	kafkaOpts                  Options
	saramaConfig               *sarama.Config
	logger                     log.Logger
	promDesc                   *PromDesc
	disableCalculateLagRate    bool
	renameUncommitOffsetsToLag bool
	quitPruneCh                chan struct{}
}

type Options struct {
	Uri                        []string
	UseSASL                    bool
	UseSASLHandshake           bool
	SaslUsername               string
	SaslPassword               string
	SaslMechanism              string
	UseTLS                     bool
	TlsCAFile                  string
	TlsCertFile                string
	TlsKeyFile                 string
	TlsInsecureSkipTLSVerify   bool
	KafkaVersion               string
	UseZooKeeperLag            bool
	UriZookeeper               []string
	Labels                     string
	MetadataRefreshInterval    string
	OffsetShowAll              bool
	AllowConcurrent            bool
	MaxOffsets                 int
	PruneIntervalSeconds       int
	DisableCalculateLagRate    bool
	RenameUncommitOffsetsToLag bool

	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// CanReadCertAndKey returns true if the certificate and key files already exists,
// otherwise returns false. If lost one of cert and key, returns error.
func CanReadCertAndKey(certPath, keyPath string) (bool, error) {
	certReadable := canReadFile(certPath)
	keyReadable := canReadFile(keyPath)

	if certReadable == false && keyReadable == false {
		return false, nil
	}

	if certReadable == false {
		return false, fmt.Errorf("error reading %s, certificate and key must be supplied as a pair", certPath)
	}

	if keyReadable == false {
		return false, fmt.Errorf("error reading %s, certificate and key must be supplied as a pair", keyPath)
	}

	return true, nil
}

// If the file represented by path exists and
// readable, returns true otherwise returns false.
func canReadFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}

	defer f.Close()

	return true
}

// New returns an initialized Exporter.
func New(logger log.Logger, opts Options, topicFilter, topicExclude, groupFilter, groupExclude string) (*Exporter, error) {
	var zookeeperClient *kazoo.Kazoo
	config := sarama.NewConfig()
	config.ClientID = clientID
	kafkaVersion, err := sarama.ParseKafkaVersion(opts.KafkaVersion)
	if err != nil {
		return nil, err
	}
	config.Version = kafkaVersion

	config.Net.DialTimeout = opts.DialTimeout
	config.Net.ReadTimeout = opts.ReadTimeout
	config.Net.WriteTimeout = opts.WriteTimeout

	if opts.UseSASL {
		// Convert to lowercase so that SHA512 and SHA256 is still valid
		opts.SaslMechanism = strings.ToLower(opts.SaslMechanism)
		switch opts.SaslMechanism {
		case "scram-sha512":
			config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient { return &XDGSCRAMClient{HashGeneratorFcn: SHA512} }
			config.Net.SASL.Mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA512)
		case "scram-sha256":
			config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient { return &XDGSCRAMClient{HashGeneratorFcn: SHA256} }
			config.Net.SASL.Mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA256)

		case "plain":
		default:
			level.Error(logger).Log("msg", "invalid sasl mechanism. can only be \"scram-sha256\", \"scram-sha512\" or \"plain\"", "SaslMechanism", opts.SaslMechanism)
			return nil, fmt.Errorf("invalid sasl mechanism \"%s\": can only be \"scram-sha256\", \"scram-sha512\" or \"plain\"", opts.SaslMechanism)
		}

		config.Net.SASL.Enable = true
		config.Net.SASL.Handshake = opts.UseSASLHandshake

		if opts.SaslUsername != "" {
			config.Net.SASL.User = opts.SaslUsername
		}

		if opts.SaslPassword != "" {
			config.Net.SASL.Password = opts.SaslPassword
		}
	}

	if opts.UseTLS {
		config.Net.TLS.Enable = true

		config.Net.TLS.Config = &tls.Config{
			RootCAs:            x509.NewCertPool(),
			InsecureSkipVerify: opts.TlsInsecureSkipTLSVerify,
		}

		if opts.TlsCAFile != "" {
			if ca, err := os.ReadFile(opts.TlsCAFile); err == nil {
				config.Net.TLS.Config.RootCAs.AppendCertsFromPEM(ca)
			} else {
				level.Error(logger).Log("msg", "unable to open TlsCAFile", "TlsCAFile", opts.TlsCAFile)
				return nil, fmt.Errorf("UseTLS is true but unable to open TlsCAFile: %s", opts.TlsCAFile)
			}
		}

		canReadCertAndKey, err := CanReadCertAndKey(opts.TlsCertFile, opts.TlsKeyFile)
		if err != nil {
			level.Error(logger).Log("msg", "Error attempting to read TlsCertFile or TlsKeyFile", "err", err.Error())
			return nil, err
		}
		if canReadCertAndKey {
			cert, err := tls.LoadX509KeyPair(opts.TlsCertFile, opts.TlsKeyFile)
			if err == nil {
				config.Net.TLS.Config.Certificates = []tls.Certificate{cert}
			} else {
				level.Error(logger).Log("msg", "Error attempting to load X509KeyPair", "err", err.Error())
				return nil, err
			}
		}
	}

	if opts.UseZooKeeperLag {
		zookeeperClient, err = kazoo.NewKazoo(opts.UriZookeeper, nil)
		if err != nil {
			level.Error(logger).Log("msg", "Error connecting to ZooKeeper", "err", err.Error())
			return nil, err
		}
	}

	interval, err := time.ParseDuration(opts.MetadataRefreshInterval)
	if err != nil {
		level.Error(logger).Log("msg", "Error parsing metadata refresh interval", "err", err.Error())
		return nil, err
	}

	config.Metadata.RefreshFrequency = interval

	client, err := sarama.NewClient(opts.Uri, config)

	if err != nil {
		level.Error(logger).Log("msg", "Error initiating kafka client: %s", "err", err.Error())
		return nil, err
	}
	level.Debug(logger).Log("msg", "Done with kafka client initialization")

	// Init our exporter.
	newExporter := &Exporter{
		client:                     client,
		topicFilter:                regexp.MustCompile(topicFilter),
		topicExclude:               regexp.MustCompile(topicExclude),
		groupFilter:                regexp.MustCompile(groupFilter),
		groupExclude:               regexp.MustCompile(groupExclude),
		useZooKeeperLag:            opts.UseZooKeeperLag,
		zookeeperClient:            zookeeperClient,
		nextMetadataRefresh:        time.Now(),
		metadataRefreshInterval:    interval,
		offsetShowAll:              opts.OffsetShowAll,
		allowConcurrent:            opts.AllowConcurrent,
		sgMutex:                    sync.Mutex{},
		sgWaitCh:                   nil,
		sgChans:                    []chan<- prometheus.Metric{},
		consumerGroupFetchAll:      config.Version.IsAtLeast(sarama.V2_0_0_0),
		consumerGroupLagTable:      interpolationMap{mu: sync.Mutex{}},
		kafkaOpts:                  opts,
		saramaConfig:               config,
		logger:                     logger,
		promDesc:                   nil, // initialized in func initializeMetrics
		disableCalculateLagRate:    opts.DisableCalculateLagRate,
		renameUncommitOffsetsToLag: opts.RenameUncommitOffsetsToLag,
	}

	level.Debug(logger).Log("msg", "Initializing metrics")
	newExporter.initializeMetrics()

	if !newExporter.disableCalculateLagRate {
		newExporter.quitPruneCh = make(chan struct{})
		go newExporter.RunPruner()
	}
	return newExporter, nil
}

func (e *Exporter) fetchOffsetVersion() int16 {
	version := e.client.Config().Version
	if e.client.Config().Version.IsAtLeast(sarama.V2_0_0_0) {
		return 4
	} else if version.IsAtLeast(sarama.V0_10_2_0) {
		return 2
	} else if version.IsAtLeast(sarama.V0_8_2_2) {
		return 1
	}
	return 0
}

// Describe describes all the metrics ever exported by the Kafka exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.promDesc.up
	ch <- e.promDesc.clusterBrokers
	ch <- e.promDesc.clusterBrokerInfo
	ch <- e.promDesc.topicCurrentOffset
	ch <- e.promDesc.topicOldestOffset
	ch <- e.promDesc.topicPartitions
	ch <- e.promDesc.topicPartitionLeader
	ch <- e.promDesc.topicPartitionReplicas
	ch <- e.promDesc.topicPartitionInSyncReplicas
	ch <- e.promDesc.topicPartitionUsesPreferredReplica
	ch <- e.promDesc.topicUnderReplicatedPartition
	ch <- e.promDesc.consumergroupCurrentOffset
	ch <- e.promDesc.consumergroupCurrentOffsetSum
	ch <- e.promDesc.consumergroupUncomittedOffsets
	ch <- e.promDesc.consumergroupUncommittedOffsetsZookeeper
	ch <- e.promDesc.consumergroupUncommittedOffsetsSum
	ch <- e.promDesc.topicPartitionLagMillis
	ch <- e.promDesc.lagDatapointUsedInterpolation
	ch <- e.promDesc.lagDatapointUsedExtrapolation
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	if e.allowConcurrent {
		e.collect(ch)
		return
	}
	// Locking to avoid race add
	e.sgMutex.Lock()
	e.sgChans = append(e.sgChans, ch)
	// Safe to compare length since we own the Lock
	if len(e.sgChans) == 1 {
		e.sgWaitCh = make(chan struct{})
		go e.collectChans(e.sgWaitCh)
	} else {
		level.Info(e.logger).Log("msg", "concurrent calls detected, waiting for first to finish")
	}
	// Put in another variable to ensure not overwriting it in another Collect once we wait
	waiter := e.sgWaitCh
	e.sgMutex.Unlock()
	// Released lock, we have insurance that our chan will be part of the collectChan slice
	<-waiter
	// collectChan finished
}

// Collect fetches the stats from configured Kafka location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) collectChans(quit chan struct{}) {
	original := make(chan prometheus.Metric)
	container := make([]prometheus.Metric, 0, 100)
	go func() {
		for metric := range original {
			container = append(container, metric)
		}
	}()
	e.collect(original)
	close(original)
	// Lock to avoid modification on the channel slice
	e.sgMutex.Lock()
	for _, ch := range e.sgChans {
		for _, metric := range container {
			ch <- metric
		}
	}
	// Reset the slice
	e.sgChans = e.sgChans[:0]
	// Notify remaining waiting Collect they can return
	close(quit)
	// Release the lock so Collect can append to the slice again
	e.sgMutex.Unlock()
}

func (e *Exporter) collect(ch chan<- prometheus.Metric) {
	var wg = sync.WaitGroup{}
	ch <- prometheus.MustNewConstMetric(
		e.promDesc.clusterBrokers, prometheus.GaugeValue, float64(len(e.client.Brokers())),
	)
	for _, b := range e.client.Brokers() {
		ch <- prometheus.MustNewConstMetric(
			e.promDesc.clusterBrokerInfo, prometheus.GaugeValue, 1, strconv.Itoa(int(b.ID())), b.Addr(),
		)
	}
	offsetMap := make(map[string]map[int32]int64)

	now := time.Now()

	if now.After(e.nextMetadataRefresh) {
		level.Info(e.logger).Log("msg", "Refreshing client metadata")
		if err := e.client.RefreshMetadata(); err != nil {
			level.Error(e.logger).Log("msg", "Error refreshing topics. Using cached topic data", "err", err.Error())
		}

		e.nextMetadataRefresh = now.Add(e.metadataRefreshInterval)
	}

	var value float64
	defer func() {
		ch <- prometheus.MustNewConstMetric(e.promDesc.up, prometheus.GaugeValue, value)
	}()

	topics, err := e.client.Topics()
	if err != nil {
		level.Error(e.logger).Log("msg", "Error getting topics: %s. Skipping metric generation", "err", err.Error())
		return
	}

	value = 1

	level.Info(e.logger).Log("msg", "Generating topic metrics")
	for _, topic := range topics {
		wg.Add(1)
		topic := topic
		go func() {
			defer wg.Done()
			e.metricsForTopic(topic, offsetMap, ch)
		}()
	}

	level.Debug(e.logger).Log("msg", "waiting for topic metric generation to complete")
	wg.Wait()

	level.Info(e.logger).Log("msg", "Generating consumergroup metrics")
	if len(e.client.Brokers()) > 0 {
		for _, broker := range e.client.Brokers() {
			wg.Add(1)

			broker := broker
			go func() {
				defer wg.Done()
				e.metricsForConsumerGroup(broker, offsetMap, ch)
			}()
		}
		level.Debug(e.logger).Log("msg", "waiting for consumergroup metric generation to complete")
		wg.Wait()
	} else {
		level.Error(e.logger).Log("msg", "No brokers found. Unable to generate topic metrics")
	}

	if !e.disableCalculateLagRate {
		level.Info(e.logger).Log("msg", "Calculating consumergroup lag")
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.metricsForLag(ch)
		}()
		level.Debug(e.logger).Log("msg", "waiting for consumergroup lag estimation metric generation to complete")
		wg.Wait()
	}

}

func (e *Exporter) metricsForTopic(topic string, offsetMap map[string]map[int32]int64, ch chan<- prometheus.Metric) {
	if !e.topicFilter.MatchString(topic) || e.topicExclude.MatchString(topic) {
		return
	}

	level.Debug(e.logger).Log("msg", "Fetching topic metrics", "topic", topic)
	partitions, err := e.client.Partitions(topic)
	if err != nil {
		level.Error(e.logger).Log("msg", "Error getting partitions for topic", "topic", topic, "err", err.Error())
		return
	}
	ch <- prometheus.MustNewConstMetric(
		e.promDesc.topicPartitions, prometheus.GaugeValue, float64(len(partitions)), topic,
	)
	e.mu.Lock()
	offset := make(map[int32]int64, len(partitions))
	e.mu.Unlock()
	for _, partition := range partitions {
		broker, err := e.client.Leader(topic, partition)
		if err != nil {
			level.Error(e.logger).Log("msg", "Error getting leader for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicPartitionLeader, prometheus.GaugeValue, float64(broker.ID()), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		currentOffset, err := e.client.GetOffset(topic, partition, sarama.OffsetNewest)
		if err != nil {
			level.Error(e.logger).Log("msg", "Error getting offset for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
		} else {
			e.mu.Lock()
			offset[partition] = currentOffset
			offsetMap[topic] = offset
			e.mu.Unlock()
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicCurrentOffset, prometheus.GaugeValue, float64(currentOffset), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		oldestOffset, err := e.client.GetOffset(topic, partition, sarama.OffsetOldest)
		if err != nil {
			level.Error(e.logger).Log("msg", "Error getting oldest offset for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicOldestOffset, prometheus.GaugeValue, float64(oldestOffset), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		replicas, err := e.client.Replicas(topic, partition)
		if err != nil {
			level.Error(e.logger).Log("msg", "Error getting replicas for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicPartitionReplicas, prometheus.GaugeValue, float64(len(replicas)), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		inSyncReplicas, err := e.client.InSyncReplicas(topic, partition)
		if err != nil {
			level.Error(e.logger).Log("msg", "Error getting in-sync replicas for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicPartitionInSyncReplicas, prometheus.GaugeValue, float64(len(inSyncReplicas)), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		if broker != nil && replicas != nil && len(replicas) > 0 && broker.ID() == replicas[0] {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicPartitionUsesPreferredReplica, prometheus.GaugeValue, float64(1), topic, strconv.FormatInt(int64(partition), 10),
			)
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicPartitionUsesPreferredReplica, prometheus.GaugeValue, float64(0), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		if replicas != nil && inSyncReplicas != nil && len(inSyncReplicas) < len(replicas) {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicUnderReplicatedPartition, prometheus.GaugeValue, float64(1), topic, strconv.FormatInt(int64(partition), 10),
			)
		} else {
			ch <- prometheus.MustNewConstMetric(
				e.promDesc.topicUnderReplicatedPartition, prometheus.GaugeValue, float64(0), topic, strconv.FormatInt(int64(partition), 10),
			)
		}

		if e.useZooKeeperLag {
			ConsumerGroups, err := e.zookeeperClient.Consumergroups()

			if err != nil {
				level.Error(e.logger).Log("msg", "Error getting consumergroups from ZooKeeper", "err", err.Error())
			}

			for _, group := range ConsumerGroups {
				offset, _ := group.FetchOffset(topic, partition)
				if offset > 0 {

					consumerGroupLag := currentOffset - offset
					ch <- prometheus.MustNewConstMetric(
						e.promDesc.consumergroupUncommittedOffsetsZookeeper, prometheus.GaugeValue, float64(consumerGroupLag), group.Name, topic, strconv.FormatInt(int64(partition), 10),
					)
				}
			}
		}
	}
}

func (e *Exporter) metricsForConsumerGroup(broker *sarama.Broker, offsetMap map[string]map[int32]int64, ch chan<- prometheus.Metric) {
	level.Debug(e.logger).Log("msg", "Fetching consumer group metrics for broker", "broker", broker.ID(), "broker_addr", broker.Addr())
	if err := broker.Open(e.client.Config()); err != nil && err != sarama.ErrAlreadyConnected {
		level.Error(e.logger).Log("msg", "Error connecting to broker", "broker", broker.ID(), "broker_addr", broker.Addr(), "err", err.Error())
		return
	}
	defer broker.Close()

	level.Debug(e.logger).Log("msg", "listing consumergroups for broker", "broker", broker.ID(), "broker_addr", broker.Addr())
	groups, err := broker.ListGroups(&sarama.ListGroupsRequest{})
	if err != nil {
		level.Error(e.logger).Log("msg", "Error listing consumergroups for broker", "broker", broker.ID(), "broker_addr", broker.Addr(), "err", err.Error())
		return
	}
	groupIds := make([]string, 0)
	for groupId := range groups.Groups {
		if e.groupFilter.MatchString(groupId) && !e.groupExclude.MatchString(groupId) {
			groupIds = append(groupIds, groupId)
		}
	}
	level.Debug(e.logger).Log("msg", "describing consumergroups for broker", "broker", broker.ID())
	describeGroups, err := broker.DescribeGroups(&sarama.DescribeGroupsRequest{Groups: groupIds})
	if err != nil {
		level.Error(e.logger).Log("msg", "Error from broker.DescribeGroups()", "err", err.Error())
		return
	}
	for _, group := range describeGroups.Groups {
		offsetFetchRequest := sarama.OffsetFetchRequest{ConsumerGroup: group.GroupId, Version: 1}
		if e.offsetShowAll {
			for topic, partitions := range offsetMap {
				for partition := range partitions {
					offsetFetchRequest.AddPartition(topic, partition)
				}
			}
		} else {
			for _, member := range group.Members {
				assignment, err := member.GetMemberAssignment()
				if err != nil {
					level.Error(e.logger).Log("msg", "Cannot get GetMemberAssignment of group member", "member", member, "err", err.Error())
					return
				}
				for topic, partions := range assignment.Topics {
					for _, partition := range partions {
						offsetFetchRequest.AddPartition(topic, partition)
					}
				}
			}
		}

		ch <- prometheus.MustNewConstMetric(
			e.promDesc.consumergroupMembers, prometheus.GaugeValue, float64(len(group.Members)), group.GroupId,
		)
		level.Debug(e.logger).Log("msg", "fetching offsets for broker/group", "broker", broker.ID(), "group", group.GroupId)
		if offsetFetchResponse, err := broker.FetchOffset(&offsetFetchRequest); err != nil {
			level.Error(e.logger).Log("msg", "Error fetching offset for consumergroup", "group", group.GroupId, "err", err.Error())
		} else {
			for topic, partitions := range offsetFetchResponse.Blocks {
				if !e.topicFilter.MatchString(topic) || e.topicExclude.MatchString(topic) {
					continue
				}
				// If the topic is not consumed by that consumer group, skip it
				topicConsumed := false
				for _, offsetFetchResponseBlock := range partitions {
					// Kafka will return -1 if there is no offset associated with a topic-partition under that consumer group
					if offsetFetchResponseBlock.Offset != -1 {
						topicConsumed = true
						break
					}
				}
				if topicConsumed {
					var currentOffsetSum int64
					var lagSum int64
					for partition, offsetFetchResponseBlock := range partitions {
						kerr := offsetFetchResponseBlock.Err
						if kerr != sarama.ErrNoError {
							level.Error(e.logger).Log("msg", "Error in response block for topic/partition", "topic", topic, "partition", partition, "err", kerr.Error())
							continue
						}
						currentOffset := offsetFetchResponseBlock.Offset
						currentOffsetSum += currentOffset

						ch <- prometheus.MustNewConstMetric(
							e.promDesc.consumergroupCurrentOffset, prometheus.GaugeValue, float64(currentOffset), group.GroupId, topic, strconv.FormatInt(int64(partition), 10),
						)
						e.mu.Lock()
						// Get and insert the next offset to be produced into the interpolation map
						nextOffset, err := e.client.GetOffset(topic, partition, sarama.OffsetNewest)
						if err != nil {
							level.Error(e.logger).Log("msg", "Error getting next offset for topic/partition", "topic", topic, "partition", partition, "err", err.Error())
						}
						if !e.disableCalculateLagRate {
							e.consumerGroupLagTable.createOrUpdate(group.GroupId, topic, partition, nextOffset)
						}

						// If the topic is consumed by that consumer group, but no offset associated with the partition
						// forcing lag to -1 to be able to alert on that
						var lag int64
						if currentOffset == -1 {
							lag = -1
						} else {
							lag = nextOffset - currentOffset
							lagSum += lag
						}
						e.mu.Unlock()
						ch <- prometheus.MustNewConstMetric(
							e.promDesc.consumergroupUncomittedOffsets, prometheus.GaugeValue, float64(lag), group.GroupId, topic, strconv.FormatInt(int64(partition), 10),
						)
					}
					ch <- prometheus.MustNewConstMetric(
						e.promDesc.consumergroupCurrentOffsetSum, prometheus.GaugeValue, float64(currentOffsetSum), group.GroupId, topic,
					)
					ch <- prometheus.MustNewConstMetric(
						e.promDesc.consumergroupUncommittedOffsetsSum, prometheus.GaugeValue, float64(lagSum), group.GroupId, topic,
					)
				}
			}
		}
	}
}

func (e *Exporter) metricsForLag(ch chan<- prometheus.Metric) {

	admin, err := sarama.NewClusterAdminFromClient(e.client)
	if err != nil {
		level.Error(e.logger).Log("msg", "Error creating cluster admin", "err", err.Error())
		return
	}
	if admin == nil {
		level.Error(e.logger).Log("msg", "Failed to create cluster admin")
		return
	}

	// Iterate over all consumergroup/topic/partitions
	e.consumerGroupLagTable.mu.Lock()
	for group, topics := range e.consumerGroupLagTable.iMap {
		for topic, partitionMap := range topics {
			var partitionKeys []int32
			// Collect partitions to create ListConsumerGroupOffsets request
			for key := range partitionMap {
				partitionKeys = append(partitionKeys, key)
			}

			// response.Blocks is a map of topic to partition to offset
			response, err := admin.ListConsumerGroupOffsets(group, map[string][]int32{
				topic: partitionKeys,
			})
			if err != nil {
				level.Error(e.logger).Log("msg", "Error listing offsets for", "group", group, "err", err.Error())
			}
			if response == nil {
				level.Error(e.logger).Log("msg", "Got nil response from ListConsumerGroupOffsets for group", "group", group)
				continue
			}

			for partition, offsets := range partitionMap {
				if len(offsets) < 2 {
					level.Debug(e.logger).Log("msg", "Insufficient data for lag calculation for group: continuing", "group", group)
					continue
				}
				if latestConsumedOffset, ok := response.Blocks[topic][partition]; ok {
					/*
						Sort offset keys so we know if we have an offset to use as a left bound in our calculation
						If latestConsumedOffset < smallestMappedOffset then extrapolate
						Else Find two offsets that bound latestConsumedOffset
					*/
					var producedOffsets []int64
					for offsetKey := range offsets {
						producedOffsets = append(producedOffsets, offsetKey)
					}
					sort.Slice(producedOffsets, func(i, j int) bool { return producedOffsets[i] < producedOffsets[j] })
					if latestConsumedOffset.Offset < producedOffsets[0] {
						level.Debug(e.logger).Log("msg", "estimating lag for group/topic/partition", "group", group, "topic", topic, "partition", partition, "method", "extrapolation")
						// Because we do not have data points that bound the latestConsumedOffset we must use extrapolation
						highestOffset := producedOffsets[len(producedOffsets)-1]
						lowestOffset := producedOffsets[0]

						px := float64(offsets[highestOffset].UnixNano()/1000000) -
							float64(highestOffset-latestConsumedOffset.Offset)*
								float64((offsets[highestOffset].Sub(offsets[lowestOffset])).Milliseconds())/float64(highestOffset-lowestOffset)
						lagMillis := float64(time.Now().UnixNano()/1000000) - px
						level.Debug(e.logger).Log("msg", "estimated lag for group/topic/partition (in ms)", "group", group, "topic", topic, "partition", partition, "lag", lagMillis)

						ch <- prometheus.MustNewConstMetric(e.promDesc.lagDatapointUsedExtrapolation, prometheus.CounterValue, 1, group, topic, strconv.FormatInt(int64(partition), 10))
						ch <- prometheus.MustNewConstMetric(e.promDesc.topicPartitionLagMillis, prometheus.GaugeValue, lagMillis, group, topic, strconv.FormatInt(int64(partition), 10))

					} else {
						level.Debug(e.logger).Log("msg", "estimating lag for group/topic/partition", "group", group, "topic", topic, "partition", partition, "method", "interpolation")
						nextHigherOffset := getNextHigherOffset(producedOffsets, latestConsumedOffset.Offset)
						nextLowerOffset := getNextLowerOffset(producedOffsets, latestConsumedOffset.Offset)
						px := float64(offsets[nextHigherOffset].UnixNano()/1000000) -
							float64(nextHigherOffset-latestConsumedOffset.Offset)*
								float64((offsets[nextHigherOffset].Sub(offsets[nextLowerOffset])).Milliseconds())/float64(nextHigherOffset-nextLowerOffset)
						lagMillis := float64(time.Now().UnixNano()/1000000) - px
						level.Debug(e.logger).Log("msg", "estimated lag for group/topic/partition (in ms)", "group", group, "topic", topic, "partition", partition, "lag", lagMillis)
						ch <- prometheus.MustNewConstMetric(e.promDesc.lagDatapointUsedInterpolation, prometheus.CounterValue, 1, group, topic, strconv.FormatInt(int64(partition), 10))
						ch <- prometheus.MustNewConstMetric(e.promDesc.topicPartitionLagMillis, prometheus.GaugeValue, lagMillis, group, topic, strconv.FormatInt(int64(partition), 10))
					}
				} else {
					level.Error(e.logger).Log("msg", "Could not get latest latest consumed offset", "group", group, "topic", topic, "partition", partition)
				}
			}
		}
	}
	e.consumerGroupLagTable.mu.Unlock()
}

func getNextHigherOffset(offsets []int64, k int64) int64 {
	index := len(offsets) - 1
	max := offsets[index]

	for max >= k && index > 0 {
		if offsets[index-1] < k {
			return max
		}
		max = offsets[index]
		index--
	}
	return max
}

func getNextLowerOffset(offsets []int64, k int64) int64 {
	index := 0
	min := offsets[index]
	for min <= k && index < len(offsets)-1 {
		if offsets[index+1] > k {
			return min
		}
		min = offsets[index]
		index++
	}
	return min
}

// Run iMap.Prune() on an interval (default 30 seconds). A new client is created
// to avoid an issue where the client may be closed before Prune attempts to
// use it.
func (e *Exporter) RunPruner() {
	ticker := time.NewTicker(time.Duration(e.kafkaOpts.PruneIntervalSeconds) * time.Second)

	for {
		select {
		case <-ticker.C:
			client, err := sarama.NewClient(e.kafkaOpts.Uri, e.saramaConfig)
			if err != nil {
				level.Error(e.logger).Log("msg", "Error initializing kafka client for RunPruner", "err", err.Error())
				return
			}
			e.consumerGroupLagTable.Prune(e.logger, client, e.kafkaOpts.MaxOffsets)

			client.Close()
		case <-e.quitPruneCh:
			ticker.Stop()
			return
		}
	}
}

func (e *Exporter) Close() {
	close(e.quitPruneCh)
	e.client.Close()
}

func (e *Exporter) initializeMetrics() {
	labels := make(map[string]string)

	// Protect against empty labels
	if e.kafkaOpts.Labels != "" {
		for _, label := range strings.Split(e.kafkaOpts.Labels, ",") {
			splitLabels := strings.Split(label, "=")
			if len(splitLabels) >= 2 {
				labels[splitLabels[0]] = splitLabels[1]
			}
		}
	}

	up := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether Kafka is up.",
		nil, labels,
	)

	clusterBrokers := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "brokers"),
		"Number of Brokers in the Kafka Cluster.",
		nil, labels,
	)
	clusterBrokerInfo := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "broker_info"),
		"Information about the Kafka Broker.",
		[]string{"id", "address"}, labels,
	)
	topicPartitions := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partitions"),
		"Number of partitions for this Topic",
		[]string{"topic"}, labels,
	)
	topicCurrentOffset := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_current_offset"),
		"Current Offset of a Broker at Topic/Partition",
		[]string{"topic", "partition"}, labels,
	)
	topicOldestOffset := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_oldest_offset"),
		"Oldest Offset of a Broker at Topic/Partition",
		[]string{"topic", "partition"}, labels,
	)

	topicPartitionLeader := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_leader"),
		"Leader Broker ID of this Topic/Partition",
		[]string{"topic", "partition"}, labels,
	)

	topicPartitionReplicas := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_replicas"),
		"Number of Replicas for this Topic/Partition",
		[]string{"topic", "partition"}, labels,
	)

	topicPartitionInSyncReplicas := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_in_sync_replica"),
		"Number of In-Sync Replicas for this Topic/Partition",
		[]string{"topic", "partition"}, labels,
	)

	topicPartitionUsesPreferredReplica := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_leader_is_preferred"),
		"1 if Topic/Partition is using the Preferred Broker",
		[]string{"topic", "partition"}, labels,
	)

	topicUnderReplicatedPartition := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "topic", "partition_under_replicated_partition"),
		"1 if Topic/Partition is under Replicated",
		[]string{"topic", "partition"}, labels,
	)

	consumergroupCurrentOffset := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "consumergroup", "current_offset"),
		"Current Offset of a ConsumerGroup at Topic/Partition",
		[]string{"consumergroup", "topic", "partition"}, labels,
	)

	consumergroupCurrentOffsetSum := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "consumergroup", "current_offset_sum"),
		"Current Offset of a ConsumerGroup at Topic for all partitions",
		[]string{"consumergroup", "topic"}, labels,
	)

	var consumergroupUncomittedOffsets, consumergroupUncommittedOffsetsZookeeper, consumergroupUncommittedOffsetsSum *prometheus.Desc
	if e.renameUncommitOffsetsToLag {
		consumergroupUncomittedOffsets = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroup", "lag"),
			"Current Approximate Lag of a ConsumerGroup at Topic/Partition",
			[]string{"consumergroup", "topic", "partition"}, labels,
		)
		consumergroupUncommittedOffsetsZookeeper = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroupzookeeper", "lag_zookeeper"),
			"Current Approximate Lag(zookeeper) of a ConsumerGroup at Topic/Partition",
			[]string{"consumergroup", "topic", "partition"}, nil,
		)

		consumergroupUncommittedOffsetsSum = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroup", "lag_sum"),
			"Current Approximate Lag of a ConsumerGroup at Topic for all partitions",
			[]string{"consumergroup", "topic"}, labels,
		)

	} else {
		consumergroupUncomittedOffsets = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroup", "uncommitted_offsets"),
			"Current Approximate count of uncommitted offsets for a ConsumerGroup at Topic/Partition",
			[]string{"consumergroup", "topic", "partition"}, labels,
		)

		consumergroupUncommittedOffsetsZookeeper = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroupzookeeper", "uncommitted_offsets_zookeeper"),
			"Current Approximate count of uncommitted offsets(zookeeper) for a ConsumerGroup at Topic/Partition",
			[]string{"consumergroup", "topic", "partition"}, nil,
		)

		consumergroupUncommittedOffsetsSum = prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "consumergroup", "uncommitted_offsets_sum"),
			"Current Approximate count of uncommitted offsets for a ConsumerGroup at Topic for all partitions",
			[]string{"consumergroup", "topic"}, labels,
		)
	}

	consumergroupMembers := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "consumergroup", "members"),
		"Amount of members in a consumer group",
		[]string{"consumergroup"}, labels,
	)

	topicPartitionLagMillis := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "consumer_lag", "millis"),
		"Current approximation of consumer lag for a ConsumerGroup at Topic/Partition",
		[]string{"consumergroup", "topic", "partition"},
		labels,
	)

	lagDatapointUsedInterpolation := prometheus.NewDesc(prometheus.BuildFQName(namespace, "consumer_lag", "interpolation"),
		"Indicates that a consumer group lag estimation used interpolation",
		[]string{"consumergroup", "topic", "partition"},
		labels,
	)

	lagDatapointUsedExtrapolation := prometheus.NewDesc(prometheus.BuildFQName(namespace, "consumer_lag", "extrapolation"),
		"Indicates that a consumer group lag estimation used extrapolation",
		[]string{"consumergroup", "topic", "partition"},
		labels,
	)

	e.promDesc = &PromDesc{
		up:                                       up,
		clusterBrokers:                           clusterBrokers,
		clusterBrokerInfo:                        clusterBrokerInfo,
		topicPartitions:                          topicPartitions,
		topicCurrentOffset:                       topicCurrentOffset,
		topicOldestOffset:                        topicOldestOffset,
		topicPartitionLeader:                     topicPartitionLeader,
		topicPartitionReplicas:                   topicPartitionReplicas,
		topicPartitionInSyncReplicas:             topicPartitionInSyncReplicas,
		topicPartitionUsesPreferredReplica:       topicPartitionUsesPreferredReplica,
		topicUnderReplicatedPartition:            topicUnderReplicatedPartition,
		consumergroupCurrentOffset:               consumergroupCurrentOffset,
		consumergroupCurrentOffsetSum:            consumergroupCurrentOffsetSum,
		consumergroupUncomittedOffsets:           consumergroupUncomittedOffsets,
		consumergroupUncommittedOffsetsSum:       consumergroupUncommittedOffsetsSum,
		consumergroupUncommittedOffsetsZookeeper: consumergroupUncommittedOffsetsZookeeper,
		consumergroupMembers:                     consumergroupMembers,
		topicPartitionLagMillis:                  topicPartitionLagMillis,
		lagDatapointUsedInterpolation:            lagDatapointUsedInterpolation,
		lagDatapointUsedExtrapolation:            lagDatapointUsedExtrapolation,
	}
}
