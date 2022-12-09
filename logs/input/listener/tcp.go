//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/logs/pipeline"
	"flashcat.cloud/categraf/logs/restart"
)

// A TCPListener listens and accepts TCP connections and delegates the read operations to a tailer.
type TCPListener struct {
	pipelineProvider pipeline.Provider
	source           *logsconfig.LogSource
	idleTimeout      time.Duration
	frameSize        int
	listener         net.Listener
	tailers          []*Tailer
	mu               sync.Mutex
	stop             chan struct{}
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pipelineProvider pipeline.Provider, source *logsconfig.LogSource, frameSize int) *TCPListener {
	var idleTimeout time.Duration
	if source.Config.IdleTimeout != "" {
		var err error
		idleTimeout, err = time.ParseDuration(source.Config.IdleTimeout)
		if err != nil {
			log.Printf("Error parsing log's idle_timeout as a duration: %s\n", err)
			idleTimeout = 0
		}
	}

	return &TCPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		idleTimeout:      idleTimeout,
		frameSize:        frameSize,
		tailers:          []*Tailer{},
		stop:             make(chan struct{}, 1),
	}
}

// Start starts the listener to accepts new incoming connections.
func (l *TCPListener) Start() {
	log.Printf("Starting TCP forwarder on port %d, with read buffer size: %d\n", l.source.Config.Port, l.frameSize)
	err := l.startListener()
	if err != nil {
		log.Printf("Can't start TCP forwarder on port %d: %v\n", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	go l.run()
}

// Stop stops the listener from accepting new connections and all the activer tailers.
func (l *TCPListener) Stop() {
	log.Printf("Stopping TCP forwarder on port %d\n", l.source.Config.Port)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stop <- struct{}{}
	l.listener.Close()
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
	}
	stopper.Stop()
}

// run accepts new TCP connections and create a dedicated tailer for each.
func (l *TCPListener) run() {
	defer l.listener.Close()
	for {
		select {
		case <-l.stop:
			// stop accepting new connections.
			return
		default:
			conn, err := l.listener.Accept()
			switch {
			case err != nil && isClosedConnError(err):
				return
			case err != nil:
				// an error occurred, restart the listener.
				log.Printf("Can't listen on port %d, restarting a listener: %v\n", l.source.Config.Port, err)
				l.listener.Close()
				err := l.startListener()
				if err != nil {
					log.Printf("Can't restart listener on port %d: %v\n", l.source.Config.Port, err)
					l.source.Status.Error(err)
					return
				}
				l.source.Status.Success()
				continue
			default:
				l.startTailer(conn)
				l.source.Status.Success()
			}
		}
	}
}

// startListener starts a new listener, returns an error if it failed.
func (l *TCPListener) startListener() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return err
	}
	l.listener = listener
	return nil
}

// read reads data from connection, returns an error if it failed and stop the tailer.
func (l *TCPListener) read(tailer *Tailer) ([]byte, error) {
	if l.idleTimeout > 0 {
		tailer.conn.SetReadDeadline(time.Now().Add(l.idleTimeout)) //nolint:errcheck
	}
	frame := make([]byte, l.frameSize)
	n, err := tailer.conn.Read(frame)
	if err != nil {
		l.source.Status.Error(err)
		go l.stopTailer(tailer)
		return nil, err
	}
	return frame[:n], nil
}

// startTailer creates and starts a new tailer that reads from the connection.
func (l *TCPListener) startTailer(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tailer := NewTailer(l.source, conn, l.pipelineProvider.NextPipelineChan(), l.read)
	l.tailers = append(l.tailers, tailer)
	tailer.Start()
}

// stopTailer stops the tailer.
func (l *TCPListener) stopTailer(tailer *Tailer) {
	tailer.Stop()
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, t := range l.tailers {
		if t == tailer {
			l.tailers = append(l.tailers[:i], l.tailers[i+1:]...)
			break
		}
	}
}
