package mobileweb

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/pairing"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Pairing windows: 120s from enqueue to decide, 60s cooldown after an explicit deny.
const (
	pairTimeout     = 120 * time.Second
	pairCooldown    = 60 * time.Second
	pairMaxQueue    = 8
	pairWriteWindow = pairTimeout + 15*time.Second
	maxPairBody     = 4 << 10
	maxLabelBytes   = 64
	maxAgentBytes   = 256
)

var pairCodeRe = regexp.MustCompile(`^[0-9]{4}$`)

// MobileMeta is what the desktop approval prompt shows. Label and Code are client-claimed; the
// phone shows the same Code so the user can tell it apart from another LAN device's request.
type MobileMeta struct {
	Label     string
	Code      string
	RemoteIP  string
	UserAgent string
}

// Broker is the shared pairing broker specialised for phones.
type Broker = pairing.Broker[MobileMeta]

// NewBroker builds the broker the desktop approval dialog subscribes to. It outlives the server:
// restarting the listener must not drop the dialog's subscription.
func NewBroker(now func() time.Time, sink prompts.Sink) *Broker {
	b := pairing.NewBroker[MobileMeta](pairing.Config{
		Timeout: pairTimeout, Cooldown: pairCooldown, MaxQueue: pairMaxQueue, Now: now,
		Prompts: sink, Kind: prompts.KindMobilePairing,
	})
	b.Title = func(r pairing.Request[MobileMeta]) string {
		if r.Meta.Label == "" {
			return i18n.T("backend.prompt.mobilePairing.title", nil)
		}
		return i18n.T("backend.prompt.mobilePairingNamed.title", map[string]string{"label": r.Meta.Label})
	}
	return b
}

type pairRequest struct {
	Label string `json:"label"`
	Code  string `json:"code"`
}

type deviceBody struct {
	DeviceID string `json:"deviceId"`
	Label    string `json:"label"`
}

// clampLabel cuts to n bytes on a rune boundary.
func clampLabel(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// handlePair parks until the desktop user decides. A phone that already holds a valid cookie is
// answered with its device and no new request.
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if dev, v := s.authenticate(r); v == verdictOK {
		writeJSON(w, http.StatusOK, deviceBody{DeviceID: dev.ID, Label: dev.Label})
		return
	}
	if !s.pairRate.allow(ip) {
		writeRateLimited(w)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(10 * time.Second))
	r.Body = http.MaxBytesReader(w, r.Body, maxPairBody)
	var req pairRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	_ = rc.SetReadDeadline(time.Time{})
	if err != nil || !pairCodeRe.MatchString(req.Code) {
		writeErrorT(w, http.StatusBadRequest, string(ipcerr.CodeBadRequest), "backend.space.mobileweb.pairingFields.body")
		return
	}
	meta := MobileMeta{
		Label: clampLabel(req.Label, maxLabelBytes), Code: req.Code, RemoteIP: ip,
		UserAgent: clampLabel(r.UserAgent(), maxAgentBytes),
	}
	if meta.Label == "" {
		meta.Label = "Unnamed phone"
	}

	_ = rc.SetWriteDeadline(time.Now().Add(pairWriteWindow))
	var requestID string
	var stop func() bool
	outcome := s.cfg.Broker.Request(ip, meta, func(q pairing.Request[MobileMeta]) {
		requestID = q.RequestID
		// A phone that goes away mid-wait must not leave a request that Approve could still honour.
		stop = context.AfterFunc(r.Context(), func() { s.cfg.Broker.Cancel(q.RequestID) })
	})
	if stop != nil {
		stop()
	}

	switch outcome {
	case pairing.Approved:
		s.finishPairing(w, requestID, meta)
	case pairing.Denied:
		writeJSON(w, http.StatusForbidden, errorBody{Code: codePairDenied, Message: i18n.T("backend.space.mobileweb.accessDenied.body", nil), Reason: "denied"})
	case pairing.TimedOut:
		writeJSON(w, http.StatusForbidden, errorBody{Code: codePairDenied, Message: i18n.T("backend.space.mobileweb.requestTimedOut.body", nil), Reason: "timeout"})
	default:
		writeErrorT(w, http.StatusServiceUnavailable, codeUnavailable, "backend.space.mobileweb.pairingUnavailable.body")
	}
}

// finishPairing persists the device row before the cookie is set: the reverse order could hand out
// a token no row backs.
func (s *Server) finishPairing(w http.ResponseWriter, requestID string, meta MobileMeta) {
	tok, ok := s.cfg.Broker.TakeApprovedToken(requestID)
	if !ok {
		writeJSON(w, http.StatusForbidden, errorBody{Code: codePairDenied, Message: i18n.T("backend.space.mobileweb.accessDenied.body", nil), Reason: "denied"})
		return
	}
	now := s.cfg.Now().UnixMilli()
	row := repos.MobileDeviceRow{
		ID: ids.NewID(), Label: meta.Label, UserAgent: meta.UserAgent,
		TokenHash: tok.Hash, TokenSalt: tok.Salt, CreatedAt: now, LastSeenAt: now, LastIP: meta.RemoteIP,
		CanWrite: true, ExpiresAt: now + deviceTTL.Milliseconds(),
	}
	if err := s.cfg.Devices.Insert(row); err != nil {
		slog.Error("mobileweb: insert paired device", "scope", "mobileweb", "err", err)
		writeErrorT(w, http.StatusInternalServerError, string(ipcerr.CodeInternal), "backend.space.mobileweb.pairingSave.body")
		return
	}
	setCookie(w, row.ID, tok.Plain, deviceTTL)
	writeJSON(w, http.StatusOK, deviceBody{DeviceID: row.ID, Label: row.Label})
	if s.cfg.OnDevicesChanged != nil {
		s.cfg.OnDevicesChanged()
	}
}
