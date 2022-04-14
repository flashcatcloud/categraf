package writer

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"flashcat.cloud/categraf/config"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
)

type WriterType struct {
	Opts   config.WriterOption
	Client api.Client
}

func (w WriterType) Write(items []*prompb.TimeSeries) {
	if len(items) == 0 {
		return
	}

	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		log.Println("W! marshal prom data to proto got error:", err, "data:", items)
		return
	}

	if err := w.Post(snappy.Encode(nil, data)); err != nil {
		log.Println("W! post to", w.Opts.Url, "got error:", err)
		log.Println("W! example timeseries:", items[0].String())
	}
}

func (w WriterType) Post(req []byte) error {
	httpReq, err := http.NewRequest("POST", w.Opts.Url, bytes.NewReader(req))
	if err != nil {
		log.Println("W! create remote write request got error:", err)
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "categraf")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if w.Opts.BasicAuthUser != "" {
		httpReq.SetBasicAuth(w.Opts.BasicAuthUser, w.Opts.BasicAuthPass)
	}

	resp, body, err := w.Client.Do(context.Background(), httpReq)
	if err != nil {
		log.Println("W! push data with remote write request got error:", err, "response body:", string(body))
		return err
	}

	if resp.StatusCode >= 400 {
		err = fmt.Errorf("push data with remote write request got status code: %v, response body: %s", resp.StatusCode, string(body))
		return err
	}

	return nil
}

var Writers = make(map[string]WriterType)

func Init(opts []config.WriterOption) error {
	for i := 0; i < len(opts); i++ {
		cli, err := api.NewClient(api.Config{
			Address: opts[i].Url,
			RoundTripper: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(opts[i].DialTimeout) * time.Millisecond,
					KeepAlive: time.Duration(opts[i].KeepAlive) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(opts[i].Timeout) * time.Millisecond,
				TLSHandshakeTimeout:   time.Duration(opts[i].TLSHandshakeTimeout) * time.Millisecond,
				ExpectContinueTimeout: time.Duration(opts[i].ExpectContinueTimeout) * time.Millisecond,
				MaxConnsPerHost:       opts[i].MaxConnsPerHost,
				MaxIdleConns:          opts[i].MaxIdleConns,
				MaxIdleConnsPerHost:   opts[i].MaxIdleConnsPerHost,
				IdleConnTimeout:       time.Duration(opts[i].IdleConnTimeout) * time.Millisecond,
			},
		})

		if err != nil {
			return err
		}

		writer := WriterType{
			Opts:   opts[i],
			Client: cli,
		}

		Writers[opts[i].Url] = writer
	}

	return nil
}
