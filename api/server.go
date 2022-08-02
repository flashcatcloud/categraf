package api

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ServerOption is an HTTP server option.
type ServerOption func(*Server)

// Network with server network.
func Network(network string) ServerOption {
	return func(s *Server) {
		s.network = network
	}
}

// Address with server address.
func Address(addr string) ServerOption {
	return func(s *Server) {
		s.address = addr
	}
}

// Timeout with server timeout.
func Timeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.timeout = timeout
	}
}

// Listener with server lis
func Listener(lis net.Listener) ServerOption {
	return func(s *Server) {
		s.lis = lis
	}
}

type Server struct {
	*http.Server
	engine  *gin.Engine
	lis     net.Listener
	network string
	address string
	timeout time.Duration
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		network: "tcp",
		address: ":0",
		timeout: 1 * time.Second,
	}
	for _, o := range opts {
		o(srv)
	}

	srv.engine = gin.New()
	srv.engine.Use(gin.Recovery(), gin.Logger())
	srv.Server = &http.Server{
		Handler: srv.engine,
	}

	newRouter(srv)
	return srv
}

// ServeHTTP should write reply headers and data to the ResponseWriter and then return.
func (s *Server) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	s.Handler.ServeHTTP(res, req)
}

func (s *Server) listenAndEndpoint() error {
	if s.lis == nil {
		lis, err := net.Listen(s.network, s.address)
		if err != nil {
			return err
		}
		s.lis = lis
	}

	return nil
}

// Start start the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	if err := s.listenAndEndpoint(); err != nil {
		return err
	}
	s.BaseContext = func(net.Listener) context.Context {
		return ctx
	}

	log.Printf("[HTTP] server listening on: %s]\n", s.lis.Addr().String())
	go func() {
		err := s.Serve(s.lis)
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[HTTP] server error: %v", err)
		}
	}()
	return nil
}

// Stop stop the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	log.Println("[HTTP] server stopping")
	return s.Shutdown(ctx)
}
