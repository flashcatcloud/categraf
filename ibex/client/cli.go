//go:build !no_ibex

package client

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/rpc"
	"reflect"
	"sync"
	"time"

	"github.com/ugorji/go/codec"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/types"
)

var (
	cliMu sync.RWMutex
	cli   *rpcClient
)

type rpcClient struct {
	client      *rpc.Client
	callTimeout time.Duration
}

func (c *rpcClient) Call(method string, args interface{}, reply interface{}, callTimeout ...time.Duration) error {
	timeout := c.callTimeout
	if len(callTimeout) > 0 {
		timeout = callTimeout[0]
	}
	call := c.client.Go(method, args, reply, make(chan *rpc.Call, 1))
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return fmt.Errorf("timeout")
	case completed := <-call.Done:
		return completed.Error
	}
}

func (c *rpcClient) Close() error {
	return c.client.Close()
}

func connect() *rpcClient {
	// detect the fastest server
	var (
		address  string
		client   *rpc.Client
		duration int64 = math.MaxInt64
	)

	// auto close other slow server
	acm := make(map[string]*rpc.Client)

	l := len(config.Config.Ibex.Servers)
	for i := 0; i < l; i++ {
		addr := config.Config.Ibex.Servers[i]
		begin := time.Now()
		conn, err := net.DialTimeout("tcp", addr, time.Second*5)
		if err != nil {
			log.Printf("W! dial %s fail: %s", addr, err)
			continue
		}

		var bufConn = struct {
			io.Closer
			*bufio.Reader
			*bufio.Writer
		}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

		var mh codec.MsgpackHandle
		mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

		rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufConn, &mh)
		c := rpc.NewClientWithCodec(rpcCodec)

		acm[addr] = c

		var out string
		err = c.Call("Server.Ping", "", &out)
		if err != nil {
			log.Printf("W! ping %s fail: %s", addr, err)
			continue
		}
		use := time.Since(begin).Nanoseconds()

		if use < duration {
			address = addr
			client = c
			duration = use
		}
	}

	if address == "" {
		for _, c := range acm {
			_ = c.Close()
		}
		log.Println("E! no job server found")
		return nil
	}

	log.Printf("I! choose server: %s, duration: %dms", address, duration/1000000)

	for addr, c := range acm {
		if addr == address {
			continue
		}
		c.Close()
	}

	return &rpcClient{client: client, callTimeout: 5 * time.Second}
}

// Call serializes client initialization and keeps CloseCli from closing the
// selected connection while an RPC is in flight. net/rpc.Client itself permits
// concurrent calls, so established connections use a shared read lock.
func Call(method string, args interface{}, reply interface{}, callTimeout ...time.Duration) error {
	for {
		cliMu.RLock()
		c := cli
		if c != nil {
			err := c.Call(method, args, reply, callTimeout...)
			cliMu.RUnlock()
			return err
		}
		cliMu.RUnlock()

		cliMu.Lock()
		if cli == nil {
			cli = connect()
		}
		connected := cli != nil
		cliMu.Unlock()
		if !connected {
			time.Sleep(time.Second * 10)
		}
	}
}

// CloseCli 关闭客户端连接
func CloseCli() {
	cliMu.Lock()
	defer cliMu.Unlock()
	if cli != nil {
		_ = cli.Close()
		cli = nil
	}
}

// Meta 从Server端获取任务元信息
func Meta(id int64) (script string, args string, account string, stdin string, err error) {
	var resp types.TaskMetaResponse
	err = Call("Server.GetTaskMeta", id, &resp)
	if err != nil {
		log.Println("E! rpc call Server.GetTaskMeta:", err)
		CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E! rpc call Server.GetTaskMeta:", resp.Message)
		err = fmt.Errorf("%s", resp.Message)
		return
	}

	script = resp.Script
	args = resp.Args
	account = resp.Account
	stdin = resp.Stdin

	return
}
