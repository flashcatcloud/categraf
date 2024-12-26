package bind

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"flashcat.cloud/categraf/types"
)

type jsonStats struct {
	OpCodes   map[string]int
	QTypes    map[string]int
	RCodes    map[string]int
	ZoneStats map[string]int
	NSStats   map[string]int
	SockStats map[string]int
	Views     map[string]jsonView
	Memory    jsonMemory
}

type jsonMemory struct {
	TotalUse    int64
	InUse       int64
	BlockSize   int64
	ContextSize int64
	Lost        int64
	Contexts    []struct {
		ID    string
		Name  string
		Total int64
		InUse int64
	}
}

type jsonView struct {
	Resolver map[string]map[string]int
}

// addJSONCounter adds a counter array to a sample list, with the specified tags.
func addJSONCounter(slist *types.SampleList, commonTags map[string]string, stats map[string]int) {
	for name, value := range stats {
		if commonTags["type"] == "opcode" && strings.HasPrefix(name, "RESERVED") {
			continue
		}

		tags := make(map[string]string)

		// Create local copy of tags since maps are reference types
		for k, v := range commonTags {
			tags[k] = v
		}
		slist.PushSample("bind_counter", name, value, tags)
	}
}

// addStatsJson walks a jsonStats struct and adds the values to the sample list
func (b *Instance) addStatsJSON(stats jsonStats, slist *types.SampleList, urlTag string) {
	tags := map[string]string{"url": urlTag}
	host, port, _ := net.SplitHostPort(urlTag)
	tags["source"] = host
	tags["port"] = port

	// Opcodes
	tags["type"] = "opcode"
	addJSONCounter(slist, tags, stats.OpCodes)

	// RCodes stats
	tags["type"] = "rcode"
	addJSONCounter(slist, tags, stats.RCodes)

	// Query RDATA types
	tags["type"] = "qtype"
	addJSONCounter(slist, tags, stats.QTypes)

	// Nameserver stats
	tags["type"] = "nsstat"
	addJSONCounter(slist, tags, stats.NSStats)

	// Socket statistics
	tags["type"] = "sockstat"
	addJSONCounter(slist, tags, stats.SockStats)

	// Zonestats
	tags["type"] = "zonestat"
	addJSONCounter(slist, tags, stats.ZoneStats)

	// Memory stats
	fields := map[string]interface{}{
		"total_use":    stats.Memory.TotalUse,
		"in_use":       stats.Memory.InUse,
		"block_size":   stats.Memory.BlockSize,
		"context_size": stats.Memory.ContextSize,
		"lost":         stats.Memory.Lost,
	}
	slist.PushSamples("bind_memory", fields, map[string]string{"url": urlTag, "source": host, "port": port})

	// Detailed, per-context memory stats
	if b.GatherMemoryContexts {
		for _, c := range stats.Memory.Contexts {
			tags := map[string]string{"url": urlTag, "id": c.ID, "name": c.Name, "source": host, "port": port}
			fields := map[string]interface{}{"total": c.Total, "in_use": c.InUse}

			slist.PushSamples("bind_memory_context", fields, tags)
		}
	}

	// Detailed, per-view stats
	if b.GatherViews {
		for vName, view := range stats.Views {
			for cntrType, counters := range view.Resolver {
				for cntrName, value := range counters {
					tags := map[string]string{
						"url":    urlTag,
						"source": host,
						"port":   port,
						"view":   vName,
						"type":   cntrType,
					}

					slist.PushSample("bind_counter", cntrName, value, tags)
				}
			}
		}
	}
}

// readStatsJSON takes a base URL to probe, and requests the individual statistics blobs that we
// are interested in. These individual blobs have a combined size which is significantly smaller
// than if we requested everything at once (e.g. taskmgr and socketmgr can be omitted).
func (b *Instance) readStatsJSON(addr *url.URL, slist *types.SampleList) error {
	var stats jsonStats

	// Progressively build up full jsonStats struct by parsing the individual HTTP responses
	for _, suffix := range [...]string{"/server", "/net", "/mem"} {
		err := func() error {
			scrapeURL := addr.String() + suffix

			resp, err := b.client.Get(scrapeURL)
			if err != nil {
				return err
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%s returned HTTP status: %s", scrapeURL, resp.Status)
			}

			if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
				return fmt.Errorf("unable to decode JSON blob: %w", err)
			}

			return nil
		}()

		if err != nil {
			return err
		}
	}

	b.addStatsJSON(stats, slist, addr.Host)
	return nil
}
