package opencode

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const sessionListTimeout = 10 * time.Second

// FindSession returns the one session the CLI lists for q.Cwd created at or after q.Since and not
// in q.Exclude; "" when none or several match. The
// CLI lists only the project of the process cwd, so it runs there. Kira never opens the CLI's
// database.
func (p *Provider) FindSession(ctx context.Context, q harness.SessionQuery) (string, error) {
	bin, err := p.Locate(ctx, "")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, sessionListTimeout)
	defer cancel()
	out, err := toolexec.RunIO(ctx, bin, []string{"session", "list", "--format", "json", "-n", "20"}, q.Cwd, p.ScrubEnv(os.Environ()), nil)
	if err != nil {
		return "", err
	}
	id, ok := parseSessionList(out, q)
	if !ok {
		slog.Warn("opencode: session list output not understood", "scope", "harness", "bytes", len(out))
	}
	return id, nil
}

// parseSessionList reads `session list --format json` (U2, verified shape: an array of
// {id,title,updated,created,projectId,directory}, times in epoch ms). It tolerates a created time
// given as an RFC 3339 string. ok is false when the output is not a JSON array.
func parseSessionList(out []byte, q harness.SessionQuery) (id string, ok bool) {
	var items []struct {
		ID        string `json:"id"`
		Directory string `json:"directory"`
		Created   any    `json:"created"`
	}
	if json.Unmarshal(out, &items) != nil {
		return "", false
	}
	var found []string
	for _, it := range items {
		if !strings.HasPrefix(it.ID, sessionIDPrefix) || !harness.ValidSessionID(it.ID) || !sameDir(it.Directory, q.Cwd) {
			continue
		}
		ms, ok := createdMillis(it.Created)
		if !ok || ms < q.Since.UnixMilli() || slices.Contains(q.Exclude, it.ID) {
			continue
		}
		found = append(found, it.ID)
	}
	if len(found) == 1 {
		id = found[0]
	}
	return id, true
}

func createdMillis(v any) (int64, bool) {
	switch c := v.(type) {
	case float64:
		return int64(c), true
	case string:
		t, err := time.Parse(time.RFC3339, c)
		return t.UnixMilli(), err == nil
	}
	return 0, false
}

func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
