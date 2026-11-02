//go:generate ../../../tools/readme_config_includer/generator
package gnmi

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	xpath "flashcat.cloud/categraf/internal/gnmipath"
	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc/metadata"

	cryptotls "crypto/tls"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

// Define the warning to show if we cannot get a metric name.
const (
	emptyNameWarning = `Got empty metric-name for response, usually indicating
configuration issues as the response cannot be related to any subscription.
Please open an issue on https://github.com/influxdata/telegraf including your
device model and the following response data:
%+v
This message is only printed once.`
	inputName = `gnmi`
)

// Currently supported GNMI Extensions
var supportedExtensions = []string{"juniper_header"}

// gNMI plugin instance
type (
	GNMI struct {
		config.PluginConfig
		Instances []*Instance
	}

	Instance struct {
		config.InstanceConfig

		Addresses           []string          `toml:"addresses"`
		Subscriptions       []Subscription    `toml:"subscription"`
		TagSubscriptions    []TagSubscription `toml:"tag_subscription"`
		Aliases             map[string]string `toml:"aliases"`
		Encoding            string            `toml:"encoding"`
		Origin              string            `toml:"origin"`
		Prefix              string            `toml:"prefix"`
		Target              string            `toml:"target"`
		UpdatesOnly         bool              `toml:"updates_only"`
		VendorSpecific      []string          `toml:"vendor_specific"`
		Username            string            `toml:"username"`
		Password            string            `toml:"password"`
		Redial              config.Duration   `toml:"redial"`
		MaxMsgSize          int64             `toml:"max_msg_size"`
		Trace               bool              `toml:"dump_responses"`
		CanonicalFieldNames bool              `toml:"canonical_field_names"`
		TrimFieldNames      bool              `toml:"trim_field_names"`
		GuessPathTag        bool              `toml:"guess_path_tag"`
		EnableTLS           bool              `toml:"enable_tls" deprecated:"1.27.0;use 'tls_enable' instead"`
		tls.ClientConfig

		// Internal state
		internalAliases map[*pathInfo]string
		// tagStores are kept on the instance (not the per-connection handler)
		// so tags learned on one connection survive reconnects. They are keyed
		// by device address to avoid leaking tags between targets.
		tagStores   map[string]*tagStore
		storeInitMu sync.Mutex
		request     *gnmiLib.SubscribeRequest
		tlsConfig   *cryptotls.Config
		cancel      context.CancelFunc
		wg          sync.WaitGroup
		slist       *types.SampleList

		AgentHostTag string `toml:"agent_host_tag"`
	}
)

// Subscription for a gNMI client
type Subscription struct {
	Name   string
	Origin string
	Path   string

	fullPath *gnmiLib.Path
	// Keep source metadata before fullPath normalization strips the module.
	sourcePath *pathInfo

	// Subscription mode and interval
	SubscriptionMode string          `toml:"subscription_mode"`
	SampleInterval   config.Duration `toml:"sample_interval"`

	// Duplicate suppression
	SuppressRedundant bool            `toml:"suppress_redundant"`
	HeartbeatInterval config.Duration `toml:"heartbeat_interval"`

	// Mark this subscription as a tag-only lookup source, not emitting any metric
	TagOnly bool `toml:"tag_only" deprecated:"1.25.0;2.0.0;please use 'tag_subscription's instead"`

	DisableConcatenation bool   `toml:"disable_concatenation"`
	Conversion           string `toml:"conversion"`
}

// Tag Subscription for a gNMI client
type TagSubscription struct {
	Subscription
	Match         string `toml:"match"`
	Elements      []string
	DropSourceKey bool `toml:"drop_source_key"`
}

func (c *Instance) Init() error {
	if len(c.Addresses) == 0 {
		return types.ErrInstancesEmpty
	}
	if len(c.Encoding) == 0 {
		c.Encoding = "proto"
	}
	if len(c.AgentHostTag) == 0 {
		c.AgentHostTag = "agent_host"
	}
	c.slist = types.NewSampleList()
	c.tagStores = make(map[string]*tagStore)
	// Check options
	if time.Duration(c.Redial) <= 0 {
		c.Redial = config.Duration(10 * time.Second)
		log.Println("W! redial duration must be positive")
	}

	// Check vendor_specific options configured by user
	if err := choice.CheckSlice(c.VendorSpecific, supportedExtensions); err != nil {
		return fmt.Errorf("unsupported vendor_specific option: %w", err)
	}

	// Use the new TLS option for enabling
	// Honor deprecated option
	enable := c.ClientConfig.UseTLS || c.EnableTLS
	c.ClientConfig.UseTLS = enable

	// Split the subscriptions into "normal" and "tag" subscription
	// and prepare them.
	for i := len(c.Subscriptions) - 1; i >= 0; i-- {
		subscription := c.Subscriptions[i]

		// Check the subscription
		if subscription.Name == "" {
			return fmt.Errorf("empty 'name' found for subscription %d", i+1)
		}
		if subscription.Path == "" {
			return fmt.Errorf("empty 'path' found for subscription %d", i+1)
		}

		// Support and convert legacy TagOnly subscriptions
		if subscription.TagOnly {
			tagSub := TagSubscription{
				Subscription: subscription,
				Match:        "name",
			}
			c.TagSubscriptions = append(c.TagSubscriptions, tagSub)
			// Remove from the original subscriptions list
			c.Subscriptions = append(c.Subscriptions[:i], c.Subscriptions[i+1:]...)
			continue
		}
		if err := subscription.buildFullPath(c); err != nil {
			return err
		}
	}
	for idx := range c.TagSubscriptions {
		if err := c.TagSubscriptions[idx].buildFullPath(c); err != nil {
			return err
		}
		if c.TagSubscriptions[idx].TagOnly != c.TagSubscriptions[0].TagOnly {
			return fmt.Errorf("do not mix legacy tag_only subscriptions and tag subscriptions")
		}
		switch c.TagSubscriptions[idx].Match {
		case "":
			if len(c.TagSubscriptions[idx].Elements) > 0 {
				c.TagSubscriptions[idx].Match = "elements"
			} else {
				c.TagSubscriptions[idx].Match = "name"
			}
		case "unconditional":
		case "name":
		case "elements":
			if len(c.TagSubscriptions[idx].Elements) == 0 {
				return fmt.Errorf("tag_subscription must have at least one element")
			}
		default:
			return fmt.Errorf("unknown match type %q for tag-subscription %q", c.TagSubscriptions[idx].Match, c.TagSubscriptions[idx].Name)
		}
	}

	if err := validateTagSubscriptions(c.TagSubscriptions); err != nil {
		return err
	}

	// Invert explicit alias list and prefill subscription names
	c.internalAliases = make(map[*pathInfo]string, len(c.Subscriptions)+len(c.Aliases)+len(c.TagSubscriptions))
	for _, s := range c.Subscriptions {
		if err := s.buildAlias(c.internalAliases); err != nil {
			return err
		}
	}
	for _, s := range c.TagSubscriptions {
		if err := s.buildAlias(c.internalAliases); err != nil {
			return err
		}
	}
	for alias, encodingPath := range c.Aliases {
		c.internalAliases[newInfoFromString(encodingPath)] = alias
	}
	log.Printf("D! Internal alias mapping: %+v", c.internalAliases)

	// Prepare the subscribe request and TLS configuration synchronously so
	// configuration errors (invalid path/encoding/subscription mode/TLS) are
	// reported at startup instead of being silently dropped by a goroutine.
	request, err := c.newSubscribeRequest()
	if err != nil {
		return err
	}
	c.request = request

	tlsCfg, err := c.TLSConfig()
	if err != nil {
		return err
	}
	c.tlsConfig = tlsCfg

	go c.run()
	return nil
}

// run dials every configured device and keeps the gNMI subscriptions alive,
// reconnecting until the instance is dropped. All configuration that can fail
// is validated up-front in Init(), so here errors during the connection
// lifecycle are only logged.
func (c *Instance) run() {
	// Prepare the context, optionally with credentials
	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())
	if len(c.Username) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, "username", c.Username, "password", c.Password)
	}

	// Create a goroutine for each device, dial and subscribe
	c.wg.Add(len(c.Addresses))
	for _, addr := range c.Addresses {
		go func(addr string) {
			defer c.wg.Done()

			// Reuse one store per device address across reconnects so tags
			// already learned are not wiped whenever the stream drops.
			store := c.getTagStore(addr)

			h := handler{
				address:             addr,
				aliases:             c.internalAliases,
				tagsubs:             c.TagSubscriptions,
				subs:                c.Subscriptions,
				maxMsgSize:          int(c.MaxMsgSize),
				vendorExt:           c.VendorSpecific,
				tagStore:            store,
				trace:               c.Trace,
				canonicalFieldNames: c.CanonicalFieldNames,
				trimSlash:           c.TrimFieldNames,
				guessPathTag:        c.GuessPathTag,
				sourceTag:           c.AgentHostTag,
			}
			for ctx.Err() == nil {
				if err := h.subscribeGNMI(ctx, c.slist, c.tlsConfig, c.request); err != nil && ctx.Err() == nil {
					log.Println("W! Error in gNMI subscription:", err)
				}

				select {
				case <-ctx.Done():
				case <-time.After(time.Duration(c.Redial)):
				}
			}
		}(addr)
	}
}

// getTagStore returns the per-device tag store, creating it on first use.
// Stores live on the Instance and are shared across the reconnect loop, so
// learned labels survive stream drops, while the address key prevents
// different targets from leaking labels into each other.
func (c *Instance) getTagStore(address string) *tagStore {
	c.storeInitMu.Lock()
	defer c.storeInitMu.Unlock()
	if store, ok := c.tagStores[address]; ok {
		return store
	}
	store := newTagStore(c.TagSubscriptions)
	store.address = address
	c.tagStores[address] = store
	return store
}

func (s *Subscription) buildSubscription() (*gnmiLib.Subscription, error) {
	gnmiPath, err := parsePath(s.Origin, s.Path, "")
	if err != nil {
		return nil, err
	}
	// Default to SAMPLE when the mode is not configured, so a missing
	// subscription_mode (whether omitted or "") does not silently fail and
	// does not fall through to the protobuf zero value TARGET_DEFINED.
	modeName := s.SubscriptionMode
	if modeName == "" {
		modeName = "SAMPLE"
	}
	modeValue, ok := gnmiLib.SubscriptionMode_value[strings.ToUpper(modeName)]
	if !ok {
		return nil, fmt.Errorf("invalid subscription mode %q for subscription %q", s.SubscriptionMode, s.Name)
	}
	mode := gnmiLib.SubscriptionMode(modeValue)

	// A zero/missing sample_interval is passed through to the device as-is.
	// Many targets accept it and apply their own default sampling cadence, so
	// we do not reject it (that broke valid configs). The interval is only
	// meaningful in SAMPLE mode; ON_CHANGE/TARGET_DEFINED ignore it.
	sampleInterval := time.Duration(s.SampleInterval)

	// Redundancy suppression only makes sense together with a heartbeat
	// interval, otherwise an unchanged value may never be reported again.
	if s.SuppressRedundant && time.Duration(s.HeartbeatInterval) <= 0 {
		log.Printf("W! subscription %q has suppress_redundant=true without a positive heartbeat_interval; unchanged values may never be reported", s.Name)
	}

	return &gnmiLib.Subscription{
		Path:              gnmiPath,
		Mode:              mode,
		HeartbeatInterval: uint64(time.Duration(s.HeartbeatInterval).Nanoseconds()),
		SampleInterval:    uint64(sampleInterval.Nanoseconds()),
		SuppressRedundant: s.SuppressRedundant,
	}, nil
}

// Create a new gNMI SubscribeRequest
func (c *Instance) newSubscribeRequest() (*gnmiLib.SubscribeRequest, error) {
	// Create subscription objects
	subscriptions := make([]*gnmiLib.Subscription, 0, len(c.Subscriptions)+len(c.TagSubscriptions))
	for _, subscription := range c.TagSubscriptions {
		sub, err := subscription.buildSubscription()
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, sub)
	}
	for _, subscription := range c.Subscriptions {
		sub, err := subscription.buildSubscription()
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, sub)
	}

	// Construct subscribe request
	gnmiPath, err := parsePath(c.Origin, c.Prefix, c.Target)
	if err != nil {
		return nil, err
	}

	// Do not provide an empty prefix. Required for Huawei NE40 router v8.21
	// (and possibly others). See https://github.com/influxdata/telegraf/issues/12273.
	if gnmiPath.Origin == "" && gnmiPath.Target == "" && len(gnmiPath.Elem) == 0 {
		gnmiPath = nil
	}

	if c.Encoding != "proto" && c.Encoding != "json" && c.Encoding != "json_ietf" && c.Encoding != "bytes" {
		return nil, fmt.Errorf("unsupported encoding %s", c.Encoding)
	}

	return &gnmiLib.SubscribeRequest{
		Request: &gnmiLib.SubscribeRequest_Subscribe{
			Subscribe: &gnmiLib.SubscriptionList{
				Prefix:       gnmiPath,
				Mode:         gnmiLib.SubscriptionList_STREAM,
				Encoding:     gnmiLib.Encoding(gnmiLib.Encoding_value[strings.ToUpper(c.Encoding)]),
				Subscription: subscriptions,
				UpdatesOnly:  c.UpdatesOnly,
			},
		},
	}, nil
}

// ParsePath from XPath-like string to gNMI path structure
func parsePath(origin string, pathToParse string, target string) (*gnmiLib.Path, error) {
	gnmiPath, err := xpath.ToGNMIPath(pathToParse)
	if err != nil {
		return nil, err
	}
	gnmiPath.Origin = origin
	gnmiPath.Target = target
	return gnmiPath, err
}

// Stop listener and cleanup
func (c *Instance) Drop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// Gather plugin measurements (unused)
func (c *Instance) Gather(slist *types.SampleList) {
	slist.PushFrontN(c.slist.PopBackAll())
}

func (c *GNMI) Name() string {
	return inputName
}

func (c *GNMI) Clone() inputs.Input {
	return &GNMI{}
}

func (c *GNMI) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(c.Instances))
	for i := 0; i < len(c.Instances); i++ {
		ret[i] = c.Instances[i]
	}
	return ret
}

func (c *GNMI) Drop() {
	for _, instance := range c.Instances {
		instance.Drop()
	}
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(GNMI)
var _ inputs.InstancesGetter = new(GNMI)

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &GNMI{}
	})
}

func (s *Subscription) buildFullPath(c *Instance) error {
	var err error
	if s.fullPath, err = xpath.ToGNMIPath(s.Path); err != nil {
		return err
	}
	s.fullPath.Origin = s.Origin
	if s.Origin == "" {
		s.fullPath.Origin = c.Origin
	}
	s.fullPath.Target = c.Target
	if c.Prefix != "" {
		prefix, err := xpath.ToGNMIPath(c.Prefix)
		if err != nil {
			return err
		}
		s.fullPath.Elem = append(prefix.Elem, s.fullPath.Elem...)
	}
	// Extract an inline "module:" prefix (e.g. /rg-interfaces:...) so tag
	// subscriptions match the normalized paths built from device updates.
	s.sourcePath = newInfoFromPath(s.fullPath)
	normalizeGNMIPath(s.fullPath)
	return nil
}

func (s *Subscription) buildAlias(aliases map[*pathInfo]string) error {
	// Build the subscription path without keys
	path, err := parsePath(s.Origin, s.Path, "")
	if err != nil {
		return err
	}
	info := newInfoFromPathWithoutKeys(path)

	// If the user didn't provide a measurement name, use last path element
	name := s.Name
	if name == "" && len(info.segments) > 0 {
		name = info.segments[len(info.segments)-1]
	}
	if name != "" {
		aliases[info] = name
	}
	return nil
}
