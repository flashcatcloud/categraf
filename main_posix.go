//go:build linux

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/pprof"
)

func runAgent(ag *agent.Agent) {
	initLog(config.Config.Log.FileName)
	if os.Getpid() == 1 {
		log.Println("W! categraf is running as PID 1; use the official image with tini, docker run --init, or an equivalent init process to reap orphaned children")
	}
	ag.Start()
	go profile()
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
