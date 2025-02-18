package api

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

const agentHostnameLabelKey = "agent_hostname"

func readerGzipBody(contentEncoding string, request *http.Request) (bytes []byte, err error) {
	switch contentEncoding {
	case "gzip":
		var r *gzip.Reader
		r, err = gzip.NewReader(request.Body)
		if err != nil {
			return nil, err
		}
		defer r.Close()

		bytes, err = io.ReadAll(r)
	case "snappy":
		defer request.Body.Close()
		var compressed []byte
		compressed, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}

		bytes, err = snappy.Decode(nil, compressed)
	default:
		defer request.Body.Close()
		bytes, err = io.ReadAll(request.Body)
	}

	if err != nil || len(bytes) == 0 {
		return nil, errors.New("request parameter error")
	}

	return bytes, nil
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	compressed, err := io.ReadAll(r)
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

type Option func(c *gin.Context) bool

func QueryBoolWithValues(k string, vs ...string) Option {
	return func(c *gin.Context) bool {
		v := c.Query(k)
		for _, vv := range vs {
			if vv == v {
				return true
			}
		}
		return v == "true"
	}
}
