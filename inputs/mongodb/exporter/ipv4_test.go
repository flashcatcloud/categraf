package exporter

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flashcat.cloud/categraf/internal/nettest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/ocsp"
)

func TestMongoDBIPv4TLSOCSP(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		for _, status := range []int{ocsp.Good, ocsp.Revoked, -1} {
			t.Run(fmt.Sprintf("env=%s/ocsp=%d", value, status), func(t *testing.T) {
				t.Setenv("PREFER_IPV4_STACK", value)
				t.Setenv("NO_PROXY", "*")
				t.Setenv("no_proxy", "*")
				// The same test name is used for both service ports. OCSP's HTTP
				// request counter additionally proves the independent HTTP path ran.
				counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
				var requests atomic.Int32
				var response []byte
				responder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if status < 0 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.Header().Set("Content-Type", "application/ocsp-response")
					_, _ = w.Write(response)
				}))
				defer responder.Close()
				_, responderPort, _ := net.SplitHostPort(responder.Listener.Addr().String())
				serverTLS, caFile, ocspResponse := mongoCertificate(t, "http://"+net.JoinHostPort("example.com.", responderPort), status)
				response = ocspResponse
				address := nettest.TCPServer(t, serverTLS, serveMongo)
				_, port, _ := net.SplitHostPort(address)
				dsn := "mongodb://" + net.JoinHostPort("example.com.", port) + "/?directConnection=true&tls=true&tlsCAFile=" + caFile + "&serverSelectionTimeoutMS=350&connectTimeoutMS=300"
				if status == ocsp.Good {
					dsn += "&maxIdleTimeMS=10"
				}
				for rebuilt := 0; rebuilt < 2; rebuilt++ {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					client, httpClient, err := connect(ctx, dsn, "", "", true)
					if status == ocsp.Revoked {
						if err == nil || !strings.Contains(err.Error(), "revoked") {
							t.Errorf("expected revoked certificate error, got %v", err)
						}
					} else {
						if err != nil {
							cancel()
							t.Fatal(err)
						}
						// Expire an idle pool entry to force another TLS handshake without
						// marking the server unhealthy. The topology OCSP cache must be reused.
						before := requests.Load()
						beforeA, _ := counts()
						if status == ocsp.Good {
							time.Sleep(20 * time.Millisecond)
						}
						if err := client.Ping(ctx, nil); err != nil {
							t.Error(err)
						}
						if requests.Load() != before {
							t.Error("unexpected OCSP request on cache/reuse path")
						}
						if status == ocsp.Good {
							afterA, _ := counts()
							if afterA <= beforeA {
								t.Error("OCSP cache test did not reconnect the database socket")
							}
						}
					}
					if client != nil {
						_ = client.Disconnect(context.Background())
					}
					if httpClient != nil {
						httpClient.CloseIdleConnections()
					}
					cancel()
				}
				a, aaaa := counts()
				if requests.Load() == 0 || a == 0 || value == "true" && aaaa != 0 || value == "false" && aaaa == 0 {
					t.Fatalf("OCSP requests=%d A=%d AAAA=%d", requests.Load(), a, aaaa)
				}
			})
		}
	}
}

func TestMongoDBPolicyCapturesBothClients(t *testing.T) {
	t.Setenv("PREFER_IPV4_STACK", "true")
	counts := nettest.ObserveDNS(t, "example.com.", net.IPv4(127, 0, 0, 1), nil)
	opts := options.Client()
	source := opts.HTTPClient
	owned, err := configureIPv4(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.CloseIdleConnections()
	if owned == source || owned.Transport == source.Transport {
		t.Fatal("SDK shared HTTP client was not cloned")
	}
	t.Setenv("PREFER_IPV4_STACK", "false")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := opts.Dialer.DialContext(ctx, "tcp", net.JoinHostPort("example.com.", port))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	response, err := owned.Get("http://" + net.JoinHostPort("example.com.", port))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	a, aaaa := counts()
	if a < 2 || aaaa != 0 {
		t.Fatalf("A=%d AAAA=%d", a, aaaa)
	}
}

func mongoCertificate(t *testing.T, endpoint string, status int) (*tls.Config, string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "IPv4 fixture CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"example.com"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, OCSPServer: []string{endpoint}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if status < 0 {
		status = ocsp.Good
	}
	response, err := ocsp.CreateResponse(ca, ca, ocsp.Response{Status: status, SerialNumber: leaf.SerialNumber, ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour), RevokedAt: now.Add(-time.Minute)}, key)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der, caDER}, PrivateKey: key}}}, caFile, response
}

// MongoDB's real client performs hello, monitoring, ping and endSessions over
// OP_QUERY/OP_MSG; the fixture responds using the matching wire protocol.
func serveMongo(conn net.Conn) {
	doc, _ := bson.Marshal(bson.D{{Key: "ok", Value: 1}, {Key: "isWritablePrimary", Value: true}, {Key: "ismaster", Value: true}, {Key: "minWireVersion", Value: 0}, {Key: "maxWireVersion", Value: 17}, {Key: "maxBsonObjectSize", Value: 16777216}, {Key: "maxMessageSizeBytes", Value: 48000000}, {Key: "maxWriteBatchSize", Value: 100000}})
	for {
		header := make([]byte, 16)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		size := int(binary.LittleEndian.Uint32(header)) - 16
		if size < 0 || size > 1<<20 {
			return
		}
		if _, err := io.CopyN(io.Discard, conn, int64(size)); err != nil {
			return
		}
		opcode := binary.LittleEndian.Uint32(header[12:])
		var body []byte
		var responseOp uint32
		switch opcode {
		case 2004:
			body = append(make([]byte, 16), 4, 0, 0, 0)
			binary.LittleEndian.PutUint32(body[16:], 1)
			body = append(body, doc...)
			responseOp = 1
		case 2013:
			body = append(make([]byte, 5), doc...)
			responseOp = 2013
		default:
			return
		}
		response := make([]byte, 16)
		binary.LittleEndian.PutUint32(response, uint32(16+len(body)))
		binary.LittleEndian.PutUint32(response[4:], 1)
		copy(response[8:12], header[4:8])
		binary.LittleEndian.PutUint32(response[12:], responseOp)
		if _, err := conn.Write(append(response, body...)); err != nil {
			return
		}
	}
}

func TestMongoDBOpaqueHTTPTransportBoundary(t *testing.T) {
	source := &http.Client{Transport: opaqueTransport{}}
	opts := options.Client().SetHTTPClient(source)
	t.Setenv("PREFER_IPV4_STACK", "false")
	if owned, err := configureIPv4(opts); err != nil || owned != nil || opts.HTTPClient != source || opts.Dialer != nil {
		t.Fatal("disabled policy changed custom options")
	}
	t.Setenv("PREFER_IPV4_STACK", "true")
	if _, err := configureIPv4(opts); err == nil || opts.HTTPClient != source || opts.Dialer != nil {
		t.Fatal("opaque transport was silently accepted or options were partially changed")
	}
	previous := http.DefaultTransport
	http.DefaultTransport = opaqueTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	opts = options.Client().SetHTTPClient(&http.Client{})
	if _, err := configureIPv4(opts); err == nil {
		t.Fatal("opaque default transport was silently accepted")
	}
}

type opaqueTransport struct{}

func (opaqueTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("fixture transport must not be used")
}
