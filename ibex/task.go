//go:build !no_ibex

package ibex

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/sys"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/client"
)

type Task struct {
	sync.Mutex

	Id     int64
	Clock  int64
	Action string
	Status string

	alive  bool
	Cmd    *exec.Cmd
	Stdout bytes.Buffer
	Stderr bytes.Buffer
	Stdin  *bytes.Reader

	Args     string
	Account  string
	StdinStr string
}

func (t *Task) SetStatus(status string) {
	t.Lock()
	t.Status = status
	t.Unlock()
}

func (t *Task) GetStatus() string {
	t.Lock()
	s := t.Status
	t.Unlock()
	return s
}

func (t *Task) GetAlive() bool {
	t.Lock()
	pa := t.alive
	t.Unlock()
	return pa
}

func (t *Task) SetAlive(pa bool) {
	t.Lock()
	t.alive = pa
	t.Unlock()
}

func (t *Task) GetStdout() string {
	t.Lock()
	out := t.Stdout.String()
	t.Unlock()
	return out
}

func (t *Task) GetStderr() string {
	t.Lock()
	out := t.Stderr.String()
	t.Unlock()
	return out
}

func (t *Task) ResetBuff() {
	t.Lock()
	t.Stdout.Reset()
	t.Stderr.Reset()
	t.Unlock()
}

func (t *Task) doneBefore() bool {
	doneFlag := filepath.Join(config.Config.Ibex.MetaDir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	return file.IsExist(doneFlag)
}

func (t *Task) loadResult() {
	metadir := config.Config.Ibex.MetaDir

	doneFlag := filepath.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	stdoutFile := filepath.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderrFile := filepath.Join(metadir, fmt.Sprint(t.Id), "stderr")

	var err error

	t.Status, err = file.ReadStringTrim(doneFlag)
	if err != nil {
		log.Printf("E! read file %s fail %v", doneFlag, err)
	}
	stdout, err := file.ReadString(stdoutFile)
	if err != nil {
		log.Printf("E! read file %s fail %v", stdoutFile, err)
	}
	stderr, err := file.ReadString(stderrFile)
	if err != nil {
		log.Printf("E! read file %s fail %v", stderrFile, err)
	}

	t.Stdout = *bytes.NewBufferString(stdout)
	t.Stderr = *bytes.NewBufferString(stderr)
}

func (t *Task) prepare() error {
	if t.Account != "" {
		// already prepared
		return nil
	}

	IdDir := filepath.Join(config.Config.Ibex.MetaDir, fmt.Sprint(t.Id))
	err := file.EnsureDir(IdDir)
	if err != nil {
		log.Printf("E! mkdir -p %s fail: %v", IdDir, err)
		return err
	}

	writeFlag := filepath.Join(IdDir, ".write")
	if file.IsExist(writeFlag) {
		// 从磁盘读取
		argsFile := filepath.Join(IdDir, "args")
		args, err := file.ReadStringTrim(argsFile)
		if err != nil {
			log.Printf("E! read %s fail %v", argsFile, err)
			return err
		}

		accountFile := filepath.Join(IdDir, "account")
		account, err := file.ReadStringTrim(accountFile)
		if err != nil {
			log.Printf("E! read %s fail %v", accountFile, err)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		stdin, err := file.ReadStringTrim(stdinFile)
		if err != nil {
			log.Printf("E: read %s fail %v", stdinFile, err)
			return err
		}

		t.Args = args
		t.Account = account
		t.StdinStr = stdin

	} else {
		// 从远端读取，再写入磁盘
		script, args, account, stdin, err := client.Meta(t.Id)
		if err != nil {
			log.Println("E! query task meta fail:", err)
			return err
		}

		switch runtime.GOOS {
		case "windows":
			scriptFile := filepath.Join(IdDir, "script.bat")
			_, err = file.WriteString(scriptFile, fmt.Sprintf("@echo off\r\n%s", script))
			if err != nil {
				log.Printf("E! write script to %s fail: %v", scriptFile, err)
				return err
			}
		default:
			scriptFile := filepath.Join(IdDir, "script")
			_, err = file.WriteString(scriptFile, script)
			if err != nil {
				log.Printf("E! write script to %s fail: %v", scriptFile, err)
				return err
			}
			out, err := sys.CmdOutTrim("chmod", "+x", scriptFile)
			if err != nil {
				log.Printf("E! chmod +x %s fail %v. output: %s", scriptFile, err, out)
				return err
			}
		}

		argsFile := filepath.Join(IdDir, "args")
		_, err = file.WriteString(argsFile, args)
		if err != nil {
			log.Printf("E! write args to %s fail: %v", argsFile, err)
			return err
		}

		accountFile := filepath.Join(IdDir, "account")
		_, err = file.WriteString(accountFile, account)
		if err != nil {
			log.Printf("E! write account to %s fail: %v", accountFile, err)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		_, err = file.WriteString(stdinFile, stdin)
		if err != nil {
			log.Printf("E: write tags to %s fail: %v", stdinFile, err)
			return err
		}

		_, err = file.WriteString(writeFlag, "")
		if err != nil {
			log.Printf("E! create %s flag file fail: %v", writeFlag, err)
			return err
		}

		t.Args = args
		t.Account = account
		t.StdinStr = stdin
	}

	t.Stdin = bytes.NewReader([]byte(t.StdinStr))

	return nil
}

func (t *Task) start() {
	if t.GetAlive() {
		return
	}

	err := t.prepare()
	if err != nil {
		return
	}

	args := t.Args
	if args != "" {
		args = strings.Replace(args, ",,", "' '", -1)
		args = "'" + args + "'"
	}

	scriptFileType := "script"
	if runtime.GOOS == "windows" {
		scriptFileType = "script.bat"
	}

	scriptFile, err := filepath.Abs(filepath.Join(config.Config.Ibex.MetaDir, fmt.Sprint(t.Id), scriptFileType))
	if err != nil {
		log.Println("E! cannot get current absolute path:", err)
		return
	}

	sh := fmt.Sprintf("%s %s", scriptFile, args)
	var cmd *exec.Cmd

	loginUser, err := user.Current()
	if err != nil {
		log.Println("E! cannot get current login user:", err)
		return
	}

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", sh)
	default:
		if loginUser.Username == "root" {
			// current login user is root
			if t.Account == "root" {
				cmd = exec.Command("sh", "-c", sh)
				cmd.Dir = loginUser.HomeDir
			} else {
				cmd = exec.Command("su", "-c", sh, "-", t.Account)
			}
		} else {
			// current login user not root
			cmd = exec.Command("sh", "-c", sh)
			cmd.Dir = loginUser.HomeDir
		}
	}

	//cmd.Stdout = &t.Stdout
	cmd.Stderr = &t.Stderr
	cmd.Stdin = t.Stdin
	t.Cmd = cmd

	var wg sync.WaitGroup
	wg.Add(2)

	runProcessRealtime(&wg, cmd, t)

	err = CmdStart(cmd)
	if err != nil {
		log.Printf("E! cannot start cmd of task[%d]: %v", t.Id, err)
		return
	}

	wg.Wait()

	//go runProcess(t)
}

func (t *Task) kill() {
	go killProcess(t)
}

func runProcessRealtime(wg *sync.WaitGroup, cmd *exec.Cmd, t *Task) {
	//捕获标准输出
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("INFO:", err)
		os.Exit(1)
	}
	readout := bufio.NewReader(stdout)
	go func() {
		defer wg.Done()
		GetOutput(readout, t)
	}()
}

func GetOutput(reader *bufio.Reader, t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	var sumOutput string //统计屏幕的全部输出内容
	outputBytes := make([]byte, 200)
	for {
		n, err := reader.Read(outputBytes) //获取屏幕的实时输出(并不是按照回车分割，所以要结合sumOutput)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println(err)
			sumOutput += err.Error()
		}
		output := string(outputBytes[:n])
		//fmt.Print(output) //输出屏幕内容

		persistResult(t, output)
		sumOutput += output
	}

	err := t.Cmd.Wait()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			t.SetStatus("killed")
			log.Printf("D! process of task[%d] killed", t.Id)
		} else if strings.Contains(err.Error(), "signal: terminated") {
			// kill children process manually
			t.SetStatus("killed")
			log.Printf("D! process of task[%d] terminated", t.Id)
		} else {
			t.SetStatus("failed")
			log.Printf("D! process of task[%d] return error: %v", t.Id, err)
		}
	} else {
		t.SetStatus("success")
		log.Printf("D! process of task[%d] done", t.Id)
	}
}

func runProcess(t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	err := t.Cmd.Wait()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			t.SetStatus("killed")
			log.Printf("D! process of task[%d] killed", t.Id)
		} else if strings.Contains(err.Error(), "signal: terminated") {
			// kill children process manually
			t.SetStatus("killed")
			log.Printf("D! process of task[%d] terminated", t.Id)
		} else {
			t.SetStatus("failed")
			log.Printf("D! process of task[%d] return error: %v", t.Id, err)
		}
	} else {
		t.SetStatus("success")
		log.Printf("D! process of task[%d] done", t.Id)
	}

	//persistResult(t)
}

func persistResult(t *Task, msg string) {
	metadir := config.Config.Ibex.MetaDir
	stdout := filepath.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderr := filepath.Join(metadir, fmt.Sprint(t.Id), "stderr")
	doneFlag := filepath.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))

	fmt.Println("Output ====> ", msg)
	file.WriteString(stdout, msg)
	file.WriteString(stderr, t.GetStderr())
	file.WriteString(doneFlag, t.GetStatus())
}

func killProcess(t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	log.Printf("D! begin kill process of task[%d]", t.Id)

	err := CmdKill(t.Cmd)
	if err != nil {
		t.SetStatus("killfailed")
		log.Printf("D! kill process of task[%d] fail: %v", t.Id, err)
	} else {
		t.SetStatus("killed")
		log.Printf("D! process of task[%d] killed", t.Id)
	}

	//persistResult(t)
}
