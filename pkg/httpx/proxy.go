package httpx

import (
	"errors"
	"net/http"
	"net/url"
)

func GetProxyFunc(httpProxy string) func(*http.Request) (*url.URL, error) {
	if httpProxy == "" {
		return http.ProxyFromEnvironment
	}
	proxyURL, err := url.Parse(httpProxy)
	if err != nil {
		return func(_ *http.Request) (*url.URL, error) {
			return nil, errors.New("bad proxy: " + err.Error())
		}
	}
	return func(r *http.Request) (*url.URL, error) {
		return proxyURL, nil
	}
}
