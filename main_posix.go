//go:build !windows

package main

import (
	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
)

func runAgent(ag *agent.Agent) {
	initLog(config.Config.Log.FileName)
	ag.Start()
	handleSignal(ag)
}

func doOSsvc() {
}
