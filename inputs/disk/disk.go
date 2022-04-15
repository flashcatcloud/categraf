package disk

// import (
// 	"fmt"
// 	"log"
// 	"runtime"
// 	"strings"
// 	"time"

// 	"flashcat.cloud/categraf/config"
// 	"flashcat.cloud/categraf/inputs"
// 	"flashcat.cloud/categraf/inputs/system"
// 	"flashcat.cloud/categraf/types"
// )

// const InputName = "disk"

// type DiskStats struct {
// 	PrintConfigs    bool
// 	IntervalSeconds int64

// 	quit chan struct{}

// 	ps system.PS

// 	CollectPlatformFields bool
// }

// func init() {
// 	ps := system.NewSystemPS()
// 	inputs.Add(InputName, func() inputs.Input {
// 		return &DiskStats{
// 			ps:   ps,
// 			quit: make(chan struct{}),
// 		}
// 	})
// }

// func (s *DiskStats) getInterval() time.Duration {
// 	if s.IntervalSeconds != 0 {
// 		return time.Duration(s.IntervalSeconds) * time.Second
// 	}
// 	return config.GetInterval()
// }

// // overwrite func
// func (s *DiskStats) Init() error {
// 	s.platform = runtime.GOOS
// 	return nil
// }

// // overwrite func
// func (s *DiskStats) StopGoroutines() {
// 	s.quit <- struct{}{}
// }

// // overwrite func
// func (s *DiskStats) StartGoroutines(queue chan *types.Sample) {
// 	go s.LoopGather(queue)
// }

// func (s *DiskStats) LoopGather(queue chan *types.Sample) {
// 	interval := s.getInterval()
// 	for {
// 		select {
// 		case <-s.quit:
// 			close(s.quit)
// 			return
// 		default:
// 			time.Sleep(interval)
// 			s.Gather(queue)
// 		}
// 	}
// }

// // overwrite func
// func (s *DiskStats) Gather(queue chan *types.Sample) {
// 	var samples []*types.Sample

// 	defer func() {
// 		if r := recover(); r != nil {
// 			if strings.Contains(fmt.Sprint(r), "closed channel") {
// 				return
// 			} else {
// 				log.Println("E! gather metrics panic:", r)
// 			}
// 		}

// 		now := time.Now()
// 		for i := 0; i < len(samples); i++ {
// 			samples[i].Timestamp = now
// 			samples[i].Metric = InputName + "_" + samples[i].Metric
// 			queue <- samples[i]
// 		}
// 	}()

// 	// ----------------------------------------------

// 	vm, err := s.ps.VMStat()
// 	if err != nil {
// 		log.Println("E! failed to get vmstat:", err)
// 		return
// 	}

// 	fields := map[string]interface{}{
// 		"total":             vm.Total,     // bytes
// 		"available":         vm.Available, // bytes
// 		"used":              vm.Used,      // bytes
// 		"used_percent":      100 * float64(vm.Used) / float64(vm.Total),
// 		"available_percent": 100 * float64(vm.Available) / float64(vm.Total),
// 	}

// 	if s.CollectPlatformFields {
// 		switch s.platform {
// 		case "darwin":
// 			fields["active"] = vm.Active
// 			fields["free"] = vm.Free
// 			fields["inactive"] = vm.Inactive
// 			fields["wired"] = vm.Wired
// 		case "openbsd":
// 			fields["active"] = vm.Active
// 			fields["cached"] = vm.Cached
// 			fields["free"] = vm.Free
// 			fields["inactive"] = vm.Inactive
// 			fields["wired"] = vm.Wired
// 		case "freebsd":
// 			fields["active"] = vm.Active
// 			fields["buffered"] = vm.Buffers
// 			fields["cached"] = vm.Cached
// 			fields["free"] = vm.Free
// 			fields["inactive"] = vm.Inactive
// 			fields["laundry"] = vm.Laundry
// 			fields["wired"] = vm.Wired
// 		case "linux":
// 			fields["active"] = vm.Active
// 			fields["buffered"] = vm.Buffers
// 			fields["cached"] = vm.Cached
// 			fields["commit_limit"] = vm.CommitLimit
// 			fields["committed_as"] = vm.CommittedAS
// 			fields["dirty"] = vm.Dirty
// 			fields["free"] = vm.Free
// 			fields["high_free"] = vm.HighFree
// 			fields["high_total"] = vm.HighTotal
// 			fields["huge_pages_free"] = vm.HugePagesFree
// 			fields["huge_page_size"] = vm.HugePageSize
// 			fields["huge_pages_total"] = vm.HugePagesTotal
// 			fields["inactive"] = vm.Inactive
// 			fields["low_free"] = vm.LowFree
// 			fields["low_total"] = vm.LowTotal
// 			fields["mapped"] = vm.Mapped
// 			fields["page_tables"] = vm.PageTables
// 			fields["shared"] = vm.Shared
// 			fields["slab"] = vm.Slab
// 			fields["sreclaimable"] = vm.Sreclaimable
// 			fields["sunreclaim"] = vm.Sunreclaim
// 			fields["swap_cached"] = vm.SwapCached
// 			fields["swap_free"] = vm.SwapFree
// 			fields["swap_total"] = vm.SwapTotal
// 			fields["vmalloc_chunk"] = vm.VmallocChunk
// 			fields["vmalloc_total"] = vm.VmallocTotal
// 			fields["vmalloc_used"] = vm.VmallocUsed
// 			fields["write_back_tmp"] = vm.WriteBackTmp
// 			fields["write_back"] = vm.WriteBack
// 		}
// 	}

// 	samples = inputs.NewSamples(fields)
// }
