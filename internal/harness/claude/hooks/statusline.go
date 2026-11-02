package hooks

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// maxStatusLineBytes bounds one /statusline body; a statusline payload is a few KiB.
const maxStatusLineBytes = 64 * 1024

// rateWindow is one Claude Code rate-limit window as its statusline reports it.
type rateWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	// ResetsAt is Unix epoch seconds.
	ResetsAt int64 `json:"resets_at"`
}

// rateLimits is the only part of a statusline payload this package decodes; either window may be
// absent (Claude Code omits a window it has no data for).
type rateLimits struct {
	FiveHour *rateWindow `json:"five_hour"`
	SevenDay *rateWindow `json:"seven_day"`
}

func (w *rateWindow) window() *harness.RateWindow {
	if w == nil {
		return nil
	}
	return &harness.RateWindow{UsedPercent: w.UsedPercentage, ResetsAt: w.ResetsAt}
}

type statusLineRequest struct {
	RateLimits *rateLimits `json:"rate_limits"`
}

// statusLineEntry is the settings `statusLine` object.
type statusLineEntry struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	Padding         *int   `json:"padding,omitempty"`
	RefreshInterval *int   `json:"refreshInterval,omitempty"`
}

// handleStatusLine is POST /statusline: same token and terminal checks as /hook, then a bounded
// body of which only rate_limits is kept.
func (s *Server) handleStatusLine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(s.ln.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	terminalID := r.Header.Get("X-Kira-Terminal")
	if terminalID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req statusLineRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxStatusLineBytes)).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if s.onStatusLine != nil && req.RateLimits != nil && (req.RateLimits.FiveHour != nil || req.RateLimits.SevenDay != nil) {
		s.onStatusLine(terminalID, harness.RateLimits{FiveHour: req.RateLimits.FiveHour.window(), SevenDay: req.RateLimits.SevenDay.window()})
	}
	w.WriteHeader(http.StatusOK)
}

// wrapStatusLine is the settings statusLine Kira installs: the usage-feed wrapper around the app's own
// statusline user, nil for none. The extra env carries the user's command for the wrapper to run.
func (s *Server) wrapStatusLine(user *statusLineEntry) (*statusLineEntry, []string) {
	entry := &statusLineEntry{Type: "command", Command: s.quotedShim + " statusline"}
	var extra []string
	if user != nil {
		entry.Padding, entry.RefreshInterval = user.Padding, user.RefreshInterval
		if !strings.ContainsRune(user.Command, 0) {
			extra = []string{"KIRA_USER_STATUSLINE=" + user.Command}
		}
	}
	return entry, extra
}

// composeStatusLine writes this terminal's settings file (hooks plus the statusline wrapper, no user
// statusline: that comes only from the app's settings) and returns its single-quoted path.
func (s *Server) composeStatusLine(terminalID string) (string, error) {
	entry, _ := s.wrapStatusLine(nil)
	doc, err := buildHooksDocument(s.quotedShim, entry)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(terminalID))
	path := filepath.Join(s.ln.Dir, "settings-"+hex.EncodeToString(sum[:])[:12]+".json")
	if err := renameio.WriteFile(path, doc, 0o600); err != nil {
		return "", fmt.Errorf("claude hooks: write settings: %w", err)
	}
	return shellSingleQuote(path)
}

// userStatusLine is the app-supplied statusLine of a settings document, nil unless it is a command.
func userStatusLine(raw json.RawMessage) *statusLineEntry {
	var e statusLineEntry
	if json.Unmarshal(raw, &e) != nil || e.Type != "command" || e.Command == "" {
		return nil
	}
	return &e
}
