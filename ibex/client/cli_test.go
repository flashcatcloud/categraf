//go:build !no_ibex

package client

import (
	"net"
	"net/rpc"
	"sync"
	"testing"
	"time"
)

type blockingRPCService struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingRPCService) Block(_ struct{}, reply *bool) error {
	s.started <- struct{}{}
	<-s.release
	*reply = true
	return nil
}

func TestCloseCliWaitsForConcurrentCalls(t *testing.T) {
	const callers = 8
	serverConn, clientConn := net.Pipe()
	service := &blockingRPCService{
		started: make(chan struct{}, callers),
		release: make(chan struct{}),
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Blocking", service); err != nil {
		t.Fatal(err)
	}
	go server.ServeConn(serverConn)

	cliMu.Lock()
	cli = &rpcClient{client: rpc.NewClient(clientConn), callTimeout: 5 * time.Second}
	cliMu.Unlock()
	t.Cleanup(func() {
		CloseCli()
		_ = serverConn.Close()
	})

	errs := make(chan error, callers)
	var calls sync.WaitGroup
	for i := 0; i < callers; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			var reply bool
			errs <- Call("Blocking.Block", struct{}{}, &reply)
		}()
	}
	for i := 0; i < callers; i++ {
		select {
		case <-service.started:
		case <-time.After(3 * time.Second):
			t.Fatal("RPC calls did not start")
		}
	}

	closed := make(chan struct{})
	go func() {
		CloseCli()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("CloseCli returned while RPC calls were still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(service.release)
	calls.Wait()
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("CloseCli did not finish after RPC calls completed")
	}
}

func TestCloseCliAfterCallTimeout(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	service := &blockingRPCService{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Blocking", service); err != nil {
		t.Fatal(err)
	}
	go server.ServeConn(serverConn)

	cliMu.Lock()
	cli = &rpcClient{client: rpc.NewClient(clientConn), callTimeout: 20 * time.Millisecond}
	cliMu.Unlock()
	t.Cleanup(func() {
		CloseCli()
		_ = serverConn.Close()
	})

	var reply bool
	err := Call("Blocking.Block", struct{}{}, &reply)
	if err == nil || err.Error() != "timeout" {
		t.Fatalf("unexpected call result: %v", err)
	}
	CloseCli()
	close(service.release)
}
