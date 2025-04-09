package kafka

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/inputs/kafka/exporter"
	"flashcat.cloud/categraf/types"
	"github.com/IBM/sarama"
	"github.com/go-kit/log/level"

	klog "github.com/go-kit/log"
)

const inputName = "kafka"

type Kafka struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Kafka{}
	})
}
func (r *Kafka) Clone() inputs.Input {
	return &Kafka{}
}

func (r *Kafka) Name() string {
	return inputName
}

func (r *Kafka) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

func (r *Kafka) Drop() {
	for _, i := range r.Instances {
		if i == nil {
			continue
		}

		if i.e != nil {
			i.e.Close()
		}
	}
}

type Instance struct {
	config.InstanceConfig

	LogLevel string `toml:"log_level"`

	// Address (host:port) of Kafka server.
	KafkaURIs []string `toml:"kafka_uris,omitempty"`

	// Connect using SASL/PLAIN
	UseSASL bool `toml:"use_sasl,omitempty"`

	// Only set this to false if using a non-Kafka SASL proxy
	UseSASLHandshake *bool `toml:"use_sasl_handshake,omitempty"`

	// SASL user name
	SASLUsername string `toml:"sasl_username,omitempty"`

	// SASL user password
	SASLPassword string `toml:"sasl_password,omitempty"`

	// The SASL SCRAM SHA algorithm sha256 or sha512 as mechanism
	SASLMechanism string `toml:"sasl_mechanism,omitempty"`

	// Connect using TLS
	UseTLS bool `toml:"use_tls,omitempty"`

	// The optional certificate authority file for TLS client authentication
	CAFile string `toml:"ca_file,omitempty"`

	// The optional certificate file for TLS client authentication
	CertFile string `toml:"cert_file,omitempty"`

	// The optional key file for TLS client authentication
	KeyFile string `toml:"key_file,omitempty"`

	// If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
	InsecureSkipVerify bool `toml:"insecure_skip_verify,omitempty"`

	// Kafka broker version
	KafkaVersion string `toml:"kafka_version,omitempty"`

	// if you need to use a group from zookeeper
	UseZooKeeperLag bool `toml:"use_zookeeper_lag,omitempty"`

	// Address array (hosts) of zookeeper server.
	ZookeeperURIs []string `toml:"zookeeper_uris,omitempty"`

	// Metadata refresh interval
	MetadataRefreshInterval string `toml:"metadata_refresh_interval,omitempty"`

	// Whether show the offset/lag for all consumer group, otherwise, only show connected consumer groups, default is true
	OffsetShowAll *bool `toml:"offset_show_all,omitempty"`

	// If true, all scrapes will trigger kafka operations otherwise, they will share results. WARN: This should be disabled on large clusters
	AllowConcurrent *bool `toml:"allow_concurrency,omitempty"`

	// Maximum number of offsets to store in the interpolation table for a partition
	MaxOffsets int `toml:"max_offsets,omitempty"`

	// How frequently should the interpolation table be pruned, in seconds
	PruneIntervalSeconds int `toml:"prune_interval_seconds,omitempty"`

	// Regex filter for topics to be monitored
	TopicsFilter string `toml:"topics_filter_regex,omitempty"`

	TopicExclude string `toml:"topic_exclude_regex,omitempty"`

	// Regex filter for consumer groups to be monitored
	GroupFilter string `toml:"groups_filter_regex,omitempty"`

	GroupExclude string `toml:"group_exclude_regex,omitempty"`

	// rename metric: kafka_consumergroup_uncommitted_offsets to kafka_consumergroup_lag
	RenameUncommitOffsetsToLag bool `toml:"rename_uncommit_offset_to_lag,omitempty"`
	// disable calculate lag rate
	DisableCalculateLagRate bool `toml:"disable_calculate_lag_rate,omitempty"`

	l klog.Logger        `toml:"-"`
	e *exporter.Exporter `toml:"-"`

	DialTimeout  int `toml:"dial_timeout"`
	ReadTimeout  int `toml:"read_timeout"`
	WriteTimeout int `toml:"write_timeout"`
}

func (ins *Instance) Init() error {
	if len(ins.KafkaURIs) == 0 || ins.KafkaURIs[0] == "" {
		return types.ErrInstancesEmpty
	}
	if ins.UseSASL && (ins.SASLPassword == "" || ins.SASLUsername == "") {
		return fmt.Errorf("SASL is enabled but username or password was not provided")
	}
	if ins.UseZooKeeperLag && (len(ins.ZookeeperURIs) == 0 || ins.ZookeeperURIs[0] == "") {
		return fmt.Errorf("zookeeper lag is enabled but no zookeeper uri was provided")
	}

	// default options
	if ins.UseSASLHandshake == nil {
		flag := true
		ins.UseSASLHandshake = &flag
	}
	if len(ins.KafkaVersion) == 0 {
		ins.KafkaVersion = sarama.V2_0_0_0.String()
	}
	if len(ins.MetadataRefreshInterval) == 0 {
		ins.MetadataRefreshInterval = "1m"
	}
	if ins.AllowConcurrent == nil {
		flag := false
		ins.AllowConcurrent = &flag
	}
	if ins.OffsetShowAll == nil {
		flag := true
		ins.OffsetShowAll = &flag
	}
	if ins.MaxOffsets <= 0 {
		ins.MaxOffsets = 1000
	}
	if ins.PruneIntervalSeconds <= 0 {
		ins.PruneIntervalSeconds = 30
	}
	if len(ins.TopicsFilter) == 0 {
		ins.TopicsFilter = ".*"
	}
	if len(ins.TopicExclude) == 0 {
		ins.TopicExclude = "^$"
	}
	if len(ins.GroupFilter) == 0 {
		ins.GroupFilter = ".*"
	}
	if len(ins.GroupExclude) == 0 {
		ins.GroupExclude = "^$"
	}
	if ins.DialTimeout == 0 {
		ins.DialTimeout = 30
	}
	if ins.ReadTimeout == 0 {
		ins.ReadTimeout = 30
	}
	if ins.WriteTimeout == 0 {
		ins.WriteTimeout = 30
	}

	options := exporter.Options{
		Uri:                        ins.KafkaURIs,
		UseSASL:                    ins.UseSASL,
		UseSASLHandshake:           *ins.UseSASLHandshake,
		SaslUsername:               ins.SASLUsername,
		SaslPassword:               string(ins.SASLPassword),
		SaslMechanism:              ins.SASLMechanism,
		UseTLS:                     ins.UseTLS,
		TlsCAFile:                  ins.CAFile,
		TlsCertFile:                ins.CertFile,
		TlsKeyFile:                 ins.KeyFile,
		TlsInsecureSkipTLSVerify:   ins.InsecureSkipVerify,
		KafkaVersion:               ins.KafkaVersion,
		UseZooKeeperLag:            ins.UseZooKeeperLag,
		UriZookeeper:               ins.ZookeeperURIs,
		MetadataRefreshInterval:    ins.MetadataRefreshInterval,
		OffsetShowAll:              *ins.OffsetShowAll,
		AllowConcurrent:            *ins.AllowConcurrent,
		MaxOffsets:                 ins.MaxOffsets,
		PruneIntervalSeconds:       ins.PruneIntervalSeconds,
		DisableCalculateLagRate:    ins.DisableCalculateLagRate,
		RenameUncommitOffsetsToLag: ins.RenameUncommitOffsetsToLag,
		DialTimeout:                time.Duration(ins.DialTimeout) * time.Second,
		ReadTimeout:                time.Duration(ins.ReadTimeout) * time.Second,
		WriteTimeout:               time.Duration(ins.WriteTimeout) * time.Second,
	}

	ins.l = level.NewFilter(klog.NewLogfmtLogger(klog.NewSyncWriter(os.Stderr)), levelFilter(ins.LogLevel))

	e, err := exporter.New(ins.l, options, ins.TopicsFilter, ins.TopicExclude, ins.GroupFilter, ins.GroupExclude)
	if err != nil {
		return fmt.Errorf("could not instantiate kafka lag exporter: %w", err)
	}

	ins.e = e
	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	defer func(begun time.Time) {
		slist.PushSample(inputName, "scrape_use_seconds", time.Since(begun).Seconds())
	}(time.Now())

	err := inputs.Collect(ins.e, slist)
	if err != nil {
		log.Println("E! failed to collect metrics:", err)
	}
}

func levelFilter(l string) level.Option {
	l = strings.ToLower(l)
	switch l {
	case "debug":
		return level.AllowDebug()
	case "info":
		return level.AllowInfo()
	case "warn":
		return level.AllowWarn()
	case "error":
		return level.AllowError()
	default:
		return level.AllowAll()
	}
}
