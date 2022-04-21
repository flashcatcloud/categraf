package procstat

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "procstat"

type PID int32

type Instance struct {
	SearchExecSubstring    string            `toml:"search_exec_substring"`
	SearchCmdlineSubstring string            `toml:"search_cmdline_substring"`
	SearchWinService       string            `toml:"search_win_service"`
	Labels                 map[string]string `toml:"labels"`
	IntervalTimes          int64             `toml:"interval_times"`
	Mode                   string            `toml:"mode"`
	GatherMoreMetrics      []string          `toml:"gather_more_metrics"`

	searchString string
	solarisMode  bool
	procs        map[PID]Process
}

func (ins *Instance) Init() error {
	if ins.Mode == "" {
		ins.Mode = "irix"
	}

	if strings.ToLower(ins.Mode) == "solaris" {
		ins.solarisMode = true
	}

	if ins.SearchExecSubstring != "" {
		ins.searchString = ins.SearchExecSubstring
		log.Println("I! procstat: search_exec_substring:", ins.SearchExecSubstring)
	} else if ins.SearchCmdlineSubstring != "" {
		ins.searchString = ins.SearchCmdlineSubstring
		log.Println("I! procstat: search_cmdline_substring:", ins.SearchCmdlineSubstring)
	} else if ins.SearchWinService != "" {
		ins.searchString = ins.SearchWinService
		log.Println("I! procstat: search_win_service:", ins.SearchWinService)
	} else {
		return errors.New("the fields should not be all blank: search_exec_substring, search_cmdline_substring, search_win_service")
	}

	return nil
}

type Procstat struct {
	Interval  config.Duration `toml:"interval"`
	Instances []*Instance     `toml:"instances"`
	Counter   uint64
	wg        sync.WaitGroup
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Procstat{}
	})
}

func (s *Procstat) GetInputName() string {
	return inputName
}

func (s *Procstat) GetInterval() config.Duration {
	return s.Interval
}

func (s *Procstat) Init() error {
	if len(s.Instances) == 0 {
		return fmt.Errorf("instances empty")
	}

	for i := 0; i < len(s.Instances); i++ {
		if err := s.Instances[i].Init(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Procstat) Drop() {}

func (s *Procstat) Gather() (samples []*types.Sample) {
	atomic.AddUint64(&s.Counter, 1)

	slist := list.NewSafeList()

	for i := range s.Instances {
		ins := s.Instances[i]
		s.wg.Add(1)
		go s.gatherOnce(slist, ins)
	}
	s.wg.Wait()

	interfaceList := slist.PopBackAll()
	for i := 0; i < len(interfaceList); i++ {
		samples = append(samples, interfaceList[i].(*types.Sample))
	}
	return
}

func (s *Procstat) gatherOnce(slist *list.SafeList, ins *Instance) {
	defer s.wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&s.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	var (
		pids []PID
		err  error
		tags = map[string]string{"search_string": ins.searchString}
	)

	for k, v := range ins.Labels {
		tags[k] = v
	}

	pg, _ := NewNativeFinder()
	if ins.SearchExecSubstring != "" {
		pids, err = pg.Pattern(ins.SearchExecSubstring)
	} else if ins.SearchCmdlineSubstring != "" {
		pids, err = pg.FullPattern(ins.SearchCmdlineSubstring)
	} else if ins.SearchWinService != "" {
		pids, err = s.winServicePIDs(ins.SearchWinService)
	} else {
		log.Println("E! Oops... search string not found")
		return
	}

	if err != nil {
		log.Println("E! procstat: failed to lookup pids, search string:", ins.searchString, "error:", err)
		slist.PushFront(inputs.NewSample("lookup_count", 0, tags))
		return
	}

	slist.PushFront(inputs.NewSample("lookup_count", len(pids), tags))
	if len(pids) == 0 {
		return
	}

	if len(ins.GatherMoreMetrics) == 0 {
		return
	}

	s.updateProcesses(ins, pids)

	for _, field := range ins.GatherMoreMetrics {
		switch field {
		case "threads":
			s.gatherThreads(slist, ins.procs, tags)
		case "fd":
			s.gatherFD(slist, ins.procs, tags)
		case "io":
			s.gatherIO(slist, ins.procs, tags)
		case "uptime":
			s.gatherUptime(slist, ins.procs, tags)
		case "cpu":
			s.gatherCPU(slist, ins.procs, tags, ins.solarisMode)
		case "mem":
			s.gatherMem(slist, ins.procs, tags)
		case "limit":
			s.gatherLimit(slist, ins.procs, tags)
		default:
			log.Println("unknown choice in gather_more_metrics:", field)
		}
	}
}

func (s *Procstat) updateProcesses(ins *Instance, pids []PID) {
	procs := make(map[PID]Process)

	for _, pid := range pids {
		old, has := ins.procs[pid]
		if has {
			if name, err := old.Name(); err != nil || name == "" {
				continue
			}
			procs[pid] = old
		} else {
			proc, err := NewProc(pid)
			if err != nil {
				continue
			}

			if name, err := proc.Name(); err != nil || name == "" {
				continue
			}

			procs[pid] = proc
		}
	}

	ins.procs = procs
}

func (s *Procstat) gatherThreads(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	var val int32
	for pid := range procs {
		v, err := procs[pid].NumThreads()
		if err == nil {
			val += v
		}
	}
	slist.PushFront(inputs.NewSample("num_threads", val, tags))
}

func (s *Procstat) gatherFD(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	var val int32
	for pid := range procs {
		v, err := procs[pid].NumFDs()
		if err == nil {
			val += v
		}
	}
	slist.PushFront(inputs.NewSample("num_fds", val, tags))
}

func (s *Procstat) gatherIO(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	var (
		readCount  uint64
		writeCount uint64
		readBytes  uint64
		writeBytes uint64
	)

	for pid := range procs {
		io, err := procs[pid].IOCounters()
		if err == nil {
			readCount += io.ReadCount
			writeCount += io.WriteCount
			readBytes += io.ReadBytes
			writeBytes += io.WriteBytes
		}
	}

	slist.PushFront(inputs.NewSample("read_count", readCount, tags))
	slist.PushFront(inputs.NewSample("write_count", writeCount, tags))
	slist.PushFront(inputs.NewSample("read_bytes", readBytes, tags))
	slist.PushFront(inputs.NewSample("write_bytes", writeBytes, tags))
}

func (s *Procstat) gatherUptime(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	// use the smallest one
	var value int64 = -1
	for pid := range procs {
		v, err := procs[pid].CreateTime() // returns epoch in ms
		if err == nil {
			if value == -1 {
				value = v
				continue
			}

			if value > v {
				value = v
			}
		}
	}
	slist.PushFront(inputs.NewSample("uptime", value, tags))
}

func (s *Procstat) gatherCPU(slist *list.SafeList, procs map[PID]Process, tags map[string]string, solarisMode bool) {
	var value float64
	for pid := range procs {
		v, err := procs[pid].Percent(time.Duration(0))
		if err == nil {
			if solarisMode {
				value += v / float64(runtime.NumCPU())
			} else {
				value += v
			}
		}
	}
	slist.PushFront(inputs.NewSample("cpu_usage", value, tags))
}

func (s *Procstat) gatherMem(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	var value float32
	for pid := range procs {
		v, err := procs[pid].MemoryPercent()
		if err == nil {
			value += v
		}
	}
	slist.PushFront(inputs.NewSample("mem_usage", value, tags))
}

func (s *Procstat) gatherLimit(slist *list.SafeList, procs map[PID]Process, tags map[string]string) {
	// limit use the first one
	for pid := range procs {
		rlims, err := procs[pid].RlimitUsage(false)
		if err == nil {
			for _, rlim := range rlims {
				if rlim.Resource == process.RLIMIT_NOFILE {
					slist.PushFront(inputs.NewSample("rlimit_num_fds_soft", rlim.Soft, tags))
					slist.PushFront(inputs.NewSample("rlimit_num_fds_hard", rlim.Hard, tags))
					return
				}
			}
		}
	}
}

func (s *Procstat) winServicePIDs(winService string) ([]PID, error) {
	var pids []PID

	pid, err := queryPidWithWinServiceName(winService)
	if err != nil {
		return pids, err
	}

	pids = append(pids, PID(pid))

	return pids, nil
}
