package connections

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// resolved is resolve's return shape: the engine-bound config plus
// the two preconnect fields stripped from it, which only doConnect needs.
type resolved struct {
	config            model.ResolvedConnectionConfig
	preconnect        *string
	preconnectSidecar bool
	preconnectDelay   time.Duration
	// readyAddr is the loopback host:port the pre-connect script is expected to open ("" when the
	// target is not a loopback address with an explicit port).
	readyAddr string
	// P28 §5.5: not a ResolvedConnectionConfig field (that shape carries a password and exists for
	// adapters; a UI pacing knob doesn't belong in it) — carried alongside it purely so attemptConnect
	// can hand it to Backend.SetThrottle from the summary it already read.
	throttlePerSec float64
}

// resolve reads the row and resolves the secret its file names: fields mode the password, URI mode the
// whole URI, with Options derived from its query on top of the stored ones. Never returned over IPC (the
// engine channel is the only consumer). An unset or unreadable secret fails the connect and names the
// reference; an empty password is never substituted.
func resolve(conns *repos.ConnectionsRepo, files *connfiles.Loader, id string) (resolved, error) {
	summary, err := conns.Get(id)
	if err != nil {
		return resolved{}, fmt.Errorf("connections: resolve %s: %w", id, err)
	}
	if summary == nil {
		return resolved{}, fmt.Errorf("connection %s not found", id)
	}
	rec, ok := files.RecordOf(id)
	if !ok {
		return resolved{}, fmt.Errorf("connection %s has no file", id)
	}

	var password, uri *string
	options := summary.Options
	if summary.Mode == "uri" {
		uri, err = resolveSecret(files, connfiles.KindURI, rec.File.URI)
		if err != nil {
			return resolved{}, err
		}
		if uri == nil {
			return resolved{}, errURIRequired()
		}
		options = mergeOptions(options, uriQueryOptions(*uri))
	} else if password, err = resolveSecret(files, connfiles.KindPassword, rec.File.Password); err != nil {
		return resolved{}, err
	}

	cfg := model.ResolvedConnectionConfig{
		ID: summary.ID, Name: summary.Name, Kind: summary.Kind, Color: summary.Color,
		Mode: summary.Mode, ReadOnly: summary.ReadOnly, Host: summary.Host, Port: summary.Port,
		Database: summary.Database, Username: summary.Username, URI: uri, Options: options,
		SortOrder: summary.SortOrder, CreatedAt: summary.CreatedAt, UpdatedAt: summary.UpdatedAt,
		Password: password,
	}
	return resolved{
		config:            cfg,
		readyAddr:         readyAddr(cfg),
		preconnect:        summary.Preconnect,
		preconnectSidecar: summary.PreconnectSidecar,
		preconnectDelay:   time.Duration(summary.PreconnectDelaySec) * time.Second,
		throttlePerSec:    summary.ThrottlePerSec,
	}, nil
}

// openSecret resolves the reference ref of a secret kind; nil when the file names none. A secret that
// is not set wraps configfs.ErrSecretMissing, one that cannot be decrypted connfiles.ErrSecretUnreadable.
func openSecret(files *connfiles.Loader, kind, ref string) (*string, error) {
	if ref == "" {
		return nil, nil
	}
	r := configfs.Resolver{Secrets: files.Secrets().Snapshot(), Namespace: kind}
	val, _, err := r.Resolve(context.Background(), ref)
	if err != nil {
		return nil, err
	}
	return &val, nil
}

// resolveSecret is openSecret with its failures as IPC errors that name the reference.
func resolveSecret(files *connfiles.Loader, kind, ref string) (*string, error) {
	val, err := openSecret(files, kind, ref)
	switch {
	case err == nil:
		return val, nil
	case errors.Is(err, configfs.ErrSecretMissing):
		return nil, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connectionFiles.secretUnset.body", map[string]string{"name": connfiles.SecretName(ref)})
	case errors.Is(err, connfiles.ErrSecretUnreadable):
		return nil, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connectionFiles.secretUnreadable.body", map[string]string{"name": connfiles.SecretName(ref)})
	}
	return nil, ipcerr.Wrap(err)
}

// mergeOptions overlays over onto a copy of base.
func mergeOptions(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// readyCheck is the TCP-dial readiness probe for addr, or nil (settle-window fallback) when addr is "".
func readyCheck(addr string) *preconnect.ReadyCheck {
	if addr == "" {
		return nil
	}
	return preconnect.DialReady(addr)
}

// readyAddr derives the loopback host:port a pre-connect tunnel script is expected to listen on.
// A non-loopback target says nothing about the script (it is reachable regardless), so it yields "".
func readyAddr(cfg model.ResolvedConnectionConfig) string {
	if connfiles.FileKind(cfg.Kind) {
		return ""
	}
	var host, port string
	if cfg.Mode == "uri" {
		if cfg.URI == nil {
			return ""
		}
		u, err := url.Parse(*cfg.URI)
		if err != nil || strings.Contains(u.Host, ",") {
			return ""
		}
		host, port = u.Hostname(), u.Port()
	} else {
		if cfg.Host == nil || cfg.Port == nil {
			return ""
		}
		host, port = *cfg.Host, strconv.Itoa(*cfg.Port)
	}
	if host == "" || port == "" || !isLoopback(host) {
		return ""
	}
	return net.JoinHostPort(host, port)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
