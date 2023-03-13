//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/pprof"
)

func runAgent(ag *agent.Agent) {
	initLog(config.Config.Log.FileName)
	ag.Start()
	go profile()
	handleSignal(ag)
}

func doOSsvc() {
}

var (
	pprofStart uint32
)

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
