//go:build !windows

package main

import (
	"log"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

func runAgent(ag *agent.Agent) {
	ag.Start()
	handleSignal(ag)
}

func doOSsvc() {
	if config.Config.Log.Enable {
		log.SetOutput(&lumberjack.Logger{
			Filename:   config.Config.Log.FileName,
			MaxSize:    config.Config.Log.MaxSize,
			MaxAge:     config.Config.Log.MaxAge,
			MaxBackups: config.Config.Log.MaxBackups,
			LocalTime:  config.Config.Log.LocalTime,
			Compress:   config.Config.Log.Compress,
		})
	}
}
