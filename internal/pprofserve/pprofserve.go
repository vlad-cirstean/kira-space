// Package pprofserve is the opt-in profiling endpoint: with KIRA_PPROF=<host:port> set, it serves
// net/http/pprof and a runtime/metrics snapshot on loopback. Off by default, never non-loopback.
package pprofserve

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime/metrics"
	"time"
)

// envVar is the variable that turns the endpoint on.
const envVar = "KIRA_PPROF"

var gauges = []string{
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/total:bytes",
	"/sched/goroutines:goroutines",
}

// Start serves the endpoint when envVar is set and returns the address it bound, "" when off or
// refused. A port of 0 picks a free one, for processes (memory-mcp per session) that share the
// variable.
func Start() string {
	spec := os.Getenv(envVar)
	if spec == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(spec)
	if err != nil || !loopback(host) {
		slog.Warn("pprof refused: want a loopback host:port", "scope", "pprof", "value", spec)
		return ""
	}
	ln, err := net.Listen("tcp", spec)
	if err != nil {
		slog.Warn("pprof listen failed", "scope", "pprof", "err", err)
		return ""
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/metrics", serveMetrics)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	addr := ln.Addr().String()
	slog.Info("pprof listening", "scope", "pprof", "addr", addr)
	return addr
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func serveMetrics(w http.ResponseWriter, _ *http.Request) {
	samples := make([]metrics.Sample, len(gauges))
	for i, n := range gauges {
		samples[i].Name = n
	}
	metrics.Read(samples)
	out := map[string]uint64{}
	for _, s := range samples {
		out[s.Name] = s.Value.Uint64()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
