package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// HttpService is P2's one new bound service: a single outbound HTTP request run through the
// *existing* op scheduler the DB adapters already use (D3) — no new scheduler, no new op log,
// no new cancel path.
type HttpService struct {
	Deps appcore.Deps
}

// HttpSendArgs carries httpclient.Request's fields plus the op-log addressing every op needs
// (OpID minted by the renderer, exactly as every data-plane op's already is; TabID so the
// Operations panel can resolve a tab column for a connectionless op). CollectionID/
// EnvironmentID are both possibly empty (a scratch tab has no collection; no environment may be
// selected) and are used for exactly one thing: naming the scope stage 2 resolves secrets
// against.
type HttpSendArgs struct {
	OpID          string              `json:"opId"`
	TabID         string              `json:"tabId"`
	Method        string              `json:"method"`
	URL           string              `json:"url"`
	Headers       []httpclient.Header `json:"headers"`
	Body          httpclient.Body     `json:"body"`
	CollectionID  string              `json:"collectionId"`
	EnvironmentID string              `json:"environmentId"`
	// ItemID is the saved request this tab is bound to, or "" for a scratch tab — supplied by
	// state.ts's send() as tab.state.itemId ?? '' (the tab already knows it, http.ts:208), and used
	// for exactly one thing, recording a response-history entry under the right scope.
	ItemID string `json:"itemId"`
	// Incognito is P71 §4: mirrors ItemID's optional-on-the-TS-side shape (default false on the
	// zero value). Suppresses the response-history Record call below and rides along on the op
	// spec so the op log skips persisting this op too (adapterhost/host.go, oplog/wire.go).
	Incognito bool `json:"incognito"`
	// Options is P90 item 1: per-request overrides, every field nil for "inherit the global
	// default". Resolved against the persisted settings below, never used as sent.
	Options httpclient.Options `json:"options"`
}

// Send runs one HTTP request through Host.RunOp with ConnectionID: nil (proven safe —
// both connection-dependent branches in RunOp/CancelOp already self-guard on a nil connection
// id). ctx is Wails-injected (the method's first parameter is context.Context, bindings.go's
// needsContext), so closing the window mid-request aborts it via CancelWindowCalls; RunOp derives
// its own cancellable context from it, which is what the Stop button's opsCancel path (the app's
// existing cancel mechanism, not Wails' own $CancellablePromise) cancels.
//
// A non-2xx response is not a Go error, so RunOp naturally logs it 'ok' — the op is the exchange,
// and a 404 is what testing an endpoint is for. op.SetCommand carries the human-readable
// outcome for the Operations panel: the method+URL before send, overwritten with "→ status text"
// once the response is known.
func (s *HttpService) Send(ctx context.Context, args HttpSendArgs) (httpclient.Response, error) {
	if args.OpID == "" {
		return httpclient.Response{}, ipcerr.Required("opId")
	}
	if args.TabID == "" {
		return httpclient.Response{}, ipcerr.Required("tabId")
	}

	// P90 §2.5: the globals are read once per send, from the same repo the Settings dialog writes
	// — resolveSendOptions is the whole override rule, a per-request value wins, an absent one
	// inherits the global.
	settings, err := s.Deps.Repos.DbSettings.GetAll()
	if err != nil {
		return httpclient.Response{}, ipcerr.InternalErr(err)
	}
	opts := resolveSendOptions(settings.Api, args.Options, args.Incognito)

	tabID := args.TabID
	spec := adapterhost.OpSpec{ConnectionID: nil, Kind: "http", OpID: args.OpID, TabID: &tabID, Incognito: args.Incognito}

	_, value, err := s.Deps.Router.Host().RunOp(ctx, spec,
		func(runCtx context.Context, op *adapters.OpCtx) (any, error) {
			// op.SetCommand is called with the *unresolved* URL, both times — op_log.
			// command is a persisted SQLite column rendered in the Operations panel, and a
			// {{token}} in a URL is exactly the kind of thing a user puts a credential in.
			// Resolving secrets before this line would write a plaintext credential into
			// kira.sqlite on every send.
			op.SetCommand(fmt.Sprintf("%s %s", args.Method, args.URL))

			// Stage 2 (D6): secrets enter here and go no further — resolved.URL/Headers/Body are
			// handed straight to httpclient.Send and never fed back into anything logged.
			url, headers, body, usedSecrets, resolveErr := s.Deps.ApiVars.ResolveRequest(
				args.URL, args.Headers, args.Body, args.CollectionID, args.EnvironmentID,
			)
			if resolveErr != nil {
				return nil, resolveErr
			}

			// opts is built once, before usedSecrets is known — a per-send copy carries
			// the replacer wire.go needs to mask a body BEFORE truncating it to the cap, not
			// after (the old order could leave an unmasked secret prefix when a secret straddled
			// the cut byte). nil (Replacer's own "nothing to mask" case) is exactly opts'
			// existing zero value, so a request using no secret costs nothing extra.
			sendOpts := opts
			sendOpts.WireMask = apivars.Replacer(usedSecrets)
			resp, sendErr := httpclient.Send(runCtx, httpclient.Request{
				Method:  args.Method,
				URL:     url,
				Headers: headers,
				Body:    body,
			}, sendOpts)
			if sendErr != nil {
				// A failed send's own Timeline (classifySendErr always attaches one) carries the
				// same resolved hop URLs a successful send's does, and mapHttpError
				// — outside this closure, past where usedSecrets goes out of scope — is what turns
				// it into ipcerr.Error.Details, a copyable surface. Masked here, the one place that
				// still has usedSecrets, rather than never masked at all.
				maskSendErrTimeline(sendErr, usedSecrets)
				return nil, sendErr
			}
			op.SetCommand(fmt.Sprintf("%s %s → %d %s", args.Method, args.URL, resp.Status, resp.StatusText))

			// The rendered exchange, the timeline's hop URLs and hop headers, and
			// Redirects/FinalURL are all built from the *resolved* request or a resolved redirect
			// chain, so every one of them carries a secret's plaintext — the exact situation
			// already ruled on for a generated curl command (a copyable text surface), and
			// Redirects[].URL/FinalURL have been persisted to kira.sqlite unmasked. Masked here, at
			// the point the values are already known, rather than inventing a second reveal gate: a
			// secret's plaintext must never reach a copyable surface ungated, nor kira.sqlite
			// outside api_variables.secret_value.
			maskSecrets(&resp, usedSecrets)

			// Recorded from args, never from resolved. Best-effort: a
			// failed insert logs and the send still returns its response — a history feature
			// must never be the reason a user loses the answer they were waiting for.
			// Skipped entirely for an incognito tab — nothing about this send reaches
			// SQLite.
			if !args.Incognito {
				if err := s.Deps.Repos.ResponseHistory.Record(model.ResponseHistoryRecord{
					ItemID:        args.ItemID,
					TabID:         args.TabID,
					EnvironmentID: args.EnvironmentID,
					Method:        args.Method,
					URL:           args.URL,
					Headers:       args.Headers,
					Body:          args.Body,
					Response:      resp,
				}); err != nil {
					slog.Warn("recording response history failed", "scope", "bridge/http", "opId", args.OpID, "err", err)
				}
			}

			return resp, nil
		})
	if err != nil {
		return httpclient.Response{}, mapHttpError(err)
	}
	resp, _ := value.(httpclient.Response)
	return resp, nil
}

// resolveSendOptions is P90's whole override rule: a per-request value wins, an absent one
// inherits the global. Seven independent leaves, no interaction — which is exactly why this is
// not a unit test (CLAUDE.md's bar).
func resolveSendOptions(g model.ApiSettings, o httpclient.Options, incognito bool) httpclient.Options {
	out := httpclient.Options{
		HTTPVersion:      orGlobal(o.HTTPVersion, g.HTTPVersion),
		RequestTimeoutMs: orGlobal(o.RequestTimeoutMs, g.RequestTimeoutMs),
		MaxResponseMb:    orGlobal(o.MaxResponseMb, g.MaxResponseMb),
		SSLVerify:        orGlobal(o.SSLVerify, g.SSLVerify),
		FollowRedirects:  orGlobal(o.FollowRedirects, g.FollowRedirects),
		MaxRedirects:     orGlobal(o.MaxRedirects, g.MaxRedirects),
		DisableCookieJar: orGlobal(o.DisableCookieJar, g.DisableCookieJar),
	}
	out.Ephemeral = incognito
	return out
}

func orGlobal[T any](override *T, global T) *T {
	if override != nil {
		return override
	}
	return &global
}

// HttpCookiesArgs addresses the Cookies tab's request-mode listing (P90 item 2) — what the shared
// jar would send to URL right now.
type HttpCookiesArgs struct {
	URL string `json:"url"`
}

// HttpCookieDeleteArgs names one jar entry by the domain, path and name HttpService.Cookies
// reported; URL is only the list to return afterwards.
type HttpCookieDeleteArgs struct {
	URL    string `json:"url"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// Cookies/DeleteCookie/ClearCookies are P90 item 2's three thin bound methods. None of these go
// through RunOp — they touch no connection, issue no network I/O, and have nothing to cancel or
// log; SettingsService (above) is the precedent for a bound service that just reads.
func (s *HttpService) Cookies(args HttpCookiesArgs) ([]httpclient.Cookie, error) {
	cookies, err := httpclient.JarCookies(args.URL)
	if err != nil {
		return nil, mapHttpError(err)
	}
	return cookies, nil
}

func (s *HttpService) DeleteCookie(args HttpCookieDeleteArgs) ([]httpclient.Cookie, error) {
	if args.Name == "" || args.Domain == "" || args.Path == "" {
		return nil, ipcerr.Contract("cookie name, domain and path are required")
	}
	httpclient.DeleteJarCookie(args.Domain, args.Path, args.Name)
	return s.Cookies(HttpCookiesArgs{URL: args.URL})
}

func (s *HttpService) ClearCookies() error {
	httpclient.ClearJar()
	return nil
}

// maskSecrets is widened to four fields instead of one. resp.Redirects[].URL and resp.FinalURL are
// resolved URLs already reaching kira.sqlite unmasked today, and the per-hop URL/header list would
// widen the same gap rather than open a new one. All four are masked here, at the point the values
// are already known, rather than inventing a second reveal gate. A no-op when there is no rendered
// exchange (a dump error) or no secret was substituted.
func maskSecrets(resp *httpclient.Response, used []apivars.UsedSecret) {
	replacer := apivars.Replacer(used)
	if replacer == nil {
		return
	}
	if resp.Wire != nil {
		// Send's own Options.WireMask already masked the body portion of Wire.Request
		// BEFORE wire.go truncated it to the cap — this replace is a second pass over the whole
		// string, catching the request head httputil.DumpRequestOut renders (never capped, so
		// never at risk of the straddle-the-cut bug WireMask exists for) and cheap idempotent
		// no-ops over whatever WireMask already replaced in the body.
		resp.Wire.Request = replacer.Replace(resp.Wire.Request)
		// The reasoning ("a Location header is a URL too — the most likely place for a
		// secret-bearing query string to reappear on a redirect") applies identically here:
		// ResponseHead renders the final hop's own header list, the same bytes the Timeline pane
		// already masks per-hop. Leaving it unmasked let RawExchangePane.vue's "N secret values
		// are shown as {{name}}" note claim more than the code did.
		resp.Wire.ResponseHead = replacer.Replace(resp.Wire.ResponseHead)
		resp.Wire.MaskedSecrets = apivars.DistinctNames(used)
	}
	for i := range resp.Headers {
		resp.Headers[i].Value = replacer.Replace(resp.Headers[i].Value)
	}
	for i := range resp.Timeline.Hops {
		resp.Timeline.Hops[i].URL = replacer.Replace(resp.Timeline.Hops[i].URL)
		for j := range resp.Timeline.Hops[i].Headers {
			// F16: a Location header is a URL too — the most likely place for a secret-bearing
			// query string to reappear on a redirect.
			resp.Timeline.Hops[i].Headers[j].Value = replacer.Replace(resp.Timeline.Hops[i].Headers[j].Value)
		}
	}
	for i := range resp.Redirects {
		resp.Redirects[i].URL = replacer.Replace(resp.Redirects[i].URL)
	}
	resp.FinalURL = replacer.Replace(resp.FinalURL)
	// P90 §2.4b: a secret in a request is at least as likely to be a session token as a header
	// value, and SentCookies/ReceivedCookies is a new copyable surface reaching kira.sqlite via
	// ResponseHistory.Record.
	for i := range resp.SentCookies {
		resp.SentCookies[i].Value = replacer.Replace(resp.SentCookies[i].Value)
	}
	for i := range resp.ReceivedCookies {
		resp.ReceivedCookies[i].Value = replacer.Replace(resp.ReceivedCookies[i].Value)
	}
}

// maskSendErrTimeline is D14's reach into D15's own new failure channel: a failed send's Timeline
// (herr.Timeline, attached by classifySendErr for a transport failure) carries the same resolved
// hop URLs a successful send's Response.Timeline does, and mapHttpError below — called outside
// this function's caller's closure, past where usedSecrets is in scope — is what turns it into
// ipcerr.Error.Details, a copyable surface. herr.Message itself is masked too: both
// classifySendErr's transport-error path (client.go's err.Error() wraps the dialed, i.e. resolved,
// URL) and resolveURL's bad-URL-parse-error path (client.go:159, raised before a Timeline even
// exists) can embed the resolved URL straight into the message net/http or url.Parse produced —
// exactly the message RunOp's own err.Error() persists verbatim to op_log.error (host.go). A
// no-op when err is not an *httpclient.Error or no secret was actually substituted.
//
// Round-2 review finding 3: this used to mask only Hops[i].URL, unlike maskSecrets' success path
// which additionally masks Hops[i].Headers[j].Value (e.g. a Location header carrying a
// secret-bearing redirect URL) — those two fields need the exact same treatment here. Separately,
// httpclient/timeline.go's finishFailed writes a *second*, independent copy of the resolved URL
// into Hops[i].Error via err.Error(), a field masking herr.Message alone never reaches since it is
// a wholly different field. Both are rendered by TimelinePane.vue for a failed send (the
// failed-hop chip and the "Response headers" disclosure), so both are masked here too.
func maskSendErrTimeline(err error, used []apivars.UsedSecret) {
	var herr *httpclient.Error
	if !errors.As(err, &herr) {
		return
	}
	replacer := apivars.Replacer(used)
	if replacer == nil {
		return
	}
	herr.Message = replacer.Replace(herr.Message)
	if herr.Timeline == nil {
		return
	}
	for i := range herr.Timeline.Hops {
		herr.Timeline.Hops[i].URL = replacer.Replace(herr.Timeline.Hops[i].URL)
		herr.Timeline.Hops[i].Error = replacer.Replace(herr.Timeline.Hops[i].Error)
		for j := range herr.Timeline.Hops[i].Headers {
			herr.Timeline.Hops[i].Headers[j].Value = replacer.Replace(herr.Timeline.Hops[i].Headers[j].Value)
		}
	}
}

// mapHttpError joins httpclient's own four-code vocabulary into the ipcerr family — not
// adapters.ErrorCode, whose CodeConnect/CodeEngineDown views/shared/viewOp.ts's
// DISCONNECTED_CODES would misread as "the database connection is gone" and pop a Reconnect gate
// over a tab that has no connection to reconnect. When the failure carries a Timeline —
// already masked by maskSendErrTimeline above, before the error ever left the RunOp closure — it
// is marshalled into Details for a renderer that knows how to read it (control.ts/state.ts).
func mapHttpError(err error) error {
	var herr *httpclient.Error
	if errors.As(err, &herr) {
		e := ipcerr.New(string(herr.Code), herr.Message)
		if herr.Timeline != nil {
			if b, mErr := json.Marshal(herr.Timeline); mErr == nil {
				e.Details = b
			} else {
				slog.Warn("marshalling failed-send timeline failed", "scope", "bridge/http", "err", mErr)
			}
		}
		return e
	}
	return ipcerr.Wrap(err)
}
