package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// TestMaskSecrets_RedirectURLsFinalURLAndTimelineHopsBeforePersisting is the security assertion
// — not a CRUD round trip — the same posture the masking commit took for header/body masking
// (tests/ui/http-raw.spec.ts's "N secret values are shown as {{name}}" case).
//
// Response.Redirects[].URL and Response.FinalURL — resolved URLs — have been persisted to
// kira.sqlite unmasked, and the per-hop Timeline URL/header list would widen the same gap. This
// proves the fix at the exact point it runs (maskSecrets), then proves it end to end through the
// real path a live send takes: mask, then Record, then read the raw snapshot_json column back out
// of a real, migrated SQLite database — the secret's plaintext must appear in neither, the same
// way TestResponseHistoryRecordStripsWireBeforePersisting (response_history_test.go) proves Wire
// never reaches the column at all.
func TestMaskSecrets_RedirectURLsFinalURLAndTimelineHopsBeforePersisting(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	const masked = "{{apiKey}}"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	resp := httpclient.Response{
		Status: 200, StatusText: "OK", Proto: "HTTP/1.1",
		FinalURL: "https://api.example.com/orders/final?token=" + secret,
		Redirects: []httpclient.RedirectHop{
			{Status: 301, URL: "https://api.example.com/orders?token=" + secret},
		},
		Timeline: httpclient.Timeline{
			Hops: []httpclient.TimelineHop{
				{
					Index: 0, Method: "GET",
					URL:    "https://api.example.com/orders?token=" + secret,
					Status: 301,
					Headers: []httpclient.Header{
						// F16: a redirect's own Location header is a URL too, and the most likely
						// place for a secret-bearing query string to reappear.
						{Name: "Location", Value: "https://api.example.com/orders/final?token=" + secret},
					},
				},
				{Index: 1, Method: "GET", URL: "https://api.example.com/orders/final?token=" + secret, Status: 200},
			},
			TotalMs: 12,
		},
	}

	maskSecrets(&resp, usedSecrets)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, secret) {
			t.Errorf("%s = %q still contains the raw secret", label, s)
		}
		if !strings.Contains(s, masked) {
			t.Errorf("%s = %q, want it to contain %q", label, s, masked)
		}
	}
	assertMasked(t, "FinalURL", resp.FinalURL)
	assertMasked(t, "Redirects[0].URL", resp.Redirects[0].URL)
	assertMasked(t, "Timeline.Hops[0].URL", resp.Timeline.Hops[0].URL)
	assertMasked(t, "Timeline.Hops[1].URL", resp.Timeline.Hops[1].URL)
	assertMasked(t, "Timeline.Hops[0].Headers[0].Value (Location)", resp.Timeline.Hops[0].Headers[0].Value)

	// The real persistence path: Send (http.go) calls maskSecrets before ResponseHistory.Record on
	// every send — reproduced here against a real migrated SQLite database (storage.Open(), the
	// same helper repos_test.go's own newRepos wraps) rather than a fixture, so a future refactor
	// that changes the schema or the marshalling would still be caught.
	t.Setenv("KIRA_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	historyRepo := &repos.ResponseHistoryRepo{DB: db.DB}
	if err := historyRepo.Record(model.ResponseHistoryRecord{
		TabID: "tab1", Method: "GET", URL: "https://api.example.com/orders",
		Headers: []httpclient.Header{}, Body: httpclient.Body{Mode: "none"},
		Response: resp,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := historyRepo.List("tab:tab1")
	if err != nil || len(entries) != 1 {
		t.Fatalf("List(tab:tab1) = %d entries, err %v, want 1", len(entries), err)
	}

	var rawSnapshot string
	if err := db.DB.QueryRow(
		`SELECT snapshot_json FROM api_response_history WHERE id = ?`, entries[0].ID,
	).Scan(&rawSnapshot); err != nil {
		t.Fatalf("query snapshot_json: %v", err)
	}
	if strings.Contains(rawSnapshot, secret) {
		t.Fatalf("stored snapshot_json contains the raw secret — it reached kira.sqlite in plaintext:\n%s", rawSnapshot)
	}
	if !strings.Contains(rawSnapshot, masked) {
		t.Fatalf("stored snapshot_json does not contain the masked placeholder %q at all:\n%s", masked, rawSnapshot)
	}

	// D10: the timeline itself survives the round trip (unlike Wire, P9 D7) — confirming the
	// masked values are what a viewer of this history entry actually sees later, not just what was
	// written.
	snap, err := historyRepo.Get(entries[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(snap.Response.Timeline.Hops) != 2 {
		t.Fatalf("decoded Timeline.Hops = %d entries, want 2", len(snap.Response.Timeline.Hops))
	}
	assertMasked(t, "decoded Timeline.Hops[0].URL", snap.Response.Timeline.Hops[0].URL)
	assertMasked(t, "decoded FinalURL", snap.Response.FinalURL)
}

// TestMaskSecrets_MasksResponseHeadAndHeaders: maskSecrets masked the timeline and redirect chain
// but never resp.Wire.ResponseHead or resp.Headers — the exact response half RawExchangePane.vue
// renders and ResponseHistoryRepo.Record persists, under a UI note claiming the whole exchange is
// masked. The reasoning ("a Location header is a URL too")
// applies identically to a response header.
func TestMaskSecrets_MasksResponseHeadAndHeaders(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	const masked = "{{apiKey}}"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	resp := httpclient.Response{
		Status: 200, StatusText: "OK", Proto: "HTTP/1.1",
		Headers: []httpclient.Header{
			{Name: "X-Echo-Token", Value: secret},
		},
		Wire: &httpclient.WireExchange{
			Request:      "GET / HTTP/1.1\r\n\r\n",
			ResponseHead: "HTTP/1.1 200 OK\r\nX-Echo-Token: " + secret + "\r\n\r\n",
		},
	}

	maskSecrets(&resp, usedSecrets)

	if strings.Contains(resp.Wire.ResponseHead, secret) {
		t.Errorf("Wire.ResponseHead = %q still contains the raw secret", resp.Wire.ResponseHead)
	}
	if !strings.Contains(resp.Wire.ResponseHead, masked) {
		t.Errorf("Wire.ResponseHead = %q, want it to contain %q", resp.Wire.ResponseHead, masked)
	}
	if strings.Contains(resp.Headers[0].Value, secret) {
		t.Errorf("Headers[0].Value = %q still contains the raw secret", resp.Headers[0].Value)
	}
	if !strings.Contains(resp.Headers[0].Value, masked) {
		t.Errorf("Headers[0].Value = %q, want it to contain %q", resp.Headers[0].Value, masked)
	}
}

// TestMaskSendErrTimeline_MasksFailedSendHopURL is the reach into the new failure channel:
// mapHttpError marshals herr.Timeline into ipcerr.Error.Details — a copyable surface —
// so a failed send's own hop URL must be masked before that happens too, not only a
// successful one's.
func TestMaskSendErrTimeline_MasksFailedSendHopURL(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	sendErr := &httpclient.Error{
		Code: httpclient.CodeTransport, Message: "connect: connection refused",
		Timeline: &httpclient.Timeline{
			Hops: []httpclient.TimelineHop{
				{Index: 0, Method: "GET", URL: "https://api.example.com/orders?token=" + secret},
			},
		},
	}

	maskSendErrTimeline(sendErr, usedSecrets)

	got := sendErr.Timeline.Hops[0].URL
	if strings.Contains(got, secret) {
		t.Fatalf("Timeline.Hops[0].URL = %q still contains the raw secret", got)
	}
	if !strings.Contains(got, "{{apiKey}}") {
		t.Fatalf("Timeline.Hops[0].URL = %q, want it masked to {{apiKey}}", got)
	}
}

// TestMaskSendErrTimeline_MasksHopHeadersAndHopError is round-2 review finding 3:
// maskSendErrTimeline masked only Hops[i].URL, unlike maskSecrets' success-path twin which also
// masks Hops[i].Headers[j].Value (e.g. a Location header carrying a secret-bearing redirect URL).
// Separately, httpclient/timeline.go's finishFailed writes a second, independent copy of the
// resolved URL into Hops[i].Error via err.Error() — a field masking herr.Message alone never
// reaches. Both are rendered by TimelinePane.vue for a failed send (the failed-hop chip and the
// "Response headers" disclosure). Reproduces the shape D15 describes: a redirect hop that
// completed normally (carrying a secret-bearing Location header) followed by the hop the send
// actually died on (carrying the secret in its own Error string).
func TestMaskSendErrTimeline_MasksHopHeadersAndHopError(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	sendErr := &httpclient.Error{
		Code: httpclient.CodeTransport, Message: "connect: connection refused",
		Timeline: &httpclient.Timeline{
			Hops: []httpclient.TimelineHop{
				{
					Index: 0, Method: "GET",
					URL: "https://api.example.com/orders?token=" + secret, Status: 301,
					Headers: []httpclient.Header{
						{Name: "Location", Value: "https://api.example.com/orders/final?token=" + secret},
					},
				},
				{
					Index: 1, Method: "GET",
					URL:   "https://api.example.com/orders/final?token=" + secret,
					Error: `Get "https://api.example.com/orders/final?token=` + secret + `": dial tcp: connection refused`,
				},
			},
		},
	}

	maskSendErrTimeline(sendErr, usedSecrets)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, secret) {
			t.Errorf("%s = %q still contains the raw secret", label, s)
		}
		if !strings.Contains(s, "{{apiKey}}") {
			t.Errorf("%s = %q, want it masked to {{apiKey}}", label, s)
		}
	}
	assertMasked(t, "Hops[0].Headers[0].Value (Location)", sendErr.Timeline.Hops[0].Headers[0].Value)
	assertMasked(t, "Hops[1].Error", sendErr.Timeline.Hops[1].Error)

	// End to end through the copyable surface: mapHttpError marshals the whole Timeline into
	// ipcerr.Error.Details.
	mapped := mapHttpError(sendErr)
	var ie *ipcerr.Error
	if !errors.As(mapped, &ie) {
		t.Fatalf("mapHttpError(...) = %T, want *ipcerr.Error", mapped)
	}
	if strings.Contains(string(ie.Details), secret) {
		t.Fatalf("ipcerr.Error.Details still contains the raw secret:\n%s", ie.Details)
	}
}

// TestMaskSendErrTimeline_MasksMessageForTransportAndBadURLErrors: client.go's classifySendErr
// (transport failure) and resolveURL (bad-URL-parse failure) both build herr.Message from the
// *resolved* URL — net/http's own *url.Error and url.Parse's own error both embed the URL they
// were given verbatim — yet maskSendErrTimeline used to touch only herr.Timeline.Hops[].URL,
// never herr.Message itself. A bad-URL-parse failure in particular has no Timeline at all (it
// fails before classifySendErr ever runs), so that path went entirely unmasked. Both shapes are
// covered here since gRPC's own mapHttpError/RunOp.Error() reads herr.Message verbatim either way.
func TestMaskSendErrTimeline_MasksMessageForTransportAndBadURLErrors(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	cases := []struct {
		name string
		err  *httpclient.Error
	}{
		{
			name: "transport error (has a Timeline)",
			err: &httpclient.Error{
				Code:    httpclient.CodeTransport,
				Message: `Get "https://api.example.com/orders?token=` + secret + `": dial tcp: connection refused`,
				Timeline: &httpclient.Timeline{
					Hops: []httpclient.TimelineHop{{Index: 0, Method: "GET", URL: "https://api.example.com/orders?token=" + secret}},
				},
			},
		},
		{
			name: "bad-request URL-parse error (no Timeline yet)",
			err: &httpclient.Error{
				Code:    httpclient.CodeBadRequest,
				Message: `invalid URL: parse "https://api.example.com/orders?token=` + secret + `\x7f": net/url: invalid control character in URL`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			maskSendErrTimeline(tc.err, usedSecrets)
			if strings.Contains(tc.err.Message, secret) {
				t.Fatalf("Message = %q still contains the raw secret", tc.err.Message)
			}
			if !strings.Contains(tc.err.Message, "{{apiKey}}") {
				t.Fatalf("Message = %q, want it masked to {{apiKey}}", tc.err.Message)
			}

			// mapHttpError marshals herr.Message straight into ipcerr.Error.Message — the value
			// control.ts unwraps and the renderer can copy — so it must never carry the secret.
			mapped := mapHttpError(tc.err)
			if strings.Contains(mapped.Error(), secret) {
				t.Fatalf("mapHttpError(...).Error() = %q still contains the raw secret", mapped.Error())
			}
		})
	}
}

// TestHttpSendFailure_OpLogErrorNeverContainsSecret proves end to end, through the exact
// path a live failed send takes: RunOp's own err.Error() (host.go) is persisted verbatim to
// op_log.error by oplog's Wiring — reproduced here against a real Host and a real, migrated SQLite
// database (storage.Open(), the same helper the redirect/timeline masking test above uses) rather
// than calling maskSendErrTimeline in isolation, so a future refactor of RunOp's own error-handling
// would still be caught.
func TestHttpSendFailure_OpLogErrorNeverContainsSecret(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: secret, Placeholder: "{{apiKey}}"}}

	t.Setenv("KIRA_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	opsRepo := &repos.OpsRepo{DB: db.DB}
	host := adapterhost.NewHost(adapters.Deps{}, nil)
	wiring := oplog.New(host, opsRepo, 30)

	done := make(chan model.OpRecord, 4)
	unsubscribe := wiring.OnUpdate(func(rec model.OpRecord) {
		if rec.Status != "running" {
			done <- rec
		}
	})
	defer unsubscribe()

	wiring.Start()
	defer wiring.Stop()

	tabID := "tab1"
	spec := adapterhost.OpSpec{OpID: "op-http-fail", Kind: "http", TabID: &tabID}
	_, _, runErr := host.RunOp(context.Background(), spec,
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			// The exact shape bridge/http.go's Send closure produces for a transport failure whose
			// message embeds the resolved (secret-bearing) URL, masked exactly as that closure now
			// does before returning it to RunOp.
			sendErr := &httpclient.Error{
				Code:    httpclient.CodeTransport,
				Message: `Get "https://api.example.com/orders?token=` + secret + `": dial tcp: connection refused`,
				Timeline: &httpclient.Timeline{
					Hops: []httpclient.TimelineHop{{Index: 0, Method: "GET", URL: "https://api.example.com/orders?token=" + secret}},
				},
			}
			maskSendErrTimeline(sendErr, usedSecrets)
			return nil, sendErr
		})
	if runErr == nil {
		t.Fatal("expected RunOp to return the send error")
	}
	if strings.Contains(mapHttpError(runErr).Error(), secret) {
		t.Fatalf("mapHttpError(runErr).Error() still contains the raw secret")
	}

	select {
	case rec := <-done:
		if rec.Error == nil {
			t.Fatal("op_log row has no error recorded")
		}
		if strings.Contains(*rec.Error, secret) {
			t.Fatalf("op_log.error = %q still contains the raw secret", *rec.Error)
		}
		if !strings.Contains(*rec.Error, "{{apiKey}}") {
			t.Fatalf("op_log.error = %q, want it masked to {{apiKey}}", *rec.Error)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the op:end update")
	}

	var rawError string
	if err := db.DB.QueryRow(`SELECT error FROM op_log WHERE id = ?`, "op-http-fail").Scan(&rawError); err != nil {
		t.Fatalf("query op_log.error: %v", err)
	}
	if strings.Contains(rawError, secret) {
		t.Fatalf("stored op_log.error column contains the raw secret: %q", rawError)
	}
}

// TestMaskSecrets_MasksURLEncodedBodyDespiteQueryEscape: apivars.Replacer used to build
// its strings.Replacer over each secret's own *plaintext*, but a urlencoded body's rendered wire
// text (resp.Wire.Request) is the *url.QueryEscape'd* form buildURLEncoded/encodeURLEncodedFields
// produced — url.QueryEscape rewrites space, '+', '@' (among others), so a secret value
// containing any of them survived masking in cleartext, right next to the pane's own "N secret
// values shown as {{name}}" claim. Driven through a real httpclient.Send (a real httptest server),
// not a hand-built Response, so a future change to how the wire text is rendered would still be
// caught here.
func TestMaskSecrets_MasksURLEncodedBodyDespiteQueryEscape(t *testing.T) {
	// Contains a space, a '+', and an '@' — three of the characters url.QueryEscape rewrites,
	// exactly the ones the finding calls out.
	const secretName = "apiKey"
	const secretValue = "p@ss word+token"
	usedSecrets := []apivars.UsedSecret{{Name: secretName, Rendered: secretValue, Placeholder: "{{" + secretName + "}}"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := httpclient.Send(context.Background(), httpclient.Request{
		Method: "POST",
		URL:    srv.URL + "/x",
		Body: httpclient.Body{
			Mode:       "urlencoded",
			URLEncoded: []httpclient.Field{{Name: "token", Value: secretValue}},
		},
	}, httpclient.Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Wire == nil {
		t.Fatal("resp.Wire is nil — nothing to mask")
	}
	// Confirms the fixture actually exercises the bug this test guards: the encoded form must
	// differ from the plaintext, and both must be present in the unmasked wire text.
	encoded := url.QueryEscape(secretValue)
	if encoded == secretValue {
		t.Fatalf("QueryEscape(%q) = %q did not change — this fixture no longer exercises the bug", secretValue, encoded)
	}
	if !strings.Contains(resp.Wire.Request, encoded) {
		t.Fatalf("unmasked wire request does not contain the encoded secret %q at all:\n%s", encoded, resp.Wire.Request)
	}

	maskSecrets(&resp, usedSecrets)

	if strings.Contains(resp.Wire.Request, secretValue) {
		t.Fatalf("masked wire request still contains the raw secret:\n%s", resp.Wire.Request)
	}
	if strings.Contains(resp.Wire.Request, encoded) {
		t.Fatalf("masked wire request still contains the QueryEscape'd secret %q:\n%s", encoded, resp.Wire.Request)
	}
	if !strings.Contains(resp.Wire.Request, "{{apiKey}}") {
		t.Fatalf("masked wire request does not contain the masked placeholder:\n%s", resp.Wire.Request)
	}
}

// TestMaskSecrets_MasksPathEscapedSecret: url.QueryEscape's own form of a secret is a second
// replacer pair, but a secret used in a URL *path* segment (e.g.
// https://api.example.com/{{secret}}/orders) is percent-encoded via url.PathEscape/EscapedPath()
// instead, which differs from QueryEscape for several characters —
// a space becomes %20 via PathEscape but + via QueryEscape. That form wasn't in the replacer and
// leaked in FinalURL, Timeline.Hops[].URL and Wire.Request's request line. Driven through a real
// httpclient.Send with the secret embedded in a path segment, not a hand-built Response, so a
// future change to how the URL is rendered would still be caught here.
func TestMaskSecrets_MasksPathEscapedSecret(t *testing.T) {
	// Contains a space — PathEscape rewrites it to %20, QueryEscape to '+', so this value's
	// path-escaped form is not already covered by the QueryEscape pair.
	const secretName = "apiKey"
	const secretValue = "p@ss word+token"
	usedSecrets := []apivars.UsedSecret{{Name: secretName, Rendered: secretValue, Placeholder: "{{" + secretName + "}}"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := httpclient.Send(context.Background(), httpclient.Request{
		Method: "GET",
		URL:    srv.URL + "/" + secretValue + "/orders",
	}, httpclient.Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Wire == nil {
		t.Fatal("resp.Wire is nil — nothing to mask")
	}
	// Confirms the fixture actually exercises the bug: the path-escaped form must differ from both
	// the plaintext and the QueryEscape'd form, and it must be present in the unmasked timeline.
	pathEscaped := url.PathEscape(secretValue)
	queryEscaped := url.QueryEscape(secretValue)
	if pathEscaped == secretValue || pathEscaped == queryEscaped {
		t.Fatalf("PathEscape(%q) = %q did not distinguish itself from the plaintext/QueryEscape form — this fixture no longer exercises the bug", secretValue, pathEscaped)
	}
	if len(resp.Timeline.Hops) == 0 || !strings.Contains(resp.Timeline.Hops[0].URL, pathEscaped) {
		t.Fatalf("unmasked Timeline.Hops[0].URL does not contain the path-escaped secret %q at all: %+v", pathEscaped, resp.Timeline.Hops)
	}

	maskSecrets(&resp, usedSecrets)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, secretValue) {
			t.Errorf("%s = %q still contains the raw secret", label, s)
		}
		if strings.Contains(s, pathEscaped) {
			t.Errorf("%s = %q still contains the PathEscape'd secret %q", label, s, pathEscaped)
		}
		if !strings.Contains(s, "{{apiKey}}") {
			t.Errorf("%s = %q, want it masked to {{apiKey}}", label, s)
		}
	}
	assertMasked(t, "FinalURL", resp.FinalURL)
	assertMasked(t, "Timeline.Hops[0].URL", resp.Timeline.Hops[0].URL)
	assertMasked(t, "Wire.Request", resp.Wire.Request)
}

// TestMaskSecrets_MasksPipedSecretsBase64Form is the worked example: a secret piped
// through `{{token | base64}}` never puts its plaintext on the wire at all — `UsedSecret.Rendered`
// is the base64 form apivars.Resolver wrote — so the masking replacer must be built over
// *that* rendered text, not the secret's plaintext (which would never match anything and leave
// the base64-encoded credential sitting in every copyable surface, right next to the pane's own
// "N secret values shown as {{name}}" claim). Every surface maskSecrets exists to cover is
// exercised, mirroring TestMaskSecrets_RedirectURLsFinalURLAndTimelineHopsBeforePersisting's own
// surface list.
func TestMaskSecrets_MasksPipedSecretsBase64Form(t *testing.T) {
	const plaintext = "sk_live_super_secret_token"
	rendered := base64.StdEncoding.EncodeToString([]byte(plaintext))
	usedSecrets := []apivars.UsedSecret{{Name: "apiKey", Rendered: rendered, Placeholder: "{{apiKey | base64}}"}}

	resp := httpclient.Response{
		Status: 200, StatusText: "OK", Proto: "HTTP/1.1",
		FinalURL: "https://api.example.com/orders?token=" + rendered,
		Wire:     &httpclient.WireExchange{Request: "Authorization: Bearer " + rendered + "\r\n"},
		Timeline: httpclient.Timeline{
			Hops: []httpclient.TimelineHop{{Index: 0, Method: "GET", URL: "https://api.example.com/orders?token=" + rendered}},
		},
	}

	maskSecrets(&resp, usedSecrets)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, plaintext) {
			t.Errorf("%s = %q still contains the raw plaintext secret", label, s)
		}
		if strings.Contains(s, rendered) {
			t.Errorf("%s = %q still contains the unmasked base64 form", label, s)
		}
		if !strings.Contains(s, "{{apiKey | base64}}") {
			t.Errorf("%s = %q, want it masked to {{apiKey | base64}}", label, s)
		}
	}
	assertMasked(t, "FinalURL", resp.FinalURL)
	assertMasked(t, "Wire.Request", resp.Wire.Request)
	assertMasked(t, "Timeline.Hops[0].URL", resp.Timeline.Hops[0].URL)
	if resp.Wire.MaskedSecrets != 1 {
		t.Errorf("Wire.MaskedSecrets = %d, want 1", resp.Wire.MaskedSecrets)
	}
}

// TestMaskSecrets_MasksBothPlainAndPipedFormsOfOneSecret is the case: a request using
// a secret both plainly (`{{token}}`) and piped (`{{token | base64}}`) must mask BOTH wire forms —
// the exact case a name-keyed masking model could not represent at all, since it can only ever
// record one value per secret name.
func TestMaskSecrets_MasksBothPlainAndPipedFormsOfOneSecret(t *testing.T) {
	const plaintext = "sk_live_super_secret_token"
	b64 := base64.StdEncoding.EncodeToString([]byte(plaintext))
	usedSecrets := []apivars.UsedSecret{
		{Name: "token", Rendered: plaintext, Placeholder: "{{token}}"},
		{Name: "token", Rendered: b64, Placeholder: "{{token | base64}}"},
	}

	resp := httpclient.Response{
		Status: 200, StatusText: "OK", Proto: "HTTP/1.1",
		Wire: &httpclient.WireExchange{
			Request: "Authorization: Bearer " + plaintext + "\r\nX-Signed: " + b64 + "\r\n",
		},
	}

	maskSecrets(&resp, usedSecrets)

	if strings.Contains(resp.Wire.Request, plaintext) {
		t.Errorf("Wire.Request = %q still contains the raw plaintext secret", resp.Wire.Request)
	}
	if strings.Contains(resp.Wire.Request, b64) {
		t.Errorf("Wire.Request = %q still contains the unmasked base64 form", resp.Wire.Request)
	}
	if !strings.Contains(resp.Wire.Request, "{{token}}") {
		t.Errorf("Wire.Request = %q, want the plain form masked to {{token}}", resp.Wire.Request)
	}
	if !strings.Contains(resp.Wire.Request, "{{token | base64}}") {
		t.Errorf("Wire.Request = %q, want the piped form masked to {{token | base64}}", resp.Wire.Request)
	}
	// D9(e): two spellings of one secret still count as one secret.
	if resp.Wire.MaskedSecrets != 1 {
		t.Errorf("Wire.MaskedSecrets = %d, want 1 (distinct names, not distinct entries)", resp.Wire.MaskedSecrets)
	}
}

// TestSecretReplacer_LongerRenderedFormIsNotShadowedByAShorterOne: entries must be
// sorted by len(Rendered) descending before the replacer is built, or a shorter rendered form
// that is a prefix of a longer one shadows it — strings.Replacer matches at each position by the
// earliest-listed pattern, not the longest, so listing the short one first would under-mask the
// long one, leaking its own tail in plaintext right next to a masked placeholder.
func TestSecretReplacer_LongerRenderedFormIsNotShadowedByAShorterOne(t *testing.T) {
	// "AB" is a prefix of "ABCDEF" — listed in *ascending* length order here, on purpose, so the
	// test actually exercises Replacer's own sort rather than happening to pass because the
	// caller already listed them safely.
	usedSecrets := []apivars.UsedSecret{
		{Name: "short", Rendered: "AB", Placeholder: "{{short}}"},
		{Name: "long", Rendered: "ABCDEF", Placeholder: "{{long}}"},
	}
	resp := httpclient.Response{
		Wire: &httpclient.WireExchange{Request: "token=ABCDEF"},
	}

	maskSecrets(&resp, usedSecrets)

	if resp.Wire.Request != "token={{long}}" {
		t.Fatalf("Wire.Request = %q, want %q (the longer rendered form must win, not be shadowed by the shorter one's prefix match)", resp.Wire.Request, "token={{long}}")
	}
}
