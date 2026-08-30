//go:build !no_ibex

package ibex

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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

	done                chan struct{}
	processStarted      bool
	completing          bool
	terminationRequests chan string

	// Process hooks use the production implementations by default and are
	// overridden only by lifecycle boundary tests.
	startProcess func(*exec.Cmd) error
	killProcess  func(*exec.Cmd) error
	afterWait    func()
}

const defaultDrainGrace = 5 * time.Second

var inactiveTaskDone = func() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}()

type taskOutputWriter struct {
	task   *Task
	stderr bool
}

func (w taskOutputWriter) Write(p []byte) (int, error) {
	w.task.Lock()
	defer w.task.Unlock()
	if w.stderr {
		return w.task.Stderr.Write(p)
	}
	return w.task.Stdout.Write(p)
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
			log.Printf("E! convert out to windows-ansi fail: %v", err)
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
			log.Printf("E! convert out to windows-ansi fail: %v", err)
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

func (t *Task) GetClock() int64 {
	t.Lock()
	defer t.Unlock()
	return t.Clock
}

func (t *Task) GetAction() string {
	t.Lock()
	defer t.Unlock()
	return t.Action
}

func (t *Task) SetAssignment(clock int64, action string) {
	t.Lock()
	t.Clock = clock
	t.Action = action
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

	status, err := file.ReadStringTrim(doneFlag)
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

	t.Lock()
	t.Status = status
	t.Stdout = *bytes.NewBufferString(stdout)
	t.Stderr = *bytes.NewBufferString(stderr)
	t.Unlock()
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
			// window command(cmd) only support ANSI and CRLF
			// if change to powershell , not convert script and stdin to ANSI and CRLF
			encodedStdin, err := utf8ToAnsi(stdin)
			if err != nil {
				log.Printf("E! convert stdin[%s] to windows-ansi fail: %v", stdin, err)
				return err
			}
			stdin = encodedStdin

			encodedArgs, err := utf8ToAnsi(args)
			if err != nil {
				log.Printf("E! convert args[%s] to windows-ansi fail: %v", args, err)
				return err
			}
			args = encodedArgs

			script = strings.ReplaceAll(script, "\r", "")
			script = strings.ReplaceAll(script, "\n", "\r\n")
			encodedScript, err := utf8ToAnsi(script)
			if err != nil {
				log.Printf("E! convert script to windows-ansi fail: %v", err)
				return err
			}

			scriptFile := filepath.Join(IdDir, "script.bat")
			_, err = file.WriteString(scriptFile, fmt.Sprintf("@echo off\r\n%s", encodedScript))
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

func (t *Task) beginRun() (int64, bool) {
	t.Lock()
	defer t.Unlock()
	if t.alive {
		return 0, false
	}

	t.alive = true
	t.Status = "running"
	t.Cmd = nil
	t.done = make(chan struct{})
	t.processStarted = false
	t.completing = false
	t.terminationRequests = make(chan string, 1)
	return t.Clock, true
}

func (t *Task) start() {
	clock, ok := t.beginRun()
	if !ok {
		return
	}
	go t.run(clock)
}

func (t *Task) newCommand() (*exec.Cmd, error) {

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
		return nil, fmt.Errorf("cannot get current absolute path: %w", err)
	}

	sh := fmt.Sprintf("%s %s", scriptFile, args)
	var cmd *exec.Cmd

	loginUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("cannot get current login user: %w", err)
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
	cmd.Stdout = taskOutputWriter{task: t}
	cmd.Stderr = taskOutputWriter{task: t, stderr: true}
	cmd.WaitDelay = ibexDrainGrace()
	return cmd, nil
}

func (t *Task) kill() {
	t.requestTermination("kill")
}

func (t *Task) stop() <-chan struct{} {
	return t.requestTermination("stop")
}

func (t *Task) Done() <-chan struct{} {
	t.Lock()
	defer t.Unlock()
	if !t.alive || t.done == nil {
		return inactiveTaskDone
	}
	return t.done
}

func (t *Task) requestTermination(reason string) <-chan struct{} {
	t.Lock()
	if !t.alive {
		t.Unlock()
		return inactiveTaskDone
	}
	done := t.done
	if t.completing || t.terminationRequests == nil {
		t.Unlock()
		return done
	}
	requests := t.terminationRequests
	t.Unlock()

	select {
	case requests <- reason:
	default:
	}
	return done
}

type terminationResult struct {
	signaled bool
}

func (t *Task) handleTermination(cmd *exec.Cmd, stop <-chan struct{}, done chan<- terminationResult) {
	result := terminationResult{}
	defer func() { done <- result }()
	for {
		select {
		case <-stop:
			return
		case reason := <-t.terminationRequests:
			t.SetStatus("killing")
			if err := t.killCommand(cmd); err != nil {
				log.Printf("W! terminate process group of task[%d] for %s request failed: %v", t.Id, reason, err)
				continue
			}
			result.signaled = true
			return
		}
	}
}

func (t *Task) run(clock int64) {
	status := "failed"
	defer func() {
		t.finalize(clock, status)
	}()

	if err := t.prepare(); err != nil {
		log.Printf("E! prepare task[%d] failed: %v", t.Id, err)
		return
	}

	cmd, err := t.newCommand()
	if err != nil {
		log.Printf("E! build command of task[%d] failed: %v", t.Id, err)
		return
	}
	t.Lock()
	t.Cmd = cmd
	t.Unlock()

	if err = t.startCommand(cmd); err != nil {
		log.Printf("E! cannot start cmd of task[%d]: %v", t.Id, err)
		return
	}
	t.Lock()
	t.processStarted = true
	t.Unlock()

	stopTermination := make(chan struct{})
	terminationDone := make(chan terminationResult, 1)
	go t.handleTermination(cmd, stopTermination, terminationDone)
	waitErr := cmd.Wait()
	if t.afterWait != nil {
		t.afterWait()
	}
	t.Lock()
	t.completing = true
	t.Unlock()
	close(stopTermination)
	termination := <-terminationDone
	if errors.Is(waitErr, exec.ErrWaitDelay) {
		log.Printf("W! task[%d] output drain exceeded %s; stdout/stderr may be truncated", t.Id, ibexDrainGrace())
	}

	// Ibex tasks are one-shot jobs. Clean up descendants that stayed in the
	// task's process group after the main process exited. An error here usually
	// means that the process group is already gone.
	if err := t.killCommand(cmd); err == nil {
		log.Printf("W! cleaned remaining process group of task[%d] after main process exit; stdout/stderr may be truncated", t.Id)
	}

	status = statusFor(waitErr, cmd, termination)
	if status == "success" {
		log.Printf("D! process of task[%d] done", t.Id)
	} else if status == "killed" {
		log.Printf("D! process of task[%d] killed", t.Id)
	} else {
		log.Printf("D! process of task[%d] return error: %v", t.Id, waitErr)
	}
}

func statusFor(waitErr error, cmd *exec.Cmd, termination terminationResult) string {
	if waitErr == nil || errors.Is(waitErr, exec.ErrWaitDelay) {
		return "success"
	}
	if termination.signaled && terminationCausedExit(cmd) {
		return "killed"
	}
	return "failed"
}

func (t *Task) finalize(clock int64, status string) {
	t.SetStatus(status)
	t.flushOutput()
	persistResult(t, clock, status)

	t.Lock()
	t.alive = false
	close(t.done)
	t.Unlock()
}

func (t *Task) startCommand(cmd *exec.Cmd) error {
	if t.startProcess != nil {
		return t.startProcess(cmd)
	}
	return CmdStart(cmd)
}

func (t *Task) killCommand(cmd *exec.Cmd) error {
	if t.killProcess != nil {
		return t.killProcess(cmd)
	}
	return CmdKill(cmd)
}

func (t *Task) flushOutput() {
	metaDir := config.Config.Ibex.MetaDir
	stdoutFile := filepath.Join(metaDir, fmt.Sprint(t.Id), "stdout")
	stderrFile := filepath.Join(metaDir, fmt.Sprint(t.Id), "stderr")
	if _, err := file.WriteString(stdoutFile, t.GetStdout()); err != nil {
		log.Printf("E! write task[%d] stdout failed: %v", t.Id, err)
	}
	if _, err := file.WriteString(stderrFile, t.GetStderr()); err != nil {
		log.Printf("E! write task[%d] stderr failed: %v", t.Id, err)
	}
}

func persistResult(t *Task, clock int64, status string) {
	metadir := config.Config.Ibex.MetaDir
	doneFlag := filepath.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", clock))
	if _, err := file.WriteString(doneFlag, status); err != nil {
		log.Printf("E! persist result of task[%d] failed: %v", t.Id, err)
	}
}

func ibexDrainGrace() time.Duration {
	if config.Config != nil && config.Config.Ibex != nil && config.Config.Ibex.DrainGrace > 0 {
		return time.Duration(config.Config.Ibex.DrainGrace)
	}
	return defaultDrainGrace
}
