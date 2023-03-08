package pprof

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
)

var (
	pprof uint32
	addr  string
)

func Go() {

	if !atomic.CompareAndSwapUint32(&pprof, 0, 1) {
		log.Println("pprofile already started,", addr)
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Println(err)
		return
	}
	addr = fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", listener.Addr().(*net.TCPAddr).Port)
	log.Printf("pprof started at %s", addr)

	err = http.Serve(listener, nil)
	if err != nil {
		log.Println(err)
		return
	}
}
