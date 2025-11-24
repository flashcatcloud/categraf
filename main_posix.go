//go:build linux

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/pprof"
)

func runAgent(ag *agent.Agent) {
	initLog(config.Config.Log.FileName)
	ag.Start()
	go profile()
	go reapDaemon()
	handleSignal(ag)
}

func doOSsvc() {
}

func profile() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGUSR2)
	for {
		sig := <-sc
		switch sig {
		case syscall.SIGUSR2:
			go pprof.Go()
		}
	}
}

type exit struct {
	pid    int
	status int
}

func exitStatus(status unix.WaitStatus) int {
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}

func reap() (exits []exit, err error) {
	var (
		ws  unix.WaitStatus
		rus unix.Rusage
	)
	for {
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, &rus)
		if err != nil {
			if err == unix.ECHILD {
				return exits, nil
			}
			return nil, err
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, exit{
			pid:    pid,
			status: exitStatus(ws),
		})
	}
}
func reapDaemon() {
	if os.Getpid() != 1 {
		return
	}
	unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, unix.SIGCHLD)
	for {
		sig := <-signals
		switch sig {
		case unix.SIGCHLD:
			exits, err := reap()
			if err != nil {
				log.Printf("E! reaping children failed: %v", err)
				continue
			}
			for _, e := range exits {
				log.Printf("I! reaped pid: %d, status: %d", e.pid, e.status)
			}
		}
	}
}
