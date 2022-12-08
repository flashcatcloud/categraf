//go:build !no_ibex

package client

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"reflect"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/ibex/types"
	"github.com/toolkits/pkg/net/gobrpc"
	"github.com/ugorji/go/codec"
)

var cli *gobrpc.RPCClient

func getCli() *gobrpc.RPCClient {
	if cli != nil {
		return cli
	}

	// detect the fastest server
	var (
		address  string
		client   *rpc.Client
		duration int64 = 999999999999
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

	cli = gobrpc.NewRPCClient(address, client, 5*time.Second)
	return cli
}

// GetCli 探测所有server端的延迟，自动选择最快的
func GetCli() *gobrpc.RPCClient {
	for {
		c := getCli()
		if c != nil {
			return c
		}

		time.Sleep(time.Second * 10)
	}
}

// CloseCli 关闭客户端连接
func CloseCli() {
	if cli != nil {
		cli.Close()
		cli = nil
	}
}

// Meta 从Server端获取任务元信息
func Meta(id int64) (script string, args string, account string, err error) {
	var resp types.TaskMetaResponse
	err = GetCli().Call("Server.GetTaskMeta", id, &resp)
	if err != nil {
		log.Println("E! rpc call Server.GetTaskMeta:", err)
		CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E! rpc call Server.GetTaskMeta:", resp.Message)
		err = fmt.Errorf(resp.Message)
		return
	}

	script = resp.Script
	args = resp.Args
	account = resp.Account
	return
}
