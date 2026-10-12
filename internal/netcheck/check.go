// Package netcheck performs bounded, unauthenticated connectivity checks.
// Callers supply a dialer bound to the tunnel; no default dialer or DNS fallback is used.
package netcheck

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type DialFunc func(context.Context, string, string) (net.Conn, error)

type DNSResult struct {
	Transport string   `json:"transport"`
	Type      string   `json:"type"`
	Answers   []string `json:"answers,omitempty"`
	RCode     string   `json:"rcode,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	Duration  int64    `json:"durationMs"`
	Error     string   `json:"error,omitempty"`
}

type HTTPSResult struct {
	Mode            string `json:"mode"`
	IP              string `json:"ip"`
	TCPConnected    bool   `json:"tcpConnected"`
	TLSVerified     bool   `json:"tlsVerified"`
	TCPMs           int64  `json:"tcpMs"`
	TLSMs           int64  `json:"tlsMs"`
	FirstByteMs     int64  `json:"firstByteMs"`
	DurationMs      int64  `json:"durationMs"`
	HTTPStatus      int    `json:"httpStatus,omitempty"`
	HTTPProtocol    string `json:"httpProtocol,omitempty"`
	TLSProtocol     string `json:"tlsALPN,omitempty"`
	BodySampleBytes int64  `json:"bodySampleBytes"`
	ErrorStage      string `json:"errorStage,omitempty"`
	Error           string `json:"error,omitempty"`
}

type TargetResult struct {
	Host  string        `json:"host"`
	DNS   []DNSResult   `json:"dns"`
	HTTPS []HTTPSResult `json:"https,omitempty"`
	Note  string        `json:"note,omitempty"`
}

// QueryDNS sends exactly one query, keeping UDP and TCP results separate.
func QueryDNS(ctx context.Context, dial DialFunc, server, host, transport string, qtype dnsmessage.Type) (r DNSResult) {
	r.Transport, r.Type = transport, qtype.String()
	started := time.Now()
	defer func() { r.Duration = time.Since(started).Milliseconds() }()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var idBytes [2]byte
	if _, e := rand.Read(idBytes[:]); e != nil {
		r.Error = e.Error()
		return
	}
	name, e := dnsmessage.NewName(host + ".")
	if e != nil {
		r.Error = e.Error()
		return
	}
	question := dnsmessage.Question{Name: name, Type: qtype, Class: dnsmessage.ClassINET}
	m := dnsmessage.Message{Header: dnsmessage.Header{ID: binary.BigEndian.Uint16(idBytes[:]), RecursionDesired: true}, Questions: []dnsmessage.Question{question}}
	wire, e := m.Pack()
	if e != nil {
		r.Error = e.Error()
		return
	}
	c, e := dial(ctx, transport+"4", net.JoinHostPort(server, "53"))
	if e != nil {
		r.Error = e.Error()
		return
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	c.SetDeadline(deadline)
	if transport == "tcp" {
		wire = append([]byte{byte(len(wire) >> 8), byte(len(wire))}, wire...)
	}
	if _, e = io.Copy(c, bytes.NewReader(wire)); e != nil {
		r.Error = e.Error()
		return
	}
	var response []byte
	if transport == "tcp" {
		var size [2]byte
		if _, e = io.ReadFull(c, size[:]); e == nil {
			response = make([]byte, int(binary.BigEndian.Uint16(size[:])))
			_, e = io.ReadFull(c, response)
		}
	} else {
		response = make([]byte, 4096)
		var n int
		n, e = c.Read(response)
		response = response[:n]
	}
	if e != nil {
		r.Error = e.Error()
		return
	}
	var reply dnsmessage.Message
	if e = reply.Unpack(response); e != nil {
		r.Error = e.Error()
		return
	}
	if !reply.Response || reply.ID != m.ID || len(reply.Questions) != 1 || reply.Questions[0] != question {
		r.Error = "DNS response does not match the query"
		return
	}
	r.RCode, r.Truncated = reply.RCode.String(), reply.Truncated
	if reply.RCode != dnsmessage.RCodeSuccess {
		r.Error = "DNS server returned " + reply.RCode.String()
		return
	}
	for _, answer := range reply.Answers {
		switch b := answer.Body.(type) {
		case *dnsmessage.AResource:
			if qtype == dnsmessage.TypeA {
				r.Answers = append(r.Answers, net.IP(b.A[:]).String())
			}
		case *dnsmessage.AAAAResource:
			if qtype == dnsmessage.TypeAAAA {
				r.Answers = append(r.Answers, net.IP(b.AAAA[:]).String())
			}
		}
	}
	return
}

// CheckHTTPS records transport phases independently of HTTP authorization.
// It follows no redirects, sends no cookies/credentials, and stores no response bodies.
func CheckHTTPS(ctx context.Context, dial DialFunc, host, ip, mode string) HTTPSResult {
	return checkHTTPS(ctx, dial, host, ip, mode, nil)
}

func checkHTTPS(ctx context.Context, dial DialFunc, host, ip, mode string, roots *x509.CertPool) (r HTTPSResult) {
	r.Mode, r.IP = mode, ip
	progress := r
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	var mu sync.Mutex
	var tlsStart time.Time
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: mode == "http2-if-supported",
		TLSClientConfig:     &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: roots},
		TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 4 * time.Second, MaxResponseHeaderBytes: 32768,
	}
	transport.DialContext = func(c context.Context, _, _ string) (net.Conn, error) {
		dialCtx, done := context.WithTimeout(c, 4*time.Second)
		defer done()
		t := time.Now()
		conn, err := dial(dialCtx, "tcp4", net.JoinHostPort(ip, "443"))
		mu.Lock()
		progress.TCPMs, progress.TCPConnected = time.Since(t).Milliseconds(), err == nil
		mu.Unlock()
		return conn, err
	}
	if mode == "http1" {
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	defer transport.CloseIdleConnections()
	trace := &httptrace.ClientTrace{
		TLSHandshakeStart: func() { mu.Lock(); tlsStart = time.Now(); mu.Unlock() },
		TLSHandshakeDone: func(s tls.ConnectionState, err error) {
			mu.Lock()
			progress.TLSMs = time.Since(tlsStart).Milliseconds()
			progress.TLSVerified = err == nil
			progress.TLSProtocol = s.NegotiatedProtocol
			mu.Unlock()
		},
		GotFirstResponseByte: func() { mu.Lock(); progress.FirstByteMs = time.Since(started).Milliseconds(); mu.Unlock() },
	}
	req, e := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), "GET", "https://"+host+"/", nil)
	if e != nil {
		r.ErrorStage, r.Error = "request", e.Error()
		return
	}
	req.Header.Set("User-Agent", "Tunnel-Studio-NetworkCheck")
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	mu.Lock()
	r = progress
	mu.Unlock()
	r.DurationMs = time.Since(started).Milliseconds()
	if e != nil {
		switch {
		case !r.TCPConnected:
			r.ErrorStage = "tcp"
		case !r.TLSVerified:
			r.ErrorStage = "tls"
		default:
			r.ErrorStage = "http"
		}
		r.Error = e.Error()
		return
	}
	r.HTTPStatus, r.HTTPProtocol = resp.StatusCode, resp.Proto
	r.BodySampleBytes, e = io.CopyN(io.Discard, resp.Body, 4096)
	resp.Body.Close()
	r.DurationMs = time.Since(started).Milliseconds()
	if e != nil && !errors.Is(e, io.EOF) {
		r.ErrorStage, r.Error = "body", e.Error()
	}
	return
}

// CheckLocalHTTP tests a user-specified plain HTTP front door, never an API
// operation. Padding helps expose upload stalls that a tiny GET would miss.
func CheckLocalHTTP(ctx context.Context, dial DialFunc, address string, padding int) HTTPSResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	result := HTTPSResult{Mode: "local-http", IP: address}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 4 * time.Second,
		MaxResponseHeaderBytes: 32768,
	}
	var mu sync.Mutex
	transport.DialContext = func(c context.Context, _, _ string) (net.Conn, error) {
		dialCtx, done := context.WithTimeout(c, 3*time.Second)
		defer done()
		t := time.Now()
		conn, err := dial(dialCtx, "tcp4", address)
		mu.Lock()
		result.TCPMs, result.TCPConnected = time.Since(t).Milliseconds(), err == nil
		mu.Unlock()
		return conn, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", "http://"+address+"/", nil)
	if e != nil {
		result.ErrorStage, result.Error = "request", e.Error()
		return result
	}
	req.Header.Set("User-Agent", "Tunnel-Studio-NetworkCheck")
	if padding > 0 {
		req.Header.Set("X-Tunnel-Studio-Probe", strings.Repeat("x", padding))
	}
	response, err := client.Do(req)
	mu.Lock()
	stable := result
	mu.Unlock()
	stable.DurationMs = time.Since(started).Milliseconds()
	if err != nil {
		stable.ErrorStage = "http"
		if !stable.TCPConnected {
			stable.ErrorStage = "tcp"
		}
		stable.Error = err.Error()
		return stable
	}
	stable.HTTPStatus, stable.HTTPProtocol = response.StatusCode, response.Proto
	stable.BodySampleBytes, err = io.CopyN(io.Discard, response.Body, 4096)
	response.Body.Close()
	stable.DurationMs = time.Since(started).Milliseconds()
	if err != nil && !errors.Is(err, io.EOF) {
		stable.ErrorStage, stable.Error = "body", err.Error()
	}
	return stable
}

// Run samples public front doors only, not authenticated Codex requests or streaming.
func Run(ctx context.Context, server string, dial DialFunc) []TargetResult {
	hosts := []string{"chatgpt.com", "auth.openai.com", "www.baidu.com", "www.google.com"}
	out := make([]TargetResult, len(hosts))
	var targets sync.WaitGroup
	for i, host := range hosts {
		targets.Add(1)
		go func(i int, host string) {
			defer targets.Done()
			r := TargetResult{Host: host, DNS: make([]DNSResult, 3)}
			var queries sync.WaitGroup
			for j, query := range []struct {
				transport string
				kind      dnsmessage.Type
			}{{"udp", dnsmessage.TypeA}, {"tcp", dnsmessage.TypeA}, {"udp", dnsmessage.TypeAAAA}} {
				queries.Add(1)
				go func(j int, transport string, kind dnsmessage.Type) {
					defer queries.Done()
					r.DNS[j] = QueryDNS(ctx, dial, server, host, transport, kind)
				}(j, query.transport, query.kind)
			}
			queries.Wait()
			ip := ""
			for _, d := range r.DNS[:2] {
				if d.Error == "" && !d.Truncated && len(d.Answers) > 0 {
					ip = d.Answers[0]
					break
				}
			}
			if ip == "" {
				r.Note = "No usable IPv4 DNS answer; HTTPS skipped. No public DNS fallback."
			} else {
				r.HTTPS = make([]HTTPSResult, 2)
				var requests sync.WaitGroup
				for k, mode := range []string{"http1", "http2-if-supported"} {
					requests.Add(1)
					go func(k int, mode string) { defer requests.Done(); r.HTTPS[k] = CheckHTTPS(ctx, dial, host, ip, mode) }(k, mode)
				}
				requests.Wait()
				r.Note = fmt.Sprintf("Tested first usable IPv4 answer %s only; AAAA is recorded but IPv6 HTTPS is not tested. HTTP status is not authenticated Codex success.", ip)
			}
			out[i] = r
		}(i, host)
	}
	targets.Wait()
	return out
}
