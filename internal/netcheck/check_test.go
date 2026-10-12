package netcheck

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDNSProtocolsAndQueryValidation(t *testing.T) {
	for _, transport := range []string{"udp", "tcp"} {
		for _, mismatch := range []bool{false, true} {
			t.Run(transport+map[bool]string{false: "/answer", true: "/wrong-id"}[mismatch], func(t *testing.T) {
				dial := func(ctx context.Context, network, address string) (net.Conn, error) {
					if network != transport+"4" || address != "192.0.2.53:53" {
						t.Error("unexpected DNS destination", network, address)
					}
					client, server := net.Pipe()
					go func() {
						defer server.Close()
						server.SetDeadline(time.Now().Add(time.Second))
						var b []byte
						if transport == "tcp" {
							var size [2]byte
							if _, err := io.ReadFull(server, size[:]); err != nil {
								return
							}
							b = make([]byte, binary.BigEndian.Uint16(size[:]))
							if _, err := io.ReadFull(server, b); err != nil {
								return
							}
						} else {
							b = make([]byte, 1024)
							n, err := server.Read(b)
							if err != nil {
								return
							}
							b = b[:n]
						}
						var query dnsmessage.Message
						if err := query.Unpack(b); err != nil {
							return
						}
						response := dnsmessage.Message{
							Header:    dnsmessage.Header{ID: query.ID, Response: true},
							Questions: query.Questions,
							Answers: []dnsmessage.Resource{{
								Header: dnsmessage.ResourceHeader{Name: query.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 30},
								Body:   &dnsmessage.AResource{A: [4]byte{192, 0, 2, 1}},
							}},
						}
						if mismatch {
							response.ID++
						}
						wire, err := response.Pack()
						if err != nil {
							return
						}
						if transport == "tcp" {
							wire = append([]byte{byte(len(wire) >> 8), byte(len(wire))}, wire...)
						}
						server.Write(wire)
					}()
					return client, nil
				}
				got := QueryDNS(context.Background(), dial, "192.0.2.53", "example.com", transport, dnsmessage.TypeA)
				if mismatch {
					if !strings.Contains(got.Error, "does not match") {
						t.Fatal(got)
					}
				} else if got.Error != "" || len(got.Answers) != 1 || got.Answers[0] != "192.0.2.1" {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestHTTPSDistinguishesTCPAndTLSFailures(t *testing.T) {
	refused := func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("test TCP refusal") }
	got := CheckHTTPS(context.Background(), refused, "example.com", "192.0.2.1", "http1")
	if got.ErrorStage != "tcp" || got.TCPConnected || !strings.Contains(got.Error, "test TCP refusal") {
		t.Fatal(got)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	got = CheckHTTPS(context.Background(), dial, "example.com", "192.0.2.1", "http2-if-supported")
	if got.ErrorStage != "tls" || !got.TCPConnected || got.TLSVerified {
		t.Fatal("TLS verification must not be skipped", got)
	}
}

func TestHTTPSProtocolsAndAuthorizationStatus(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent to test front door")
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	for _, tc := range []struct{ mode, protocol string }{{"http1", "HTTP/1.1"}, {"http2-if-supported", "HTTP/2.0"}} {
		got := checkHTTPS(context.Background(), dial, "127.0.0.1", "192.0.2.1", tc.mode, roots)
		if !got.TCPConnected || !got.TLSVerified || got.HTTPStatus != 403 || got.HTTPProtocol != tc.protocol || got.Error != "" {
			t.Fatal("authorization status confused with transport failure", got)
		}
	}
}

func TestLocalHTTPPaddingCredentialsAndRedirects(t *testing.T) {
	for _, padding := range []int{0, 8192} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" || r.URL.Path != "/" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				t.Error("unexpected API operation or credentials")
			}
			if len(r.Header.Get("X-Tunnel-Studio-Probe")) != padding {
				t.Error("wrong probe size")
			}
			// A rejected unauthenticated request still proves an HTTP response.
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("body-not-stored"))
		}))
		dial := func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}
		got := CheckLocalHTTP(context.Background(), dial, "192.0.2.1:8317", padding)
		server.Close()
		if !got.TCPConnected || got.HTTPStatus != 401 || got.Error != "" || got.BodySampleBytes != 15 {
			t.Fatal(got)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://redirect-must-not-be-contacted.invalid/", 302)
	}))
	defer server.Close()
	calls := 0
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		calls++
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	got := CheckLocalHTTP(context.Background(), dial, "192.0.2.1:8317", 0)
	if got.HTTPStatus != 302 || calls != 1 {
		t.Fatal("redirect followed", calls, got)
	}
}

func TestParentCancellationStopsChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	started := time.Now()
	for _, got := range Run(ctx, "192.0.2.53", dial) {
		if len(got.HTTPS) != 0 {
			t.Fatal("HTTPS after DNS failure", got)
		}
		for _, dns := range got.DNS {
			if dns.Error == "" {
				t.Fatal("missing cancellation error")
			}
		}
	}
	if time.Since(started) > time.Second {
		t.Fatal("checks did not honor cancellation")
	}
}
