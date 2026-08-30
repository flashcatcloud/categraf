//go:build !no_ibex

package ibex

import (
	"log"
	"sync"
	"time"

	"flashcat.cloud/categraf/ibex/types"
)

type LocalTasksT struct {
	sync.RWMutex
	M         map[int64]*Task
	accepting bool
}

var Locals = &LocalTasksT{M: make(map[int64]*Task), accepting: true}

func (lt *LocalTasksT) ReportTasks() []types.ReportTask {
	lt.RLock()
	tasks := make(map[int64]*Task, len(lt.M))
	for id, task := range lt.M {
		tasks[id] = task
	}
	lt.RUnlock()

	ret := make([]types.ReportTask, 0, len(tasks))
	for id, task := range tasks {
		rt := types.ReportTask{Id: id, Clock: task.GetClock()}
		rt.Status = task.GetStatus()
		if rt.Status == "killing" {
			continue
		}

		rt.Stdout = task.GetStdout()
		rt.Stderr = task.GetStderr()

		stdoutRunes := []rune(rt.Stdout)
		stderrRunes := []rune(rt.Stderr)
		if len(stdoutRunes) > 16380 {
			rt.Stdout = string(stdoutRunes[:16380]) + "..."
		}
		if len(stderrRunes) > 16380 {
			rt.Stderr = string(stderrRunes[:16380]) + "..."
		}
		ret = append(ret, rt)
	}

	return ret
}

func (lt *LocalTasksT) GetTask(id int64) (*Task, bool) {
	lt.RLock()
	defer lt.RUnlock()
	task, found := lt.M[id]
	return task, found
}

func (lt *LocalTasksT) SetTask(task *Task) {
	lt.Lock()
	lt.M[task.Id] = task
	lt.Unlock()
}

func (lt *LocalTasksT) AssignTask(at types.AssignTask) {
	lt.Lock()
	if !lt.accepting {
		lt.Unlock()
		log.Printf("W! ignore task %d action %s while ibex is stopping", at.Id, at.Action)
		return
	}

	task, found := lt.M[at.Id]
	if found {
		if task.GetClock() == at.Clock && task.GetAction() == at.Action {
			lt.Unlock()
			return
		}
		if at.Action == "start" && task.GetAlive() {
			lt.Unlock()
			log.Printf("W! ignore start action for running task %d", at.Id)
			return
		}
		task.SetAssignment(at.Clock, at.Action)
	} else {
		if at.Action == "kill" {
			lt.Unlock()
			return
		}
		task = &Task{
			Id:     at.Id,
			Clock:  at.Clock,
			Action: at.Action,
		}
		lt.M[at.Id] = task
		if task.doneBefore() {
			task.loadResult()
			lt.Unlock()
			return
		}
	}

	action := task.GetAction()
	var (
		runClock int64
		launch   bool
	)
	if action == "start" {
		runClock, launch = task.beginRun()
	}
	lt.Unlock()

	switch action {
	case "kill":
		task.kill()
	case "start":
		if launch {
			go task.run(runClock)
		}
	default:
		log.Printf("W! unknown action: %s of task %d", action, at.Id)
	}
}

func (lt *LocalTasksT) Clean(assigned map[int64]struct{}) {
	lt.Lock()
	defer lt.Unlock()
	for id, task := range lt.M {
		if _, found := assigned[id]; found {
			continue
		}
		if task.GetAlive() {
			continue
		}
		task.ResetBuff()
		delete(lt.M, id)
	}
}

func (lt *LocalTasksT) StartAccepting() {
	lt.Lock()
	lt.accepting = true
	lt.Unlock()
}

func (lt *LocalTasksT) StopAll(timeout time.Duration) {
	lt.Lock()
	lt.accepting = false
	tasks := make([]*Task, 0, len(lt.M))
	for _, task := range lt.M {
		if task.GetAlive() {
			tasks = append(tasks, task)
		}
	}
	lt.Unlock()

	doneChannels := make([]<-chan struct{}, 0, len(tasks))
	for _, task := range tasks {
		doneChannels = append(doneChannels, task.stop())
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for i, done := range doneChannels {
		select {
		case <-done:
		case <-deadline.C:
			log.Printf("W! timed out waiting for %d ibex task(s) during shutdown", len(doneChannels)-i)
			return
		}
	}
}
