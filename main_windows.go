//go:build windows
// +build windows

package main

import (
	"log"
	"os"

	"flashcat.cloud/categraf/agent"
	"github.com/chai2010/winsvc"
)

func runAgent(ag *agent.Agent) {
	if !winsvc.IsAnInteractiveSession() {
		f, err := os.OpenFile("categraf.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalln("F! failed to open log file: categraf.log")
		}

		defer f.Close()

		log.SetOutput(f)

		if err := winsvc.RunAsService(*flagWinSvcName, ag.Start, ag.Stop, false); err != nil {
			log.Fatalln("F! failed to run windows service:", err)
		}
		return
	}

	ag.Start()
	handleSignal(ag)
}
