package servers

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxKeptBody bounds the request body bytes a Request keeps; the hash and length cover all of it.
const maxKeptBody = 1 << 20

// Request is one request the server received.
type Request struct {
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	Query      string      `json:"query"`
	Host       string      `json:"host"`
	Proto      string      `json:"proto"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
	BodyLen    int         `json:"bodyLen"`
	BodySHA256 string      `json:"bodySha256"`
	// Canceled is set when the client went away before the handler finished.
	Canceled bool `json:"canceled"`
	// TLS reports whether the request arrived over TLS.
	TLS bool `json:"tls"`
}

// HTTPServer is a running HTTP or HTTPS server that records every request.
type HTTPServer struct {
	// URL uses 127.0.0.1; AltURL reaches the same listener as localhost, a different host to a client.
	URL, AltURL string
	Addr        string

	srv *http.Server
	ln  net.Listener

	mu   sync.Mutex
	reqs []*Request
}

// StartHTTP serves plain HTTP/1.1 on a free loopback port.
func StartHTTP() (*HTTPServer, error) { return start(nil) }

// StartHTTPS serves TLS (HTTP/2 and HTTP/1.1) with a leaf certificate from ca for sans; default
// sans are 127.0.0.1 and localhost.
func StartHTTPS(ca *CA, sans ...string) (*HTTPServer, error) {
	if len(sans) == 0 {
		sans = []string{"127.0.0.1", "localhost"}
	}
	cert, err := ca.Issue(sans...)
	if err != nil {
		return nil, err
	}
	return start(&tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}})
}

func start(cfg *tls.Config) (*HTTPServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	scheme := "http"
	if cfg != nil {
		scheme = "https"
	}
	port := ln.Addr().(*net.TCPAddr).Port
	s := &HTTPServer{
		ln: ln, Addr: ln.Addr().String(),
		URL:    fmt.Sprintf("%s://127.0.0.1:%d", scheme, port),
		AltURL: fmt.Sprintf("%s://localhost:%d", scheme, port),
	}
	s.srv = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	if cfg != nil {
		s.srv.TLSConfig = cfg
	}
	go func() {
		if cfg != nil {
			_ = s.srv.ServeTLS(ln, "", "")
			return
		}
		_ = s.srv.Serve(ln)
	}()
	return s, nil
}

// Close stops the server and drops open connections.
func (s *HTTPServer) Close() { _ = s.srv.Close() }

// Requests returns a snapshot of every request received, in arrival order.
func (s *HTTPServer) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.reqs))
	for i, r := range s.reqs {
		out[i] = *r
	}
	return out
}

func (s *HTTPServer) record(r *http.Request) *Request {
	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	kept := body
	if len(kept) > maxKeptBody {
		kept = kept[:maxKeptBody]
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	rec := &Request{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Host: r.Host, Proto: r.Proto,
		Header: r.Header.Clone(), Body: kept, BodyLen: len(body), BodySHA256: hex.EncodeToString(sum[:]),
		TLS: r.TLS != nil,
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, rec)
	s.mu.Unlock()
	return rec
}

func (s *HTTPServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", s.echo)
	mux.HandleFunc("/redirect", redirect)
	mux.HandleFunc("/cookie/set", cookieSet)
	mux.HandleFunc("/cookie/echo", cookieEcho)
	mux.HandleFunc("/basic", basicAuth)
	mux.HandleFunc("/bearer", bearerAuth)
	mux.HandleFunc("/bytes", bytesHandler)
	mux.HandleFunc("/gzip", gzipHandler)
	mux.HandleFunc("/slow", slow)
	mux.HandleFunc("/status", statusHandler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__requests" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(s.Requests())
			return
		}
		rec := s.record(r)
		mux.ServeHTTP(w, r)
		if errors.Is(r.Context().Err(), context.Canceled) {
			s.mu.Lock()
			rec.Canceled = true
			s.mu.Unlock()
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// echo answers with what the server saw: method, headers, body hash and length, protocol.
func (s *HTTPServer) echo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	writeJSON(w, map[string]any{
		"method": r.Method, "path": r.URL.Path, "query": r.URL.RawQuery, "host": r.Host, "proto": r.Proto,
		"header": r.Header, "bodyLen": len(body), "bodySha256": hex.EncodeToString(sum[:]), "body": string(body),
	})
}

// redirect hops n times with status code, then lands on to (absolute URL) or /echo.
func redirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("n"))
	code, _ := strconv.Atoi(q.Get("code"))
	if code == 0 {
		code = http.StatusFound
	}
	to := q.Get("to")
	if n <= 0 {
		if to == "" {
			to = "/echo"
		}
		http.Redirect(w, r, to, code)
		return
	}
	next := fmt.Sprintf("/redirect?n=%d&code=%d", n-1, code)
	if to != "" {
		next += "&to=" + strings.ReplaceAll(to, "&", "%26")
	}
	http.Redirect(w, r, next, code)
}

func cookieSet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c := &http.Cookie{Name: q.Get("name"), Value: q.Get("value"), Path: q.Get("path"), Domain: q.Get("domain")}
	if c.Name == "" {
		c.Name, c.Value = "session", "abc"
	}
	if c.Path == "" {
		c.Path = "/"
	}
	http.SetCookie(w, c)
	writeJSON(w, map[string]any{"set": c.Name})
}

func cookieEcho(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, c := range r.Cookies() {
		out[c.Name] = c.Value
	}
	writeJSON(w, out)
}

// basicAuth wants kira:secret.
func basicAuth(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok || u != "kira" || p != "secret" {
		w.Header().Set("WWW-Authenticate", `Basic realm="flow"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"user": u})
}

// bearerAuth accepts any non-empty bearer token, or only ?expect= when given.
func bearerAuth(w http.ResponseWriter, r *http.Request) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || tok == "" || (r.URL.Query().Has("expect") && tok != r.URL.Query().Get("expect")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"token": tok})
}

// bytesHandler writes n bytes of a repeating pattern.
func bytesHandler(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.ParseInt(r.URL.Query().Get("n"), 10, 64)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	chunk := bytes.Repeat([]byte("0123456789abcdef"), 4096)
	for n > 0 {
		k := min(int64(len(chunk)), n)
		if _, err := w.Write(chunk[:k]); err != nil {
			return
		}
		n -= k
	}
}

func gzipHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	zw := gzip.NewWriter(w)
	_ = json.NewEncoder(zw).Encode(map[string]string{"hello": "gzip"})
	_ = zw.Close()
}

// slow sends headers and a first byte, then stalls the body for ms (default 5000) or until the
// client leaves.
func slow(w http.ResponseWriter, r *http.Request) {
	ms, _ := strconv.Atoi(r.URL.Query().Get("ms"))
	if ms == 0 {
		ms = 5000
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("start"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		_, _ = w.Write([]byte("end"))
	case <-r.Context().Done():
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	code, _ := strconv.Atoi(r.URL.Query().Get("code"))
	if code < 100 || code > 599 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
}
