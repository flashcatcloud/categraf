package redis

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/conv"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
	"github.com/go-redis/redis/v8"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "redis"

var replicationSlaveMetricPrefix = regexp.MustCompile(`^slave\d+`)

type Command struct {
	Command []interface{} `toml:"command"`
	Metric  string        `toml:"metric"`
}

type Instance struct {
	Address           string            `toml:"address"`
	Username          string            `toml:"username"`
	Password          string            `toml:"password"`
	PoolSize          int               `toml:"pool_size"`
	Labels            map[string]string `toml:"labels"`
	IntervalTimes     int64             `toml:"interval_times"`
	Commands          []Command         `toml:"commands"`
	UseReplicaRoleTag bool              `toml:"use_replica_role_tag"`

	tls.ClientConfig
	client *redis.Client
}

func (ins *Instance) Init() error {
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
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`

	Counter uint64
	wg      sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Redis{}
	})
}

func (r *Redis) GetInputName() string {
	return inputName
}

func (r *Redis) GetInterval() config.Duration {
	return r.Interval
}

func (r *Redis) Init() error {
	if len(r.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(r.Instances); i++ {
		if err := r.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (r *Redis) Drop() {
	for i := 0; i < len(r.Instances); i++ {
		if r.Instances[i].client != nil {
			r.Instances[i].client.Close()
		}
	}
}

func (r *Redis) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&r.Counter, 1)

	slist := list.NewSafeList()

	for i := range r.Instances {
		ins := r.Instances[i]
		r.wg.Add(1)
		go r.gatherOnce(slist, ins)
	}
	r.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}

	return
}

func (r *Redis) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer r.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&r.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	tags := map[string]string{"address": ins.Address}
	for k, v := range ins.Labels {
		tags[k] = v
	}

	begun := time.Now()

	// scrape use seconds
	defer func(begun time.Time) {
		use := time.Since(begun).Seconds()
		slist.PushFront(inputs.NewSample("scrape_use_seconds", use, tags))
	}(begun)

	// ping
	err := ins.client.Ping(context.Background()).Err()
	slist.PushFront(inputs.NewSample("ping_use_seconds", time.Since(begun).Seconds(), tags))
	if err != nil {
		slist.PushFront(inputs.NewSample("up", 0, tags))
		log.Println("E! failed to ping redis:", ins.Address, "error:", err)
		return
	} else {
		slist.PushFront(inputs.NewSample("up", 1, tags))
	}

	r.gatherInfoAll(slist, ins, tags)
	r.gatherCommandValues(slist, ins, tags)
}

func (r *Redis) gatherCommandValues(slist *list.SafeList, ins *Instance, tags map[string]string) {
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
		slist.PushFront(inputs.NewSample("exec_result_"+k, v, tags))
	}
}

func (r *Redis) gatherInfoAll(slist *list.SafeList, ins *Instance, tags map[string]string) {
	info, err := ins.client.Info(context.Background(), "ALL").Result()
	if err != nil {
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

		// Treat it as a string
		if ins.UseReplicaRoleTag {
			if name == "role" {
				tags["replica_role"] = val
				continue
			}
		}

		// ignore other string fields
	}

	var keyspaceHitrate float64
	if keyspaceHits != 0 || keyspaceMisses != 0 {
		keyspaceHitrate = float64(keyspaceHits) / float64(keyspaceHits+keyspaceMisses)
	}
	fields["keyspace_hitrate"] = keyspaceHitrate

	for k, v := range fields {
		slist.PushFront(inputs.NewSample(k, v, tags))
	}
}

// Parse the special Keyspace line at end of redis stats
// This is a special line that looks something like:
//     db0:keys=2,expires=0,avg_ttl=0
// And there is one for each db on the redis instance
func gatherKeyspaceLine(
	name string,
	line string,
	slist *list.SafeList,
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
			slist.PushFront(inputs.NewSample("keyspace_"+k, v, tags))
		}
	}
}

// Parse the special cmdstat lines.
// Example:
//     cmdstat_publish:calls=33791,usec=208789,usec_per_call=6.18
// Tag: cmdstat=publish; Fields: calls=33791i,usec=208789i,usec_per_call=6.18
func gatherCommandstateLine(
	name string,
	line string,
	slist *list.SafeList,
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
		slist.PushFront(inputs.NewSample("cmdstat_"+k, v, tags))
	}
}

// Parse the special Replication line
// Example:
//     slave0:ip=127.0.0.1,port=7379,state=online,offset=4556468,lag=0
// This line will only be visible when a node has a replica attached.
func gatherReplicationLine(
	name string,
	line string,
	slist *list.SafeList,
	globalTags map[string]string,
) {
	fields := make(map[string]interface{})
	tags := make(map[string]string)
	for k, v := range globalTags {
		tags[k] = v
	}

	tags["replica_id"] = strings.TrimLeft(name, "slave")
	tags["replica_role"] = "slave"

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
		slist.PushFront(inputs.NewSample("replication_"+k, v, tags))
	}
}
