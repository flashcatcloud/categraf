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
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/parser"
	"flashcat.cloud/categraf/parser/falcon"
	"flashcat.cloud/categraf/parser/influx"
	"flashcat.cloud/categraf/parser/prometheus"
	"flashcat.cloud/categraf/pkg/cmdx"
	"flashcat.cloud/categraf/types"
)

const inputName = "exec"

const MaxStderrBytes int = 512

type Instance struct {
	config.InstanceConfig

	Commands   []string        `toml:"commands"`
	Timeout    config.Duration `toml:"timeout"`
	DataFormat string          `toml:"data_format"`
	parser     parser.Parser
}

type Exec struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Exec{}
	})
}

func (e *Exec) Clone() inputs.Input {
	return &Exec{}
}

func (c *Exec) Name() string {
	return inputName
}

func (e *Exec) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(e.Instances))
	for i := 0; i < len(e.Instances); i++ {
		ret[i] = e.Instances[i]
	}
	return ret
}

func (ins *Instance) Init() error {
	if len(ins.Commands) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.DataFormat == "" || ins.DataFormat == "influx" {
		ins.parser = influx.NewParser()
	} else if ins.DataFormat == "falcon" {
		ins.parser = falcon.NewParser()
	} else if strings.HasPrefix(ins.DataFormat, "prom") {
		ins.parser = prometheus.EmptyParser()
	} else {
		return fmt.Errorf("data_format(%s) not supported", ins.DataFormat)
	}

	if ins.Timeout == 0 {
		ins.Timeout = config.Duration(time.Second * 5)
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
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
		go ins.ProcessCommand(slist, command, &waitCommands)
	}

	waitCommands.Wait()
}

func (ins *Instance) ProcessCommand(slist *types.SampleList, command string, wg *sync.WaitGroup) {
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
	splitCmd, err := QuoteSplit(command)
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
