package redis_sentinel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/go-redis/redis/v8"
)

const inputName = "redis_sentinel"
const measurementMasters = "redis_sentinel_masters"
const measurementSentinel = "redis_sentinel"
const measurementSentinels = "redis_sentinel_sentinels"
const measurementReplicas = "redis_sentinel_replicas"

type RedisSentinel struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &RedisSentinel{}
	})
}

func (r *RedisSentinel) Clone() inputs.Input {
	return &RedisSentinel{}
}

func (r *RedisSentinel) Name() string {
	return inputName
}

func (r *RedisSentinel) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	Servers []string `toml:"servers"`
	clients []*RedisSentinelClient
	tls.ClientConfig
}

type RedisSentinelClient struct {
	sentinel *redis.SentinelClient
	tags     map[string]string
}

func (ins *Instance) Init() error {
	if len(ins.Servers) == 0 {
		return types.ErrInstancesEmpty
	}

	ins.clients = make([]*RedisSentinelClient, len(ins.Servers))
	tlsConfig, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return err
	}

	for i, serv := range ins.Servers {
		u, err := url.Parse(serv)
		if err != nil {
			return fmt.Errorf("unable to parse to address %q: %v", serv, err)
		}

		password := ""
		if u.User != nil {
			password, _ = u.User.Password()
		}

		var address string
		tags := map[string]string{}

		switch u.Scheme {
		case "tcp":
			address = u.Host
			tags["source"] = u.Hostname()
			tags["port"] = u.Port()
		case "unix":
			address = u.Path
			tags["socket"] = u.Path
		default:
			return fmt.Errorf("invalid scheme %q, expected tcp or unix", u.Scheme)
		}

		sentinel := redis.NewSentinelClient(
			&redis.Options{
				Addr:      address,
				Password:  password,
				Network:   u.Scheme,
				PoolSize:  1,
				TLSConfig: tlsConfig,
			},
		)

		ins.clients[i] = &RedisSentinelClient{
			sentinel: sentinel,
			tags:     tags,
		}
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup

	for _, client := range ins.clients {
		wg.Add(1)

		go func(slist *types.SampleList, client *RedisSentinelClient) {
			defer wg.Done()

			masters, err := client.gatherMasterStats(slist)
			if err != nil {
				log.Println("E! failed to gather master stats:", err)
			}

			for _, master := range masters {
				if err := client.gatherReplicaStats(slist, master); err != nil {
					log.Println("E! failed to gather replica stats:", err)
				}
				if err := client.gatherSentinelStats(slist, master); err != nil {
					log.Println("E! failed to gather sentinel stats:", err)
				}
			}

			if err := client.gatherInfoStats(slist); err != nil {
				log.Println("E! failed to gather info stats:", err)
			}
		}(slist, client)
	}

	wg.Wait()
}

func (client *RedisSentinelClient) gatherInfoStats(slist *types.SampleList) error {
	infoCmd := redis.NewStringCmd(context.Background(), "info", "all")
	if err := client.sentinel.Process(context.Background(), infoCmd); err != nil {
		return err
	}

	info, err := infoCmd.Result()
	if err != nil {
		return err
	}

	rdr := strings.NewReader(info)
	infoTags, infoFields, err := convertSentinelInfoOutput(client.tags, rdr)
	if err != nil {
		return err
	}

	slist.PushSamples(measurementSentinel, infoFields, infoTags)

	return nil
}

// convertSentinelInfoOutput parses `INFO` command output
// Largely copied from the Redis input plugin's gatherInfoOutput()
func convertSentinelInfoOutput(
	globalTags map[string]string,
	rdr io.Reader,
) (map[string]string, map[string]interface{}, error) {
	scanner := bufio.NewScanner(rdr)
	rawFields := make(map[string]string)

	tags := make(map[string]string, len(globalTags))
	for k, v := range globalTags {
		tags[k] = v
	}

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		// Redis denotes configuration sections with a hashtag
		// Example of the section header: # Clients
		if line[0] == '#' {
			// Nothing interesting here
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			// Not a valid configuration option
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		rawFields[key] = val
	}

	fields, err := prepareFieldValues(rawFields, measurementSentinelFields)
	if err != nil {
		return nil, nil, err
	}

	// Rename the field and convert it to nanoseconds
	secs, ok := fields["uptime_in_seconds"].(int64)
	if !ok {
		return nil, nil, fmt.Errorf("uptime type %T is not int64", fields["uptime_in_seconds"])
	}
	fields["uptime_ns"] = secs * 1000_000_000
	delete(fields, "uptime_in_seconds")

	// Rename in order to match the "redis" input plugin
	fields["clients"] = fields["connected_clients"]
	delete(fields, "connected_clients")

	return tags, fields, nil
}

func (client *RedisSentinelClient) gatherSentinelStats(slist *types.SampleList, masterName string) error {
	sentinelsCmd := redis.NewSliceCmd(context.Background(), "sentinel", "sentinels", masterName)
	if err := client.sentinel.Process(context.Background(), sentinelsCmd); err != nil {
		return err
	}

	sentinels, err := sentinelsCmd.Result()
	if err != nil {
		return err
	}

	// Break out of the loop if one of the items comes out malformed
	// It's safe to assume that if we fail parsing one item that the rest will fail too
	// This is because we are iterating over a single server response
	for _, sentinel := range sentinels {
		sentinel, ok := sentinel.([]interface{})
		if !ok {
			return fmt.Errorf("unable to process sentinel response")
		}

		sm := toMap(sentinel)
		sentinelTags, sentinelFields, err := convertSentinelSentinelsOutput(client.tags, masterName, sm)
		if err != nil {
			return err
		}

		slist.PushSamples(measurementSentinels, sentinelFields, sentinelTags)
	}

	return nil
}

// converts `sentinel sentinels <name>` output to tags and fields
func convertSentinelSentinelsOutput(
	globalTags map[string]string,
	masterName string,
	sentinelMaster map[string]string,
) (map[string]string, map[string]interface{}, error) {
	tags := make(map[string]string, len(globalTags))
	for k, v := range globalTags {
		tags[k] = v
	}

	tags["sentinel_ip"] = sentinelMaster["ip"]
	tags["sentinel_port"] = sentinelMaster["port"]
	tags["master"] = masterName

	fields, err := prepareFieldValues(sentinelMaster, measurementSentinelsFields)
	if err != nil {
		return nil, nil, err
	}

	return tags, fields, nil
}

func (client *RedisSentinelClient) gatherReplicaStats(slist *types.SampleList, masterName string) error {
	replicasCmd := redis.NewSliceCmd(context.Background(), "sentinel", "replicas", masterName)
	if err := client.sentinel.Process(context.Background(), replicasCmd); err != nil {
		return err
	}

	replicas, err := replicasCmd.Result()
	if err != nil {
		return err
	}

	// Break out of the loop if one of the items comes out malformed
	// It's safe to assume that if we fail parsing one item that the rest will fail too
	// This is because we are iterating over a single server response
	for _, replica := range replicas {
		replica, ok := replica.([]interface{})
		if !ok {
			return fmt.Errorf("unable to process replica response")
		}

		rm := toMap(replica)
		replicaTags, replicaFields, err := convertSentinelReplicaOutput(client.tags, masterName, rm)
		if err != nil {
			return err
		}

		slist.PushSamples(measurementReplicas, replicaFields, replicaTags)
	}

	return nil
}

// converts `sentinel replicas <name>` output to tags and fields
func convertSentinelReplicaOutput(
	globalTags map[string]string,
	masterName string,
	replica map[string]string,
) (map[string]string, map[string]interface{}, error) {
	tags := make(map[string]string, len(globalTags))
	for k, v := range globalTags {
		tags[k] = v
	}

	tags["replica_ip"] = replica["ip"]
	tags["replica_port"] = replica["port"]
	tags["master"] = masterName

	fields, err := prepareFieldValues(replica, measurementReplicasFields)
	if err != nil {
		return nil, nil, err
	}

	return tags, fields, nil
}

func (client *RedisSentinelClient) gatherMasterStats(slist *types.SampleList) ([]string, error) {
	mastersCmd := redis.NewSliceCmd(context.Background(), "sentinel", "masters")
	if err := client.sentinel.Process(context.Background(), mastersCmd); err != nil {
		return nil, err
	}

	masters, err := mastersCmd.Result()
	if err != nil {
		return nil, err
	}

	// Break out of the loop if one of the items comes out malformed
	// It's safe to assume that if we fail parsing one item that the rest will fail too
	// This is because we are iterating over a single server response
	masterNames := make([]string, 0, len(masters))
	for _, master := range masters {
		master, ok := master.([]interface{})
		if !ok {
			return masterNames, fmt.Errorf("unable to process master response")
		}

		m := toMap(master)

		masterName, ok := m["name"]
		if !ok {
			return masterNames, fmt.Errorf("unable to resolve master name")
		}

		masterNames = append(masterNames, masterName)
		quorumCmd := redis.NewStringCmd(context.Background(), "sentinel", "ckquorum", masterName)
		quorumErr := client.sentinel.Process(context.Background(), quorumCmd)

		sentinelMastersTags, sentinelMastersFields, err := convertSentinelMastersOutput(client.tags, m, quorumErr)
		if err != nil {
			return masterNames, err
		}

		slist.PushSamples(measurementMasters, sentinelMastersFields, sentinelMastersTags)
	}

	return masterNames, nil
}

// converts `sentinel masters <name>` output to tags and fields
func convertSentinelMastersOutput(
	globalTags map[string]string,
	master map[string]string,
	quorumErr error,
) (map[string]string, map[string]interface{}, error) {
	tags := make(map[string]string, len(globalTags))
	for k, v := range globalTags {
		tags[k] = v
	}

	tags["master"] = master["name"]

	fields, err := prepareFieldValues(master, measurementMastersFields)
	if err != nil {
		return nil, nil, err
	}

	fields["has_quorum"] = quorumErr == nil

	return tags, fields, nil
}

// Redis list format has string key/values adjacent, so convert to a map for easier use
func toMap(vals []interface{}) map[string]string {
	m := make(map[string]string)

	for idx := 0; idx < len(vals)-1; idx += 2 {
		key, keyOk := vals[idx].(string)
		value, valueOk := vals[idx+1].(string)

		if keyOk && valueOk {
			m[key] = value
		}
	}

	return m
}

func castFieldValue(value string, fieldType configFieldType) (interface{}, error) {
	var castedValue interface{}
	var err error

	switch fieldType {
	case configFieldTypeFloat:
		castedValue, err = strconv.ParseFloat(value, 64)
	case configFieldTypeInteger:
		castedValue, err = strconv.ParseInt(value, 10, 64)
	case configFieldTypeString:
		castedValue = value
	default:
		return nil, fmt.Errorf("unsupported field type %v", fieldType)
	}

	if err != nil {
		return nil, fmt.Errorf("casting value %v failed: %v", value, err)
	}

	return castedValue, nil
}

func prepareFieldValues(fields map[string]string, typeMap map[string]configFieldType) (map[string]interface{}, error) {
	preparedFields := make(map[string]interface{})

	for key, val := range fields {
		key = strings.Replace(key, "-", "_", -1)

		valType, ok := typeMap[key]
		if !ok {
			continue
		}

		castedVal, err := castFieldValue(val, valType)
		if err != nil {
			return nil, err
		}

		preparedFields[key] = castedVal
	}

	return preparedFields, nil
}
