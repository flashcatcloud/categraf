//go:build windows
// +build windows

package main

import (
	"log"

	"flashcat.cloud/categraf/agent"
	"github.com/chai2010/winsvc"
)

func runAgent(ag *agent.Agent) {
	if !winsvc.IsAnInteractiveSession() {
		if err := winsvc.RunAsService(*flagWinSvcName, ag.Start, ag.Stop, false); err != nil {
			log.Fatalln("F! failed to run windows service:", err)
		}
		return
	}

	ag.Start()
	handleSignal(ag)
}
