package api

import (
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

const agentHostnameLabelKey = "agent_hostname"

func readerGzipBody(contentEncoding string, request *http.Request) (bytes []byte, err error) {
	if contentEncoding == "gzip" {
		var (
			r *gzip.Reader
		)
		r, err = gzip.NewReader(request.Body)
		if err != nil {
			return nil, err
		}

		defer r.Close()
		bytes, err = ioutil.ReadAll(r)
	} else {
		defer request.Body.Close()
		bytes, err = ioutil.ReadAll(request.Body)
	}
	if err != nil || len(bytes) == 0 {
		return nil, errors.New("request parameter error")
	}

	return bytes, nil
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}
