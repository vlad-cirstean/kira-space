package dockerflow

import (
	"context"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
)

func TestStatusClasses(t *testing.T) {
	app := flowharness.New(t)
	status := func(t *testing.T, host string) docker.Status {
		t.Helper()
		t.Setenv("DOCKER_HOST", host)
		return app.W.Docker.Status(docker.StatusArgs{Refresh: true})
	}

	t.Run("daemon-down", func(t *testing.T) {
		st := status(t, "unix://"+filepath.Join(t.TempDir(), "none.sock"))
		if st.State != "unavailable" || st.Reason != "daemon-down" {
			t.Fatalf("status = %+v, want unavailable/daemon-down", st)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		st := status(t, "tcp://"+addr)
		if st.State != "unavailable" || st.Reason != "unreachable" {
			t.Fatalf("status = %+v, want unavailable/unreachable", st)
		}
	})

	t.Run("tls", func(t *testing.T) {
		srv := httptest.NewTLSServer(nil)
		srv.Config.ErrorLog = log.New(io.Discard, "", 0)
		defer srv.Close()
		st := status(t, "https://"+srv.Listener.Addr().String())
		if st.State != "unavailable" || st.Reason != "tls" {
			t.Fatalf("status = %+v, want unavailable/tls", st)
		}
	})

	t.Run("not-installed", func(t *testing.T) {
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			t.Skip("not-installed needs a host without /var/run/docker.sock; this host has one")
		}
		st := status(t, "")
		if st.State != "unavailable" || st.Reason != "not-installed" {
			t.Fatalf("status = %+v, want unavailable/not-installed", st)
		}
	})

	t.Run("ok", func(t *testing.T) {
		d := flowharness.RequireDocker(t)
		st := app.W.Docker.Status(docker.StatusArgs{Refresh: true})
		if st.State != "ok" || st.Engine == nil {
			t.Fatalf("status = %+v, want ok with engine facts", st)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ver, err := d.Cli.ServerVersion(ctx, client.ServerVersionOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if st.Engine.Version != ver.Version {
			t.Fatalf("engine version = %q, want client's %q", st.Engine.Version, ver.Version)
		}
		if st.Engine.CPUs == 0 || st.Engine.MemTotal == 0 {
			t.Fatalf("engine facts empty: %+v", st.Engine)
		}
	})
}
