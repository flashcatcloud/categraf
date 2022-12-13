package writer

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"

	"flashcat.cloud/categraf/config"
)

type Writer struct {
	Opts   config.WriterOption
	Client api.Client
}

// newWriter creates a new Writer from config.WriterOption
func newWriter(opt config.WriterOption) (Writer, error) {
	cli, err := api.NewClient(api.Config{
		Address: opt.Url,
		RoundTripper: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(opt.DialTimeout) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(opt.Timeout) * time.Millisecond,
			MaxIdleConnsPerHost:   opt.MaxIdleConnsPerHost,
		},
	})

	if err != nil {
		return Writer{}, err
	}

	return Writer{
		Opts:   opt,
		Client: cli,
	}, nil
}

func (w Writer) Write(items []prompb.TimeSeries) {
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

	if err := w.post(snappy.Encode(nil, data)); err != nil {
		log.Println("W! post to", w.Opts.Url, "got error:", err)
		log.Println("W! example timeseries:", items[0].String())
	}
}

func (w Writer) post(req []byte) error {
	httpReq, err := http.NewRequest("POST", w.Opts.Url, bytes.NewReader(req))
	if err != nil {
		log.Println("W! create remote write request got error:", err)
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "categraf")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	for i := 0; i < len(w.Opts.Headers); i += 2 {
		httpReq.Header.Add(w.Opts.Headers[i], w.Opts.Headers[i+1])
		if w.Opts.Headers[i] == "Host" {
			httpReq.Host = w.Opts.Headers[i+1]
		}
	}

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
