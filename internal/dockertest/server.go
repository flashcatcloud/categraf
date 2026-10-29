// Package dockertest provides a versioned Engine API fixture for SDK regressions.
package dockertest

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func NewUnixServer(t *testing.T, apiVersion string, handler http.Handler) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket fixture")
	}
	// A short directory avoids the Unix socket path limit under macOS t.TempDir.
	dir, err := os.MkdirTemp("", "dockertest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(Handler(t, apiVersion, handler))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return "unix://" + path
}

func NewServer(t *testing.T, apiVersion string, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(Handler(t, apiVersion, handler))
	t.Cleanup(server.Close)
	return server
}

// Handler answers API negotiation and requires the negotiated prefix on all
// other requests. The supplied handler receives the unversioned path.
func Handler(t *testing.T, apiVersion string, handler http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", apiVersion)
		w.Header().Set("OSType", "linux")
		if r.URL.Path == "/_ping" {
			_, _ = io.WriteString(w, "OK")
			return
		}
		path, ok := strings.CutPrefix(r.URL.Path, "/v"+apiVersion)
		if !ok {
			t.Errorf("unexpected API version in request: %s", r.URL.Path)
			http.Error(w, "incorrect API version", http.StatusBadRequest)
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = path
		handler.ServeHTTP(w, request)
	})
}
