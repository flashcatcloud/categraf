package procstat

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
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
	OnlyGatherProcCount    bool              `toml:"only_gather_proc_count"`

	searchString string
	solarisMode  bool
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
	if ins.OnlyGatherProcCount {
		return
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
