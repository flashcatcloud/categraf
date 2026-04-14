package pprof

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"k8s.io/klog/v2"
)

var (
	pprof uint32
	addr  string
)

func Go() {

	if !atomic.CompareAndSwapUint32(&pprof, 0, 1) {
		klog.InfoS("pprof already started", "address", addr)
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		klog.ErrorS(err, "failed to start pprof listener")
		return
	}
	addr = fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", listener.Addr().(*net.TCPAddr).Port)
	klog.InfoS("pprof started", "address", addr)

	err = http.Serve(listener, nil)
	if err != nil {
		klog.ErrorS(err, "pprof server exited")
		return
	}
}
