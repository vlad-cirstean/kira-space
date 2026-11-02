package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

const mcpListTimeout = 30 * time.Second

var (
	mcpListTTL = 30 * time.Second
	// mcpPresenceTTL is how long the presence check reuses a list: names change rarely.
	mcpPresenceTTL = 5 * time.Minute
)

// SetMCPListTTL replaces how long an `opencode mcp list` result is reused and returns the restore
// func (a flow test passes 0 to see a config change at once).
func SetMCPListTTL(d time.Duration) (restore func()) {
	prevList, prevPresence := mcpListTTL, mcpPresenceTTL
	mcpListTTL, mcpPresenceTTL = d, d
	return func() { mcpListTTL, mcpPresenceTTL = prevList, prevPresence }
}

// errMCPList is the only error a failed listing surfaces; the CLI's output never reaches it.
var errMCPList = errors.New("could not list your OpenCode MCP servers")

var (
	ansi       = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	serverLine = regexp.MustCompile(`^[•*]\s+\S\s+([A-Za-z0-9_.-]+)\s+(\S.*)$`)
	detailLine = regexp.MustCompile(`^\|\s+(\S.*)$`)
)

// parseMCPList reads `opencode mcp list` (U10, partly verified: human text with colour codes).
// A server is `•  <glyph> <name> <status>` followed by `|      <detail>` lines whose last one is the
// target: a URL or the command. ok is false when the text has no server line and no summary. Only
// name, transport, URL host or command base name and a status class are kept.
func parseMCPList(out string) (servers []harness.UserServer, ok bool) {
	servers = []harness.UserServer{}
	out = ansi.ReplaceAllString(out, "")
	var cur *harness.UserServer
	flush := func() {
		if cur != nil {
			servers = append(servers, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r ")
		if m := serverLine.FindStringSubmatch(line); m != nil {
			flush()
			cur = &harness.UserServer{Name: m[1], Transport: "stdio", Status: statusOf(m[2])}
			continue
		}
		if d := detailLine.FindStringSubmatch(line); d != nil && cur != nil {
			cur.Transport, cur.Target = targetOf(strings.TrimSpace(d[1]))
		}
	}
	flush()
	ok = len(servers) > 0 || strings.Contains(out, "server(s)") || strings.Contains(strings.ToLower(out), "no mcp servers")
	return servers, ok
}

// targetOf classifies a detail line: a URL gives the host, anything else the command base name.
// A detail line that is neither a URL nor a command (an error text) is kept as a stdio target of
// its first word's base name, bounded; an error line is replaced by the next detail line.
func targetOf(d string) (transport, target string) {
	if u, err := url.Parse(d); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return "http", u.Host
	}
	first, _, _ := strings.Cut(d, " ")
	return "stdio", filepath.Base(first)
}

func statusOf(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(t, "connected"):
		return "connected"
	case strings.Contains(t, "needs"), strings.Contains(t, "authenticat"):
		return "needsAuth"
	case strings.HasPrefix(t, "failed"):
		return "failed"
	case strings.HasPrefix(t, "pending"), strings.HasPrefix(t, "connecting"):
		return "pending"
	}
	return "unknown"
}

type listCache struct {
	mu      sync.Mutex
	entries map[string]listEntry
	flight  singleflight.Group
}

type listEntry struct {
	servers []harness.UserServer
	at      time.Time
}

// UserMCPServers asks `opencode mcp list` from dir. Kira never opens OpenCode's config files. The
// CLI health-checks every server on each call, so the result is cached per dir for a short while;
// a failed listing falls back to the last good one with every status reset to unknown.
func (p *Provider) UserMCPServers(ctx context.Context, dir string) ([]harness.UserServer, error) {
	servers, stale, err := p.userServers(ctx, dir, mcpListTTL)
	if stale {
		for i := range servers {
			servers[i].Status = "unknown"
		}
	}
	return servers, err
}

// UserMCPPresence is UserMCPServers for a presence check of the want names; with wait false it
// never blocks (known is false unless a cached list has every name).
func (p *Provider) UserMCPPresence(ctx context.Context, dir string, want []string, wait bool) (servers []harness.UserServer, known bool, err error) {
	c := &p.mcpList
	c.mu.Lock()
	e, have := c.entries[dir]
	c.mu.Unlock()
	age := time.Since(e.at)
	complete := have && hasAll(e.servers, want)
	if have && ((complete && age < mcpPresenceTTL) || age < mcpListTTL) {
		return slices.Clone(e.servers), true, nil
	}
	if wait || mcpPresenceTTL <= 0 {
		servers, _, err = p.userServers(ctx, dir, mcpListTTL)
		return servers, err == nil, err
	}
	go func() { _, _, _ = p.userServers(context.Background(), dir, 0) }()
	if complete {
		return slices.Clone(e.servers), true, nil
	}
	return nil, false, nil
}

func hasAll(servers []harness.UserServer, want []string) bool {
	for _, name := range want {
		if !slices.ContainsFunc(servers, func(u harness.UserServer) bool { return u.Name == name }) {
			return false
		}
	}
	return true
}

type listResult struct {
	servers []harness.UserServer
	stale   bool
}

// userServers lists dir, reusing a result younger than ttl. The shared flight runs on a context
// detached from the first caller; each caller waits under its own ctx.
func (p *Provider) userServers(ctx context.Context, dir string, ttl time.Duration) (servers []harness.UserServer, stale bool, err error) {
	c := &p.mcpList
	c.mu.Lock()
	if e, ok := c.entries[dir]; ok && time.Since(e.at) < ttl {
		c.mu.Unlock()
		return slices.Clone(e.servers), false, nil
	}
	c.mu.Unlock()
	flightCtx := context.WithoutCancel(ctx)
	ch := c.flight.DoChan(dir, func() (any, error) {
		servers, err := p.runMCPList(flightCtx, dir)
		if err != nil {
			c.mu.Lock()
			old, ok := c.entries[dir]
			c.mu.Unlock()
			if ok {
				return listResult{old.servers, true}, nil
			}
			return nil, err
		}
		c.mu.Lock()
		if c.entries == nil {
			c.entries = map[string]listEntry{}
		}
		c.entries[dir] = listEntry{servers, time.Now()}
		c.mu.Unlock()
		return listResult{servers, false}, nil
	})
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case r := <-ch:
		if r.Err != nil {
			return nil, false, r.Err
		}
		res := r.Val.(listResult)
		return slices.Clone(res.servers), res.stale, nil
	}
}

func (p *Provider) runMCPList(ctx context.Context, dir string) ([]harness.UserServer, error) {
	if _, err := p.Locate(ctx, ""); err != nil {
		return nil, errMCPList
	}
	ctx, cancel := context.WithTimeout(ctx, mcpListTimeout)
	defer cancel()
	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	argv := loginshell.BuildArgv(shell, login, "exec opencode mcp list")
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = p.ScrubEnv(os.Environ())
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: timed out", errMCPList)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("%w: exit code %d", errMCPList, ee.ExitCode())
		}
		return nil, errMCPList
	}
	servers, ok := parseMCPList(out.String())
	if !ok {
		return nil, fmt.Errorf("%w: unrecognised output", errMCPList)
	}
	return servers, nil
}
