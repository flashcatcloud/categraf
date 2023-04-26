package redis

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/go-redis/redis/v8"
)

const inputName = "redis"

var replicationSlaveMetricPrefix = regexp.MustCompile(`^slave\d+`)

type Command struct {
	Command []interface{} `toml:"command"`
	Metric  string        `toml:"metric"`
}

type Instance struct {
	config.InstanceConfig

	Address  string    `toml:"address"`
	Username string    `toml:"username"`
	Password string    `toml:"password"`
	PoolSize int       `toml:"pool_size"`
	Commands []Command `toml:"commands"`

	tls.ClientConfig
	client *redis.Client
}

func (ins *Instance) Init() error {
	if ins.Address == "" {
		return types.ErrInstancesEmpty
	}

	redisOptions := &redis.Options{
		Addr:     ins.Address,
		Username: ins.Username,
		Password: ins.Password,
		PoolSize: ins.PoolSize,
	}

	if ins.UseTLS {
		tlsConfig, err := ins.TLSConfig()
		if err != nil {
			return fmt.Errorf("failed to init tls config: %v", err)
		}
		redisOptions.TLSConfig = tlsConfig
	}

	ins.client = redis.NewClient(redisOptions)
	return nil
}

type Redis struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Redis{}
	})
}

func (r *Redis) Clone() inputs.Input {
	return &Redis{}
}

func (r *Redis) Name() string {
	return inputName
}

func (r *Redis) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(r.Instances))
	for i := 0; i < len(r.Instances); i++ {
		ret[i] = r.Instances[i]
	}
	return ret
}

func (r *Redis) Drop() {
	for i := 0; i < len(r.Instances); i++ {
		if r.Instances[i].client != nil {
			r.Instances[i].client.Close()
		}
	}
}

func (ins *Instance) Gather(slist *types.SampleList) {
	tags := map[string]string{"address": ins.Address}
	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(types.NewSample(inputName, "scrape_use_seconds", use, tags))
	}(begun)

	// ping
	err := ins.client.Ping(context.Background()).Err()
	slist.PushFront(types.NewSample(inputName, "ping_use_seconds", time.Since(begun).Seconds(), tags))
	if err != nil {
		slist.PushFront(types.NewSample(inputName, "up", 0, tags))
		log.Println("E! failed to ping redis:", ins.Address, "error:", err)
		return
	} else {
		slist.PushFront(types.NewSample(inputName, "up", 1, tags))
	}

	ins.gatherInfoAll(slist, tags)
	ins.gatherCommandValues(slist, tags)
}

func (ins *Instance) gatherCommandValues(slist *types.SampleList, tags map[string]string) {
	fields := make(map[string]interface{})
	for _, cmd := range ins.Commands {
		val, err := ins.client.Do(context.Background(), cmd.Command...).Result()
		if err != nil {
			log.Println("E! failed to exec redis command:", cmd.Command)
			continue
		}

		fval, err := conv.ToFloat64(val)
		if err != nil {
			log.Println("E! failed to convert result of command:", cmd.Command, "error:", err)
			continue
		}

		fields[cmd.Metric] = fval
	}

	for k, v := range fields {
		slist.PushFront(types.NewSample(inputName, "exec_result_"+k, v, tags))
	}
}

func (ins *Instance) gatherInfoAll(slist *types.SampleList, tags map[string]string) {
	info, err := ins.client.Info(context.Background(), "ALL").Result()
	if err != nil || len(info) == 0 {
		info, err = ins.client.Info(context.Background()).Result()
	}

	if err != nil {
		log.Println("E! failed to call redis `info all`:", err)
		return
	}

	fields := make(map[string]interface{})

	var section string
	var keyspaceHits, keyspaceMisses int64

	scanner := bufio.NewScanner(strings.NewReader(info))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		if line[0] == '#' {
			if len(line) > 2 {
				section = line[2:]
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]

		if section == "Server" {
			// Server section only gather uptime_in_seconds
			if name != "uptime_in_seconds" {
				continue
			}
		}

		if strings.HasPrefix(name, "master_replid") {
			continue
		}

		if name == "mem_allocator" {
			continue
		}

		if strings.HasSuffix(name, "_human") {
			continue
		}

		if section == "Keyspace" {
			kline := strings.TrimSpace(parts[1])
			gatherKeyspaceLine(name, kline, slist, tags)
			continue
		}

		if section == "Commandstats" {
			kline := strings.TrimSpace(parts[1])
			gatherCommandstateLine(name, kline, slist, tags)
			continue
		}

		if section == "Replication" && replicationSlaveMetricPrefix.MatchString(name) {
			kline := strings.TrimSpace(parts[1])
			gatherReplicationLine(name, kline, slist, tags)
			continue
		}

		val := strings.TrimSpace(parts[1])

		// Some percentage values have a "%" suffix that we need to get rid of before int/float conversion
		val = strings.TrimSuffix(val, "%")

		// Try parsing as int
		if ival, err := strconv.ParseInt(val, 10, 64); err == nil {
			switch name {
			case "keyspace_hits":
				keyspaceHits = ival
			case "keyspace_misses":
				keyspaceMisses = ival
			case "rdb_last_save_time":
				// influxdb can't calculate this, so we have to do it
				fields["rdb_last_save_time_elapsed"] = time.Now().Unix() - ival
			}
			fields[name] = ival
			continue
		}

		// Try parsing as a float
		if fval, err := strconv.ParseFloat(val, 64); err == nil {
			fields[name] = fval
			continue
		}

		if fval, err := conv.ToFloat64(val); err == nil {
			fields[name] = fval
			continue
		}

		if name == "role" {
			tags["replica_role"] = val
			continue
		}

		// ignore other string fields
	}

	var keyspaceHitrate float64
	if keyspaceHits != 0 || keyspaceMisses != 0 {
		keyspaceHitrate = float64(keyspaceHits) / float64(keyspaceHits+keyspaceMisses)
	}
	fields["keyspace_hitrate"] = keyspaceHitrate

	for k, v := range fields {
		slist.PushFront(types.NewSample(inputName, k, v, tags))
	}
}

// Parse the special Keyspace line at end of redis stats
// This is a special line that looks something like:
//
//	db0:keys=2,expires=0,avg_ttl=0
//
// And there is one for each db on the redis instance
func gatherKeyspaceLine(
	name string,
	line string,
	slist *types.SampleList,
	globalTags map[string]string,
) {
	if strings.Contains(line, "keys=") {
		fields := make(map[string]interface{})
		tags := make(map[string]string)
		for k, v := range globalTags {
			tags[k] = v
		}
		tags["db"] = name
		dbparts := strings.Split(line, ",")
		for _, dbp := range dbparts {
			kv := strings.Split(dbp, "=")
			ival, err := strconv.ParseInt(kv[1], 10, 64)
			if err == nil {
				fields[kv[0]] = ival
			}
		}

		for k, v := range fields {
			slist.PushFront(types.NewSample(inputName, "keyspace_"+k, v, tags))
		}
	}
}

// Parse the special cmdstat lines.
// Example:
//
//	cmdstat_publish:calls=33791,usec=208789,usec_per_call=6.18
//
// Tag: cmdstat=publish; Fields: calls=33791i,usec=208789i,usec_per_call=6.18
func gatherCommandstateLine(
	name string,
	line string,
	slist *types.SampleList,
	globalTags map[string]string,
) {
	if !strings.HasPrefix(name, "cmdstat") {
		return
	}

	fields := make(map[string]interface{})
	tags := make(map[string]string)
	for k, v := range globalTags {
		tags[k] = v
	}
	tags["command"] = strings.TrimPrefix(name, "cmdstat_")
	parts := strings.Split(line, ",")
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "calls":
			fallthrough
		case "usec":
			ival, err := strconv.ParseInt(kv[1], 10, 64)
			if err == nil {
				fields[kv[0]] = ival
			}
		case "usec_per_call":
			fval, err := strconv.ParseFloat(kv[1], 64)
			if err == nil {
				fields[kv[0]] = fval
			}
		}
	}

	for k, v := range fields {
		slist.PushFront(types.NewSample(inputName, "cmdstat_"+k, v, tags))
	}
}

// Parse the special Replication line
// Example:
//
//	slave0:ip=127.0.0.1,port=7379,state=online,offset=4556468,lag=0
//
// This line will only be visible when a node has a replica attached.
func gatherReplicationLine(
	name string,
	line string,
	slist *types.SampleList,
	globalTags map[string]string,
) {
	fields := make(map[string]interface{})
	tags := make(map[string]string)
	for k, v := range globalTags {
		tags[k] = v
	}

	tags["replica_id"] = strings.TrimLeft(name, "slave")
	// tags["replica_role"] = "slave"

	parts := strings.Split(line, ",")
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "ip":
			tags["replica_ip"] = kv[1]
		case "port":
			tags["replica_port"] = kv[1]
		case "state":
			// ignore
		default:
			ival, err := strconv.ParseInt(kv[1], 10, 64)
			if err == nil {
				fields[kv[0]] = ival
			}
		}
	}

	for k, v := range fields {
		slist.PushFront(types.NewSample(inputName, "replication_"+k, v, tags))
	}
}
