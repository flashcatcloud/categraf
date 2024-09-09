package config

import (
	"io"
	"net/http"
	"strings"
	"time"

	"flashcat.cloud/categraf/pkg/tls"
)

type HTTPCommonConfig struct {
	Method   string `toml:"method"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Body     string `toml:"body"`
	body     io.Reader

	Headers map[string]string `toml:"headers"`

	Timeout           Duration `toml:"timeout"`
	FollowRedirects   *bool    `toml:"follow_redirects"`
	DisableKeepAlives *bool    `toml:"disable_keepalives"`

	tls.ClientConfig
	HTTPProxy
}

func (hcc *HTTPCommonConfig) SetHeaders(req *http.Request) {
	if len(hcc.Username) != 0 && len(hcc.Password) != 0 {
		req.SetBasicAuth(hcc.Username, hcc.Password)
	}
	for k, v := range hcc.Headers {
		req.Header.Set(k, v)
		if k == "Host" {
			req.Host = v
		}
	}
}

func (hcc *HTTPCommonConfig) InitHTTPClientConfig() {
	if len(hcc.Method) == 0 {
		hcc.Method = "GET"
	}

	if hcc.Timeout == 0 {
		hcc.Timeout = Duration(3 * time.Second)
	}
	if len(hcc.Body) != 0 {
		hcc.body = strings.NewReader(hcc.Body)
	}

	if hcc.DisableKeepAlives == nil {
		hcc.DisableKeepAlives = new(bool)
		*hcc.DisableKeepAlives = true
	}
	if hcc.FollowRedirects == nil {
		hcc.FollowRedirects = new(bool)
		*hcc.FollowRedirects = false
	}

}

func (hcc *HTTPCommonConfig) GetBody() io.Reader {
	return hcc.body
}
