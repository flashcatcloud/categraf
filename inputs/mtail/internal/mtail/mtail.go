// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package mtail

import (
	"context"
	"log"
	"net"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"flashcat.cloud/categraf/inputs/mtail/internal/exporter"
	"flashcat.cloud/categraf/inputs/mtail/internal/logline"
	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime"
	"flashcat.cloud/categraf/inputs/mtail/internal/tailer"
)

// Server contains the state of the main mtail program.
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup // wait for main processes to shutdown

	store *metrics.Store // Metrics storage

	tOpts []tailer.Option    // options for constructing `t`
	t     *tailer.Tailer     // t manages log patterns and log streams, which sends lines to the VMs
	rOpts []runtime.Option   // options for constructing `r`
	r     *runtime.Runtime   // r loads programs and manages the VM lifecycle
	eOpts []exporter.Option  // options for constructing `e`
	e     *exporter.Exporter // e manages the export of metrics from the store

	lines chan *logline.LogLine // primary communication channel, owned by Tailer.

	reg *prometheus.Registry

	listener net.Listener // Configured with bind address.

	buildInfo BuildInfo // go build information

	programPath string // path to programs to load
	oneShot     bool   // if set, mtail reads log files from the beginning, once, then exits
	compileOnly bool   // if set, mtail compiles programs then exit
}

func (m *Server) GetRegistry() *prometheus.Registry {
	return m.reg
}

// We can only copy the build info once to the version library.  Protects tests from data races.
var buildInfoOnce sync.Once

// initRuntime constructs a new runtime and performs the initial load of program files in the program directory.
func (m *Server) initRuntime() (err error) {
	m.r, err = runtime.New(m.lines, &m.wg, m.programPath, m.store, m.rOpts...)
	return
}

// initExporter sets up an Exporter for this Server.
func (m *Server) initExporter() (err error) {
	m.e, err = exporter.New(m.ctx, m.store, m.eOpts...)
	if err != nil {
		return err
	}
	m.reg.MustRegister(m.e)
	return nil
}

// initTailer sets up and starts a Tailer for this Server.
func (m *Server) initTailer() (err error) {
	m.t, err = tailer.New(m.ctx, &m.wg, m.lines, m.tOpts...)
	return
}

// New creates a Server from the supplied Options.  The Server is started by
// the time New returns, it watches the LogPatterns for files, starts tailing
// their changes and sends any new lines found to the virtual machines loaded
// from ProgramPath. If OneShot mode is enabled, it will exit after reading
// each log file from start to finish.
// TODO(jaq): this doesn't need to be a constructor anymore, it could start and
// block until quit, once TestServer.PollWatched is addressed.
func New(ctx context.Context, store *metrics.Store, options ...Option) (*Server, error) {
	m := &Server{
		store: store,
		lines: make(chan *logline.LogLine),
		// Using a non-pedantic registry means we can be looser with metrics that
		// are not fully specified at startup.
		reg: prometheus.NewRegistry(),
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.rOpts = append(m.rOpts, runtime.PrometheusRegisterer(m.reg))
	if err := m.SetOption(options...); err != nil {
		return nil, err
	}
	if err := m.initExporter(); err != nil {
		return nil, err
	}
	//nolint:contextcheck // TODO
	if err := m.initRuntime(); err != nil {
		return nil, err
	}
	if err := m.initTailer(); err != nil {
		return nil, err
	}
	return m, nil
}

// SetOption takes one or more option functions and applies them in order to MtailServer.
func (m *Server) SetOption(options ...Option) error {
	for _, option := range options {
		if err := option.apply(m); err != nil {
			return err
		}
	}
	return nil
}

// Run awaits mtail's shutdown.
// TODO(jaq): remove this once the test server is able to trigger polls on the components.
func (m *Server) Run() error {
	m.wg.Wait()
	m.cancel()
	if m.compileOnly {
		log.Println("compile-only is set, exiting")
		return nil
	}
	return nil
}
