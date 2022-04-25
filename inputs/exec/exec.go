package exec

import (
	"bytes"
	"fmt"
	"io"
	"log"
	osExec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/parser"
	"flashcat.cloud/categraf/parser/falcon"
	"flashcat.cloud/categraf/parser/influx"
	"flashcat.cloud/categraf/pkg/cmdx"
	"flashcat.cloud/categraf/types"
	"github.com/kballard/go-shellquote"
	"github.com/toolkits/pkg/container/list"
)

const inputName = "exec"
const MaxStderrBytes int = 512

type ExecInstance struct {
	Commands      []string        `toml:"commands"`
	Timeout       config.Duration `toml:"timeout"`
	IntervalTimes int64           `toml:"interval_times"`
	DataFormat    string          `toml:"data_format"`
	parser        parser.Parser
}

type Exec struct {
	Interval  config.Duration `toml:"interval"`
	Instances []ExecInstance  `toml:"instances"`
	Counter   uint64
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Exec{}
	})
}

func (e *Exec) GetInputName() string {
	return ""
}

func (e *Exec) Drop() {}

func (e *Exec) GetInterval() config.Duration {
	return e.Interval
}

func (e *Exec) Init() error {
	if len(e.Instances) == 0 {
		return types.ErrInstancesEmpty
	}

	for i := 0; i < len(e.Instances); i++ {
		if e.Instances[i].DataFormat == "" || e.Instances[i].DataFormat == "influx" {
			e.Instances[i].parser = influx.NewParser()
		} else if e.Instances[i].DataFormat == "falcon" {
			e.Instances[i].parser = falcon.NewParser()
		} else {
			return fmt.Errorf("data_format(%s) not supported", e.Instances[i].DataFormat)
		}

		if e.Instances[i].Timeout == 0 {
			e.Instances[i].Timeout = config.Duration(time.Second * 5)
		}
	}

	return nil
}

func (e *Exec) Gather(slist *list.SafeList) {
	atomic.AddUint64(&e.Counter, 1)

	var wg sync.WaitGroup
	wg.Add(len(e.Instances))
	for i := range e.Instances {
		ins := e.Instances[i]
		go e.GatherOnce(&wg, slist, ins)
	}

	wg.Wait()
}

func (e *Exec) GatherOnce(wg *sync.WaitGroup, slist *list.SafeList, ins ExecInstance) {
	defer wg.Done()

	if ins.IntervalTimes > 0 {
		counter := atomic.LoadUint64(&e.Counter)
		if counter%uint64(ins.IntervalTimes) != 0 {
			return
		}
	}

	var commands []string
	for _, pattern := range ins.Commands {
		cmdAndArgs := strings.SplitN(pattern, " ", 2)
		if len(cmdAndArgs) == 0 {
			continue
		}

		matches, err := filepath.Glob(cmdAndArgs[0])
		if err != nil {
			log.Println("E! failed to get filepath glob of commands:", err)
			continue
		}

		if len(matches) == 0 {
			// There were no matches with the glob pattern, so let's assume
			// that the command is in PATH and just run it as it is
			commands = append(commands, pattern)
		} else {
			// There were matches, so we'll append each match together with
			// the arguments to the commands slice
			for _, match := range matches {
				if len(cmdAndArgs) == 1 {
					commands = append(commands, match)
				} else {
					commands = append(commands,
						strings.Join([]string{match, cmdAndArgs[1]}, " "))
				}
			}
		}
	}

	if len(commands) == 0 {
		log.Println("W! no commands after parse")
		return
	}

	var waitCommands sync.WaitGroup
	waitCommands.Add(len(commands))
	for _, command := range commands {
		go e.ProcessCommand(slist, command, ins, &waitCommands)
	}

	waitCommands.Wait()
}

func (e *Exec) ProcessCommand(slist *list.SafeList, command string, ins ExecInstance, wg *sync.WaitGroup) {
	defer wg.Done()

	out, errbuf, runErr := commandRun(command, time.Duration(ins.Timeout))
	if runErr != nil || len(errbuf) > 0 {
		log.Println("E! exec_command:", command, "error:", runErr, "stderr:", string(errbuf))
		return
	}

	err := ins.parser.Parse(out, slist)
	if err != nil {
		log.Println("E! failed to parse command stdout:", err)
	}
}

func commandRun(command string, timeout time.Duration) ([]byte, []byte, error) {
	splitCmd, err := shellquote.Split(command)
	if err != nil || len(splitCmd) == 0 {
		return nil, nil, fmt.Errorf("exec: unable to parse command, %s", err)
	}

	cmd := osExec.Command(splitCmd[0], splitCmd[1:]...)

	var (
		out    bytes.Buffer
		stderr bytes.Buffer
	)
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	runError, runTimeout := cmdx.RunTimeout(cmd, timeout)
	if runTimeout {
		return nil, nil, fmt.Errorf("exec %s timeout", command)
	}

	if runError != nil {
		return nil, nil, runError
	}

	out = removeWindowsCarriageReturns(out)
	if stderr.Len() > 0 {
		stderr = removeWindowsCarriageReturns(stderr)
		stderr = truncate(stderr)
	}

	return out.Bytes(), stderr.Bytes(), nil
}

func truncate(buf bytes.Buffer) bytes.Buffer {
	// Limit the number of bytes.
	didTruncate := false
	if buf.Len() > MaxStderrBytes {
		buf.Truncate(MaxStderrBytes)
		didTruncate = true
	}
	if i := bytes.IndexByte(buf.Bytes(), '\n'); i > 0 {
		// Only show truncation if the newline wasn't the last character.
		if i < buf.Len()-1 {
			didTruncate = true
		}
		buf.Truncate(i)
	}
	if didTruncate {
		//nolint:errcheck,revive // Will always return nil or panic
		buf.WriteString("...")
	}
	return buf
}

// removeWindowsCarriageReturns removes all carriage returns from the input if the
// OS is Windows. It does not return any errors.
func removeWindowsCarriageReturns(b bytes.Buffer) bytes.Buffer {
	if runtime.GOOS == "windows" {
		var buf bytes.Buffer
		for {
			byt, err := b.ReadBytes(0x0D)
			byt = bytes.TrimRight(byt, "\x0d")
			if len(byt) > 0 {
				_, _ = buf.Write(byt)
			}
			if err == io.EOF {
				return buf
			}
		}
	}
	return b
}
