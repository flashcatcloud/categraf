//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/pipeline"
	"flashcat.cloud/categraf/logs/restart"
)

// Launcher summons different protocol specific listeners based on configuration
type Launcher struct {
	pipelineProvider pipeline.Provider
	frameSize        int
	tcpSources       chan *logsconfig.LogSource
	udpSources       chan *logsconfig.LogSource
	listeners        []restart.Restartable
	stop             chan struct{}
}

// NewLauncher returns an initialized Launcher
func NewLauncher(sources *logsconfig.LogSources, frameSize int, pipelineProvider pipeline.Provider) *Launcher {
	return &Launcher{
		pipelineProvider: pipelineProvider,
		frameSize:        frameSize,
		tcpSources:       sources.GetAddedForType(logsconfig.TCPType),
		udpSources:       sources.GetAddedForType(logsconfig.UDPType),
		stop:             make(chan struct{}),
	}
}

// Start starts the listener.
func (l *Launcher) Start() {
	go l.run()
}

// run starts new network listeners.
func (l *Launcher) run() {
	for {
		select {
		case source := <-l.tcpSources:
			listener := NewTCPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case source := <-l.udpSources:
			listener := NewUDPListener(l.pipelineProvider, source, l.frameSize)
			listener.Start()
			l.listeners = append(l.listeners, listener)
		case <-l.stop:
			return
		}
	}
}

// Stop stops all listeners
func (l *Launcher) Stop() {
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, l := range l.listeners {
		stopper.Add(l)
	}
	stopper.Stop()
}
