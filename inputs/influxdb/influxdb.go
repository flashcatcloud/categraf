package influxdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "influxdb"

type Influxdb struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Influxdb{}
	})
}

func (c *Influxdb) Clone() inputs.Input {
	return &Influxdb{}
}

func (c *Influxdb) Name() string {
	return inputName
}

func (d *Influxdb) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(d.Instances))
	for i := 0; i < len(d.Instances); i++ {
		ret[i] = d.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig

	URLs     []string        `toml:"urls"`
	Username string          `toml:"username"`
	Password string          `toml:"password"`
	Timeout  config.Duration `toml:"timeout"`

	tls.ClientConfig
	client *http.Client
}

func (ins *Instance) Init() error {
	if len(ins.URLs) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.client == nil {
		tlsCfg, err := ins.ClientConfig.TLSConfig()
		if err != nil {
			return err
		}
		ins.client = &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: time.Duration(ins.Timeout),
				TLSClientConfig:       tlsCfg,
			},
			Timeout: time.Duration(ins.Timeout),
		}
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup
	for _, u := range ins.URLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if err := ins.gatherURL(slist, url); err != nil {
				slist.PushFront(types.NewSample(
					inputName,
					"up",
					0,
					map[string]string{"url": url},
				))
			} else {
				slist.PushFront(types.NewSample(
					inputName,
					"up",
					1,
					map[string]string{"url": url},
				))
			}
		}(u)
	}

	wg.Wait()
}

type point struct {
	Name   string                 `toml:"name"`
	Tags   map[string]string      `toml:"tags"`
	Values map[string]interface{} `toml:"values"`
}

type memstats struct {
	Alloc         int64      `toml:"alloc"`
	TotalAlloc    int64      `toml:"total_alloc"`
	Sys           int64      `toml:"sys"`
	Lookups       int64      `toml:"lookups"`
	Mallocs       int64      `toml:"mallocs"`
	Frees         int64      `toml:"frees"`
	HeapAlloc     int64      `toml:"heap_alloc"`
	HeapSys       int64      `toml:"heap_sys"`
	HeapIdle      int64      `toml:"heap_idle"`
	HeapInuse     int64      `toml:"heap_inuse"`
	HeapReleased  int64      `toml:"heap_released"`
	HeapObjects   int64      `toml:"heap_objects"`
	StackInuse    int64      `toml:"stack_inuse"`
	StackSys      int64      `toml:"stack_sys"`
	MSpanInuse    int64      `toml:"m_span_inuse"`
	MSpanSys      int64      `toml:"m_span_sys"`
	MCacheInuse   int64      `toml:"m_cache_inuse"`
	MCacheSys     int64      `toml:"m_cache_sys"`
	BuckHashSys   int64      `toml:"buck_hash_sys"`
	GCSys         int64      `toml:"gc_sys"`
	OtherSys      int64      `toml:"other_sys"`
	NextGC        int64      `toml:"next_gc"`
	LastGC        int64      `toml:"last_gc"`
	PauseTotalNs  int64      `toml:"pause_total_ns"`
	PauseNs       [256]int64 `toml:"pause_ns"`
	NumGC         int64      `toml:"num_gc"`
	GCCPUFraction float64    `toml:"gc_cpu_fraction"`
}

// Gathers data from a particular URL
// Parameters:
//
//	slist    : The telegraf slistumulator to use
//	url    : endpoint to send request to
//
// Returns:
//
//	error: Any error that may have occurred
func (ins *Instance) gatherURL(
	slist *types.SampleList,
	url string,
) error {
	shardCounter := 0

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if ins.Username != "" || ins.Password != "" {
		req.SetBasicAuth(ins.Username, ins.Password)
	}

	req.Header.Set("User-Agent", "Categraf")

	resp, err := ins.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readResponseError(resp)
	}

	// It would be nice to be able to decode into a map[string]point, but
	// we'll get a decoder error like:
	// `toml: cannot unmarshal array into Go value of type influxdb.point`
	// if any of the values aren't objects.
	// To avoid that error, we decode by hand.
	dec := json.NewDecoder(resp.Body)

	// Parse beginning of object
	if t, err := dec.Token(); err != nil {
		return err
	} else if t != json.Delim('{') {
		return errors.New("document root must be a JSON object")
	}

	// Loop through rest of object
	for {
		// Nothing left in this object, we're done
		if !dec.More() {
			break
		}

		// Read in a string key. We don't do anything with the top-level keys,
		// so it's discarded.
		key, err := dec.Token()
		if err != nil {
			return err
		}

		if keyStr, ok := key.(string); ok {
			if keyStr == "memstats" {
				var m memstats
				if err := dec.Decode(&m); err != nil {
					continue
				}

				for k, v := range map[string]interface{}{
					"alloc":           m.Alloc,
					"total_alloc":     m.TotalAlloc,
					"sys":             m.Sys,
					"lookups":         m.Lookups,
					"mallocs":         m.Mallocs,
					"frees":           m.Frees,
					"heap_alloc":      m.HeapAlloc,
					"heap_sys":        m.HeapSys,
					"heap_idle":       m.HeapIdle,
					"heap_inuse":      m.HeapInuse,
					"heap_released":   m.HeapReleased,
					"heap_objects":    m.HeapObjects,
					"stack_inuse":     m.StackInuse,
					"stack_sys":       m.StackSys,
					"mspan_inuse":     m.MSpanInuse,
					"mspan_sys":       m.MSpanSys,
					"mcache_inuse":    m.MCacheInuse,
					"mcache_sys":      m.MCacheSys,
					"buck_hash_sys":   m.BuckHashSys,
					"gc_sys":          m.GCSys,
					"other_sys":       m.OtherSys,
					"next_gc":         m.NextGC,
					"last_gc":         m.LastGC,
					"pause_total_ns":  m.PauseTotalNs,
					"pause_ns":        m.PauseNs[(m.NumGC+255)%256],
					"num_gc":          m.NumGC,
					"gc_cpu_fraction": m.GCCPUFraction,
				} {
					slist.PushFront(types.NewSample(
						inputName+"_memstats",
						k,
						v,
						map[string]string{
							"url": url,
						},
					))
				}
			}
		}

		// Attempt to parse a whole object into a point.
		// It might be a non-object, like a string or array.
		// If we fail to decode it into a point, ignore it and move on.
		var p point
		if err := dec.Decode(&p); err != nil {
			continue
		}

		if p.Tags == nil {
			p.Tags = make(map[string]string)
		}

		// If the object was a point, but was not fully initialized,
		// ignore it and move on.
		if p.Name == "" || p.Values == nil || len(p.Values) == 0 {
			continue
		}

		if p.Name == "shard" {
			shardCounter++
		}

		// Add a tag to indicate the source of the data.
		p.Tags["url"] = url

		for k, v := range p.Values {
			slist.PushFront(types.NewSample(inputName+"_"+p.Name, k, v, p.Tags))
		}
	}

	for k, v := range map[string]interface{}{
		"n_shards": shardCounter,
	} {
		slist.PushFront(types.NewSample(inputName, k, v, nil))
	}

	return nil
}

const (
	maxErrorResponseBodyLength = 1024
)

type APIError struct {
	StatusCode  int
	Reason      string
	Description string `toml:"error"`
}

func (e *APIError) Error() string {
	if e.Description != "" {
		return e.Reason + ": " + e.Description
	}
	return e.Reason
}

func readResponseError(resp *http.Response) error {
	apiError := &APIError{
		StatusCode: resp.StatusCode,
		Reason:     resp.Status,
	}

	var buf bytes.Buffer
	r := io.LimitReader(resp.Body, maxErrorResponseBodyLength)
	_, err := buf.ReadFrom(r)
	if err != nil {
		return apiError
	}

	err = json.Unmarshal(buf.Bytes(), apiError)
	if err != nil {
		return apiError
	}

	return apiError
}
