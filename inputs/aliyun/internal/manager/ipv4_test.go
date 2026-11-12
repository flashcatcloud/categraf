package manager

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	cms "github.com/alibabacloud-go/cms-20190101/v8/client"
	cmsv2 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

func TestCMSPrivatePoolsAndClose(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	var requests atomic.Int32
	closed := make(chan struct{}, 16)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "fixture-key") && r.Form.Get("AccessKeyId") != "fixture-key" {
			t.Errorf("missing signing credentials: headers=%v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Success":true,"Code":200,"TotalCount":"0","Resources":{"Resource":[]},"Data":{"Records":[]},"RequestId":"fixture"}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	endpoint := strings.TrimPrefix(strings.Replace(server.URL, "127.0.0.1", "example.com.", 1), "http://")
	for rebuild := 0; rebuild < 2; rebuild++ {
		t.Setenv("PREFER_IPV4_STACK", "true")
		m, err := New(NewCmsClient("fixture-key", "fixture-secret", "fixture-region", endpoint), NewCmsV2Client("fixture-key", "fixture-secret", "fixture-region", endpoint))
		if err != nil {
			t.Fatal(err)
		}
		if m.cms.HTTPClientPool == nil || m.cmsv2.HTTPClientPool == nil || m.cms.HTTPClientPool == m.cmsv2.HTTPClientPool {
			t.Fatal("SDK clients share a pool")
		}
		m.cms.Protocol = tea.String("http")
		m.cmsv2.Protocol = tea.String("http")
		t.Setenv("PREFER_IPV4_STACK", "false")
		for i := 0; i < 2; i++ {
			if _, err = m.cms.DescribeMetricMetaList(&cms.DescribeMetricMetaListRequest{}); err != nil {
				t.Fatal(err)
			}
			if _, err = m.cmsv2.BatchGet(&cmsv2.BatchGetRequest{}); err != nil {
				t.Fatal(err)
			}
		}
		m.Close()
		m.Close()
		for i := 0; i < 2; i++ {
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("SDK transport pool remained open")
			}
		}
	}
	a, aaaa := counts()
	if requests.Load() != 8 || a == 0 || aaaa != 0 {
		t.Fatalf("requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
	}
}

func TestCMSSOCKSFirstHopPolicy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	queries := nettest.ObserveDNSQueries(t, "example.com.", net.IPv4(127, 0, 0, 1), net.IPv6loopback)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "remote-only.invalid:8080" {
			t.Errorf("HTTP Host changed: %s", r.Host)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Success":true,"TotalCount":"0","Resources":{"Resource":[]}}`)
	}))
	defer backend.Close()
	proxyAddress := nettest.TCPServer(t, nil, func(downstream net.Conn) {
		var header [4]byte
		if _, err := io.ReadFull(downstream, header[:2]); err != nil {
			return
		}
		methods := make([]byte, int(header[1]))
		if _, err := io.ReadFull(downstream, methods); err != nil {
			return
		}
		_, _ = downstream.Write([]byte{5, 0})
		if _, err := io.ReadFull(downstream, header[:]); err != nil {
			return
		}
		if header[0] != 5 || header[1] != 1 || header[3] != 3 {
			t.Errorf("SOCKS header=%v", header)
			return
		}
		var length [1]byte
		if _, err := io.ReadFull(downstream, length[:]); err != nil {
			return
		}
		name := make([]byte, int(length[0])+2)
		if _, err := io.ReadFull(downstream, name); err != nil {
			return
		}
		if string(name[:len(name)-2]) != "remote-only.invalid" {
			t.Errorf("SOCKS target changed: %q", name)
		}
		upstream, err := net.Dial("tcp", strings.TrimPrefix(backend.URL, "http://"))
		if err != nil {
			t.Error(err)
			return
		}
		defer upstream.Close()
		_, _ = downstream.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, downstream); upstream.Close(); close(done) }()
		_, _ = io.Copy(downstream, upstream)
		downstream.Close()
		<-done
	})
	_, port, _ := net.SplitHostPort(proxyAddress)
	m, err := New(NewCmsClient("fixture", "fixture", "fixture", "remote-only.invalid:8080"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.cms.Protocol = tea.String("http")
	m.cms.Socks5Proxy = tea.String("example.com.:" + port)
	m.cms.Socks5NetWork = tea.String("tcp")
	t.Setenv("PREFER_IPV4_STACK", "false")
	if _, err := m.cms.DescribeMetricMetaList(&cms.DescribeMetricMetaListRequest{}); err != nil {
		t.Fatal(err)
	}
	a, aaaa := queries("example.com.")
	if a == 0 || aaaa != 0 {
		t.Fatalf("SOCKS first hop A=%d AAAA=%d", a, aaaa)
	}
	a, aaaa = queries("remote-only.invalid.")
	if a != 0 || aaaa != 0 {
		t.Fatalf("SOCKS target resolved locally: A=%d AAAA=%d", a, aaaa)
	}
}
