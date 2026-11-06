// Command flowservers runs the flow harness's real HTTP, HTTPS and gRPC servers for the browser
// e2e-real tier, so both test tiers share one server implementation. It prints one JSON line with
// the addresses, serves recorded requests on the HTTP server at /__requests, and exits when stdin
// closes.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/servers"
)

type info struct {
	HTTP     string `json:"http"`
	HTTPS    string `json:"https"`
	GRPC     string `json:"grpc"`
	GRPCTLS  string `json:"grpcTls"`
	CAFile   string `json:"caFile"`
	ProtoDir string `json:"protoDir"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "flowservers:", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "kira-flowservers-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ca, err := servers.NewCA()
	if err != nil {
		return err
	}
	caFile, err := ca.WriteFile(dir)
	if err != nil {
		return err
	}
	protoDir, err := servers.WriteProtoDir(dir)
	if err != nil {
		return err
	}
	httpSrv, err := servers.StartHTTP()
	if err != nil {
		return err
	}
	defer httpSrv.Close()
	httpsSrv, err := servers.StartHTTPS(ca)
	if err != nil {
		return err
	}
	defer httpsSrv.Close()
	grpcSrv, err := servers.StartGRPC(servers.GRPCOptions{})
	if err != nil {
		return err
	}
	defer grpcSrv.Close()
	grpcTLS, err := servers.StartGRPC(servers.GRPCOptions{CA: ca})
	if err != nil {
		return err
	}
	defer grpcTLS.Close()

	if err := json.NewEncoder(os.Stdout).Encode(info{
		HTTP: httpSrv.URL, HTTPS: httpsSrv.URL, GRPC: grpcSrv.Addr, GRPCTLS: grpcTLS.Addr,
		CAFile: caFile, ProtoDir: protoDir,
	}); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	return nil
}
