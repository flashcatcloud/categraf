//go:build !no_ibex

package ibex

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/sys"
	"k8s.io/klog/v2"

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

	outCh chan struct{}
	errCh chan struct{}
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

	buf := t.Stdout

	var out string

	switch runtime.GOOS {
	// window exec out charset is ANSI, convert to utf-8. (pwsh and cmd same)
	case "windows":
		b := buf.Bytes()
		decoded, err := ansiToUtf8(b)
		if err != nil {
			klog.ErrorS(err, "convert stdout from windows ansi failed", "task_id", t.Id)
			out = string(b)
		}
		out = decoded
	default:
		out = buf.String()
	}
	t.Unlock()
	return out
}

func (t *Task) GetStderr() string {
	t.Lock()

	buf := t.Stderr

	var out string
	switch runtime.GOOS {
	// window exec out charset is ANSI, convert to utf-8. (pwsh and cmd same)
	case "windows":
		b := buf.Bytes()
		decoded, err := ansiToUtf8(b)
		if err != nil {
			klog.ErrorS(err, "convert stderr from windows ansi failed", "task_id", t.Id)
			out = string(b)
		}
		out = decoded
	default:
		out = buf.String()
	}
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
		klog.ErrorS(err, "read task status file failed", "task_id", t.Id, "path", doneFlag)
	}
	stdout, err := file.ReadString(stdoutFile)
	if err != nil {
		klog.ErrorS(err, "read task stdout file failed", "task_id", t.Id, "path", stdoutFile)
	}
	stderr, err := file.ReadString(stderrFile)
	if err != nil {
		klog.ErrorS(err, "read task stderr file failed", "task_id", t.Id, "path", stderrFile)
	}

	t.Stdout = *bytes.NewBufferString(stdout)
	t.Stderr = *bytes.NewBufferString(stderr)
}

func (t *Task) prepare() error {
	if t.Account != "" {
		// already prepared
		return nil
	}
	t.pipeCreate()

	IdDir := filepath.Join(config.Config.Ibex.MetaDir, fmt.Sprint(t.Id))
	err := file.EnsureDir(IdDir)
	if err != nil {
		klog.ErrorS(err, "ensure task dir failed", "task_id", t.Id, "path", IdDir)
		return err
	}

	writeFlag := filepath.Join(IdDir, ".write")
	if file.IsExist(writeFlag) {
		// 从磁盘读取
		argsFile := filepath.Join(IdDir, "args")
		args, err := file.ReadStringTrim(argsFile)
		if err != nil {
			klog.ErrorS(err, "read task args file failed", "task_id", t.Id, "path", argsFile)
			return err
		}

		accountFile := filepath.Join(IdDir, "account")
		account, err := file.ReadStringTrim(accountFile)
		if err != nil {
			klog.ErrorS(err, "read task account file failed", "task_id", t.Id, "path", accountFile)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		stdin, err := file.ReadStringTrim(stdinFile)
		if err != nil {
			klog.ErrorS(err, "read task stdin file failed", "task_id", t.Id, "path", stdinFile)
			return err
		}

		t.Args = args
		t.Account = account
		t.StdinStr = stdin

	} else {
		// 从远端读取，再写入磁盘
		script, args, account, stdin, err := client.Meta(t.Id)
		if err != nil {
			klog.ErrorS(err, "query task meta failed", "task_id", t.Id)
			return err
		}

		switch runtime.GOOS {
		case "windows":
			// window command(cmd) only support ANSI and CRLF
			// if change to powershell , not convert script and stdin to ANSI and CRLF
			encodedStdin, err := utf8ToAnsi(stdin)
			if err != nil {
				klog.ErrorS(err, "convert stdin to windows ansi failed", "task_id", t.Id)
				return err
			}
			stdin = encodedStdin

			encodedArgs, err := utf8ToAnsi(args)
			if err != nil {
				klog.ErrorS(err, "convert args to windows ansi failed", "task_id", t.Id)
				return err
			}
			args = encodedArgs

			script = strings.ReplaceAll(script, "\r", "")
			script = strings.ReplaceAll(script, "\n", "\r\n")
			encodedScript, err := utf8ToAnsi(script)
			if err != nil {
				klog.ErrorS(err, "convert script to windows ansi failed", "task_id", t.Id)
				return err
			}

			scriptFile := filepath.Join(IdDir, "script.bat")
			_, err = file.WriteString(scriptFile, fmt.Sprintf("@echo off\r\n%s", encodedScript))
			if err != nil {
				klog.ErrorS(err, "write windows task script failed", "task_id", t.Id, "path", scriptFile)
				return err
			}
		default:
			scriptFile := filepath.Join(IdDir, "script")
			_, err = file.WriteString(scriptFile, script)
			if err != nil {
				klog.ErrorS(err, "write task script failed", "task_id", t.Id, "path", scriptFile)
				return err
			}
			out, err := sys.CmdOutTrim("chmod", "+x", scriptFile)
			if err != nil {
				klog.ErrorS(err, "chmod task script failed", "task_id", t.Id, "path", scriptFile, "output", out)
				return err
			}
		}

		argsFile := filepath.Join(IdDir, "args")
		_, err = file.WriteString(argsFile, args)
		if err != nil {
			klog.ErrorS(err, "write task args failed", "task_id", t.Id, "path", argsFile)
			return err
		}

		accountFile := filepath.Join(IdDir, "account")
		_, err = file.WriteString(accountFile, account)
		if err != nil {
			klog.ErrorS(err, "write task account failed", "task_id", t.Id, "path", accountFile)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		_, err = file.WriteString(stdinFile, stdin)
		if err != nil {
			klog.ErrorS(err, "write task stdin failed", "task_id", t.Id, "path", stdinFile)
			return err
		}

		_, err = file.WriteString(writeFlag, "")
		if err != nil {
			klog.ErrorS(err, "create task write flag failed", "task_id", t.Id, "path", writeFlag)
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
		klog.ErrorS(err, "cannot get current absolute path", "task_id", t.Id)
		return
	}

	sh := fmt.Sprintf("%s %s", scriptFile, args)
	var cmd *exec.Cmd

	loginUser, err := user.Current()
	if err != nil {
		klog.ErrorS(err, "cannot get current login user", "task_id", t.Id)
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

	cmd.Stdin = t.Stdin
	t.Cmd = cmd

	stdout, err := t.Cmd.StdoutPipe()
	if err != nil {
		klog.ErrorS(err, "cannot read stdout pipe", "task_id", t.Id)
	}

	stderr, err := t.Cmd.StderrPipe()

	if err != nil {
		klog.ErrorS(err, "cannot read stderr pipe", "task_id", t.Id)
	}

	err = CmdStart(cmd)

	if err != nil {
		klog.ErrorS(err, "cannot start task command", "task_id", t.Id)
		return
	}

	go runProcessRealtime(stdout, stderr, t)
}

func (t *Task) kill() {
	go killProcess(t)
}

func (t *Task) pipeDrain() {
	<-t.outCh
	<-t.errCh
}

func (t *Task) pipeCreate() {
	t.outCh = make(chan struct{})
	t.errCh = make(chan struct{})
}

func (t *Task) stdoutFlush() {
	metaDir := config.Config.Ibex.MetaDir
	stdoutFile := filepath.Join(metaDir, fmt.Sprint(t.Id), "stdout")
	file.WriteString(stdoutFile, t.GetStdout())
	close(t.outCh)
}

func (t *Task) stderrFlush() {
	metaDir := config.Config.Ibex.MetaDir
	stderrFile := filepath.Join(metaDir, fmt.Sprint(t.Id), "stderr")
	file.WriteString(stderrFile, t.GetStderr())
	close(t.errCh)
}

func runProcessRealtime(stdout io.ReadCloser, stderr io.ReadCloser, t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	reader := bufio.NewReader(stdout)

	go func() {
		defer t.stdoutFlush()
		for {
			line, err2 := reader.ReadString('\n')
			if len(line) != 0 {
				t.Stdout.WriteString(line)
			}
			if err2 != nil {
				if err2 != io.EOF {
					klog.Warningf("read stdout fail for task %d: %v", t.Id, err2)
				}
				break
			}
		}
	}()

	errReader := bufio.NewReader(stderr)

	go func() {
		defer t.stderrFlush()
		for {
			line, err2 := errReader.ReadString('\n')
			if len(line) != 0 {
				t.Stderr.WriteString(line)
			}
			if err2 != nil {
				if err2 != io.EOF {
					klog.Warningf("read stderr fail for task %d: %v", t.Id, err2)
				}
				break
			}
		}
	}()
	t.pipeDrain()
	err := t.Cmd.Wait()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			t.SetStatus("killed")
			klog.V(1).InfoS("process killed", "task_id", t.Id)
		} else if strings.Contains(err.Error(), "signal: terminated") {
			// kill children process manually
			t.SetStatus("killed")
			klog.V(1).InfoS("process terminated", "task_id", t.Id)
		} else {
			t.SetStatus("failed")
			klog.V(1).InfoS("process returned error", "task_id", t.Id, "error", err)
		}
	} else {
		t.SetStatus("success")
		klog.V(1).InfoS("process done", "task_id", t.Id)
	}

	persistResult(t)
}

func persistResult(t *Task) {
	metadir := config.Config.Ibex.MetaDir
	doneFlag := filepath.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	file.WriteString(doneFlag, t.GetStatus())
}

func killProcess(t *Task) {
	t.SetAlive(true)
	defer t.SetAlive(false)

	klog.V(1).InfoS("begin kill process", "task_id", t.Id)

	err := CmdKill(t.Cmd)
	if err != nil {
		t.SetStatus("killfailed")
		klog.V(1).InfoS("kill process failed", "task_id", t.Id, "error", err)
	} else {
		t.SetStatus("killed")
		klog.V(1).InfoS("process killed", "task_id", t.Id)
	}

	persistResult(t)
}
