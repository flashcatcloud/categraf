//go:build !windows && !no_ibex

package ibex

import (
	"sync"
	"testing"
	"time"
)

func TestStopAllTerminatesRunningTasks(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 10, 1000, "#!/bin/sh\nexec sleep 30\n")
	tasks := &LocalTasksT{M: map[int64]*Task{task.Id: task}, accepting: true}
	task.start()

	tasks.StopAll(3 * time.Second)
	if task.GetAlive() {
		t.Fatal("task still alive after StopAll")
	}
	if status := task.GetStatus(); status != "killed" {
		t.Fatalf("unexpected status: %s", status)
	}

	tasks.RLock()
	accepting := tasks.accepting
	tasks.RUnlock()
	if accepting {
		t.Fatal("task table still accepting assignments after StopAll")
	}
}

func TestLocalTasksConcurrentReportCleanAndStop(t *testing.T) {
	metaDir := configureIbexTest(t, 100*time.Millisecond)
	task := newScriptTask(t, metaDir, 11, 1100, "#!/bin/sh\nexec sleep 30\n")
	tasks := &LocalTasksT{M: map[int64]*Task{task.Id: task}, accepting: true}
	task.start()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = tasks.ReportTasks()
				tasks.Clean(map[int64]struct{}{task.Id: {}})
			}
		}()
	}
	wg.Wait()
	tasks.StopAll(3 * time.Second)
}
