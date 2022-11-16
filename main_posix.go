//go:build !windows
// +build !windows

package main

import (
	"flashcat.cloud/categraf/agent"
)

func runAgent(ag *agent.Agent) {
	ag.Start()
	handleSignal(ag)
}

func doOSsvc() {

}
