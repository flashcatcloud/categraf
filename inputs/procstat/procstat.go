package procstat

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "procstat"

type PID int32

type Instance struct {
	config.InstanceConfig

	SearchExecSubstring    string   `toml:"search_exec_substring"`
	SearchCmdlineSubstring string   `toml:"search_cmdline_substring"`
	SearchWinService       string   `toml:"search_win_service"`
	SearchUser             string   `toml:"search_user"`
	Mode                   string   `toml:"mode"`
	GatherTotal            bool     `toml:"gather_total"`
	GatherPerPid           bool     `toml:"gather_per_pid"`
	GatherMoreMetrics      []string `toml:"gather_more_metrics"`

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
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Procstat{}
	})
}

func (p *Procstat) Clone() inputs.Input {
	return &Procstat{}
}

func (p *Procstat) Name() string {
	return inputName
}

var _ inputs.SampleGatherer = new(Instance)

func (s *Procstat) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

func UserFilter(username string) Filter {
	return func(p *process.Process) bool {
		if u, _ := p.Username(); u == username {
			return true
		}
		return false
	}
}

func (ins *Instance) Gather(slist *types.SampleList) {
	var (
		pids []PID
		err  error
		tags = map[string]string{"search_string": ins.searchString}
		opts = []Filter{}
	)

	if ins.SearchUser != "" {
		opts = append(opts, UserFilter(ins.SearchUser))
	}

	pg, _ := NewNativeFinder()
	if ins.SearchExecSubstring != "" {
		pids, err = pg.Pattern(ins.SearchExecSubstring, opts...)
	} else if ins.SearchCmdlineSubstring != "" {
		pids, err = pg.FullPattern(ins.SearchCmdlineSubstring, opts...)
	} else if ins.SearchWinService != "" {
		pids, err = ins.winServicePIDs()
	} else {
		log.Println("E! Oops... search string not found")
		return
	}

	if err != nil {
		log.Println("E! procstat: failed to lookup pids, search string:", ins.searchString, "error:", err)
		slist.PushFront(types.NewSample(inputName, "lookup_count", 0, tags))
		return
	}

	slist.PushFront(types.NewSample(inputName, "lookup_count", len(pids), tags))
	if len(pids) == 0 {
		return
	}

	if len(ins.GatherMoreMetrics) == 0 {
		return
	}

	ins.updateProcesses(pids)

	for _, field := range ins.GatherMoreMetrics {
		switch field {
		case "threads":
			ins.gatherThreads(slist, ins.procs, tags)
		case "fd":
			ins.gatherFD(slist, ins.procs, tags)
		case "io":
			ins.gatherIO(slist, ins.procs, tags)
		case "uptime":
			ins.gatherUptime(slist, ins.procs, tags)
		case "cpu":
			ins.gatherCPU(slist, ins.procs, tags, ins.solarisMode)
		case "mem":
			ins.gatherMem(slist, ins.procs, tags)
		case "limit":
			ins.gatherLimit(slist, ins.procs, tags)
		case "jvm":
			ins.gatherJvm(slist, ins.procs, tags)
		default:
			log.Println("E! unknown choice in gather_more_metrics:", field)
		}
	}
}

func (ins *Instance) updateProcesses(pids []PID) {
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

func (ins *Instance) makeProcTag(p Process) map[string]string {
	info := map[string]string{
		"pid": fmt.Sprint(p.PID()),
	}
	comm, err := p.Name()
	if err == nil {
		info["comm"] = comm
	}
	if runtime.GOOS == "windows" {
		title := getWindowTitleByPid(uint32(p.PID()))
		if len(title) != 0 {
			info["window_title"] = title
		}
	}
	return info
}

func (ins *Instance) gatherThreads(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	var val int32
	for pid := range procs {
		v, err := procs[pid].NumThreads()
		if err == nil {
			val += v
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "num_threads", val, ins.makeProcTag(procs[pid]), tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "num_threads_total", val, tags))
	}
}

func (ins *Instance) gatherFD(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	var val int32
	for pid := range procs {
		v, err := procs[pid].NumFDs()
		if err == nil {
			val += v
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "num_fds", val, ins.makeProcTag(procs[pid]), tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "num_fds_total", val, tags))
	}
}

func (ins *Instance) gatherIO(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
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
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "read_count", readCount, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "write_count", writeCount, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "read_bytes", readBytes, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "write_bytes", writeBytes, ins.makeProcTag(procs[pid]), tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "read_count_total", readCount, tags))
		slist.PushFront(types.NewSample(inputName, "write_count_total", writeCount, tags))
		slist.PushFront(types.NewSample(inputName, "read_bytes_total", readBytes, tags))
		slist.PushFront(types.NewSample(inputName, "write_bytes_total", writeBytes, tags))
	}
}

func (ins *Instance) gatherUptime(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	// use the smallest one
	var value int64 = -1
	now := time.Now().Unix()
	for pid := range procs {
		createTime, err := procs[pid].CreateTime() // returns epoch in ms
		if err == nil {
			v := now - createTime/1000
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "uptime", v, ins.makeProcTag(procs[pid]), tags))
			}
			if value == -1 {
				value = v
				continue
			}

			if value > v {
				value = v
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "uptime_minimum", value, tags))
	}
}

func (ins *Instance) gatherCPU(slist *types.SampleList, procs map[PID]Process, tags map[string]string, solarisMode bool) {
	var value float64
	for pid := range procs {
		v, err := procs[pid].Percent(time.Duration(0))
		if err == nil {
			if solarisMode {
				v /= float64(runtime.NumCPU())
			}
			value += v
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "cpu_usage", v, ins.makeProcTag(procs[pid]), tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "cpu_usage_total", value, tags))
	}
}

func (ins *Instance) gatherMem(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	var value float32
	for pid := range procs {
		v, err := procs[pid].MemoryPercent()
		if err == nil {
			value += v
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "mem_usage", v, ins.makeProcTag(procs[pid]), tags))
			}
		}

		minfo, err := procs[pid].MemoryInfo()
		if err == nil {
			if ins.GatherPerPid {
				slist.PushFront(types.NewSample(inputName, "mem_rss", minfo.RSS, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_vms", minfo.VMS, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_hwm", minfo.HWM, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_data", minfo.Data, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_stack", minfo.Stack, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_locked", minfo.Locked, ins.makeProcTag(procs[pid]), tags))
				slist.PushFront(types.NewSample(inputName, "mem_swap", minfo.Swap, ins.makeProcTag(procs[pid]), tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "mem_usage_total", value, tags))
	}
}

func (ins *Instance) gatherLimit(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	var softMin, hardMin uint64
	for pid := range procs {
		rlims, err := procs[pid].RlimitUsage(false)
		if err == nil {
			for _, rlim := range rlims {
				if rlim.Resource == process.RLIMIT_NOFILE {
					if ins.GatherPerPid {
						slist.PushFront(types.NewSample(inputName, "rlimit_num_fds_soft", rlim.Soft, ins.makeProcTag(procs[pid]), tags))
						slist.PushFront(types.NewSample(inputName, "rlimit_num_fds_hard", rlim.Hard, ins.makeProcTag(procs[pid]), tags))
					}

					if softMin == 0 {
						softMin = rlim.Soft
						hardMin = rlim.Hard
						continue
					}

					if softMin > rlim.Soft {
						softMin = rlim.Soft
					}

					if hardMin > rlim.Hard {
						hardMin = rlim.Hard
					}
				}
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample(inputName, "rlimit_num_fds_soft_minimum", softMin, tags))
		slist.PushFront(types.NewSample(inputName, "rlimit_num_fds_hard_minimum", hardMin, tags))
	}
}

func (ins *Instance) gatherJvm(slist *types.SampleList, procs map[PID]Process, tags map[string]string) {
	for pid := range procs {
		jvmStat, err := execJstat(pid)
		if err != nil {
			log.Println("E! failed to exec jstat:", err)
			continue
		}

		pidTag := map[string]string{"pid": fmt.Sprint(pid)}
		for k, v := range jvmStat {
			slist.PushSample(inputName, "jvm_"+k, v, pidTag, tags)
		}
	}
}

func execJstat(pid PID) (map[string]string, error) {
	bin, err := exec.LookPath("jstat")
	if err != nil {
		return nil, err
	}

	out, err := exec.Command(bin, "-gc", fmt.Sprint(pid)).Output()
	if err != nil {
		return nil, err
	}

	jvm := strings.Fields(string(out))
	if len(jvm)%2 != 0 {
		return nil, fmt.Errorf("failed to parse jstat output: %v", jvm)
	}

	jvmMetrics := make(map[string]string)
	half := len(jvm) / 2
	for i := 0; i < half; i++ {
		jvmMetrics[jvm[i]] = jvm[i+half]
	}

	return jvmMetrics, err
}

func (ins *Instance) winServicePIDs() ([]PID, error) {
	var pids []PID

	pid, err := queryPidWithWinServiceName(ins.SearchWinService)
	if err != nil {
		return pids, err
	}

	pids = append(pids, PID(pid))

	return pids, nil
}
