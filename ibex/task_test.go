//go:build !windows && !no_ibex

package ibex

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
)

func configureIbexTest(t *testing.T, drainGrace time.Duration) string {
	t.Helper()
	oldConfig := config.Config
	metaDir := t.TempDir()
	config.Config = &config.ConfigType{
		Ibex: &config.IbexConfig{
			MetaDir:    metaDir,
			DrainGrace: config.Duration(drainGrace),
		},
	}
	t.Cleanup(func() { config.Config = oldConfig })
	return metaDir
}

func newScriptTask(t *testing.T, metaDir string, id, clock int64, script string) *Task {
	t.Helper()
	dir := filepath.Join(metaDir, stringID(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Task{
		Id:      id,
		Clock:   clock,
		Action:  "start",
		Account: "root", // A non-empty account skips the remote metadata lookup.
		Stdin:   bytes.NewReader(nil),
	}
}

func stringID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func waitTask(t *testing.T, task *Task, timeout time.Duration) {
	t.Helper()
	done := task.Done()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("task %d did not finish within %s", task.Id, timeout)
	}
}

func TestTaskReapsMainProcessWhenBackgroundInheritsPipes(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 1, 100, "#!/bin/sh\nsleep 30 &\nexit 0\n")

	started := time.Now()
	task.start()
	waitTask(t, task, 3*time.Second)

	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("task waited too long for inherited output pipes: %s", elapsed)
	}
	if status := task.GetStatus(); status != "success" {
		t.Fatalf("unexpected status: %s", status)
	}
	if task.GetAlive() {
		t.Fatal("task still marked alive after runner completed")
	}
	result, err := os.ReadFile(filepath.Join(metaDir, "1", "100.done"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "success" {
		t.Fatalf("unexpected persisted result: %q", result)
	}
}

func TestTaskPreservesOutputAndExitStatus(t *testing.T) {
	metaDir := configureIbexTest(t, time.Second)
	task := newScriptTask(t, metaDir, 2, 200, "#!/bin/sh\necho stdout-line\necho stderr-line >&2\nexit 42\n")

	task.start()
	waitTask(t, task, 3*time.Second)

	if status := task.GetStatus(); status != "failed" {
		t.Fatalf("unexpected status: %s", status)
	}
	if !strings.Contains(task.GetStdout(), "stdout-line") {
		t.Fatalf("stdout was not captured: %q", task.GetStdout())
	}
	if !strings.Contains(task.GetStderr(), "stderr-line") {
		t.Fatalf("stderr was not captured: %q", task.GetStderr())
	}
}

func TestTaskKillIsFinalizedByRunner(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 3, 300, "#!/bin/sh\nexec sleep 30\n")
	task.start()

	deadline := time.Now().Add(2 * time.Second)
	for {
		task.Lock()
		started := task.processStarted
		task.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task process did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	task.kill()
	waitTask(t, task, 3*time.Second)
	if status := task.GetStatus(); status != "killed" {
		t.Fatalf("unexpected status: %s", status)
	}
}

func TestPrepareFailureClosesDone(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	taskDir := filepath.Join(metaDir, "4")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, ".write"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	task := &Task{Id: 4, Clock: 400, Action: "start"}

	task.start()
	waitTask(t, task, 3*time.Second)
	if status := task.GetStatus(); status != "failed" {
		t.Fatalf("unexpected status: %s", status)
	}
	result, err := os.ReadFile(filepath.Join(taskDir, "400.done"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "failed" {
		t.Fatalf("unexpected persisted result: %q", result)
	}
}

func TestTaskKillBeforeProcessStart(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 5, 500, "#!/bin/sh\nexec sleep 30\n")
	clock, ok := task.beginRun()
	if !ok {
		t.Fatal("task did not enter running state")
	}
	task.kill()
	go task.run(clock)

	waitTask(t, task, 3*time.Second)
	if status := task.GetStatus(); status != "killed" {
		t.Fatalf("unexpected status: %s", status)
	}
}

func TestInactiveTaskDoneIsAlreadyClosed(t *testing.T) {
	task := &Task{done: make(chan struct{})}
	for name, done := range map[string]<-chan struct{}{
		"Done": task.Done(),
		"stop": task.stop(),
	} {
		select {
		case <-done:
		default:
			t.Fatalf("%s returned a channel that can never be closed", name)
		}
	}
}

func TestCmdStartFailureIsFinalizedByRunner(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 6, 600, "#!/bin/sh\nexit 0\n")
	task.startProcess = func(*exec.Cmd) error { return errors.New("injected start failure") }

	task.start()
	waitTask(t, task, 3*time.Second)
	if status := task.GetStatus(); status != "failed" {
		t.Fatalf("unexpected status: %s", status)
	}
	result, err := os.ReadFile(filepath.Join(metaDir, "6", "600.done"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "failed" {
		t.Fatalf("unexpected persisted result: %q", result)
	}
}

func TestStopAndKillAfterWaitDoNotOverrideNaturalCompletion(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	for i := 0; i < 25; i++ {
		id := int64(100 + i)
		script := "#!/bin/sh\nexit 0\n"
		expectedStatus := "success"
		if i%2 != 0 {
			script = "#!/bin/sh\nexit 42\n"
			expectedStatus = "failed"
		}
		task := newScriptTask(t, metaDir, id, id*10, script)
		waitReturned := make(chan struct{})
		finishWaitHandling := make(chan struct{})
		task.afterWait = func() {
			close(waitReturned)
			<-finishWaitHandling
		}
		tasks := &LocalTasksT{M: map[int64]*Task{id: task}, accepting: true}
		task.start()

		select {
		case <-waitReturned:
		case <-time.After(3 * time.Second):
			t.Fatalf("task %d did not return from Wait", id)
		}

		stopAllDone := make(chan struct{})
		go func() {
			tasks.StopAll(3 * time.Second)
			close(stopAllDone)
		}()
		var terminators sync.WaitGroup
		for j := 0; j < 10; j++ {
			terminators.Add(1)
			go func() {
				defer terminators.Done()
				task.kill()
				_ = task.stop()
			}()
		}
		terminators.Wait()
		close(finishWaitHandling)
		waitTask(t, task, 3*time.Second)
		select {
		case <-stopAllDone:
		case <-time.After(3 * time.Second):
			t.Fatalf("StopAll did not finish for task %d", id)
		}
		if status := task.GetStatus(); status != expectedStatus {
			t.Fatalf("task %d natural completion was overridden: got %s, want %s", id, status, expectedStatus)
		}
	}
}

func TestKillSignalFailureDoesNotOverrideNaturalExit(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 7, 700, "#!/bin/sh\nsleep 0.1\nexit 0\n")
	task.killProcess = func(*exec.Cmd) error { return errors.New("injected kill failure") }
	task.start()

	deadline := time.Now().Add(2 * time.Second)
	for {
		task.Lock()
		started := task.processStarted
		task.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task process did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	task.kill()
	waitTask(t, task, 3*time.Second)
	if status := task.GetStatus(); status != "success" {
		t.Fatalf("unexpected status after kill failure: %s", status)
	}
	result, err := os.ReadFile(filepath.Join(metaDir, "7", "700.done"))
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "success" {
		t.Fatalf("unexpected persisted result: %q", result)
	}
}

func TestTaskOutputWriterConcurrentAccess(t *testing.T) {
	task := &Task{}
	writer := taskOutputWriter{task: task}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = writer.Write([]byte("x"))
				_ = task.GetStdout()
			}
		}()
	}
	wg.Wait()
	if got := len(task.GetStdout()); got != 2000 {
		t.Fatalf("unexpected stdout length: %d", got)
	}
}
