package agentapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

const (
	// httpclientMaxMb caps what the HTTP client reads, whatever the user's own setting allows.
	httpclientMaxMb  = 5
	httpclientMaxHop = 10
)

// ErrPlanChanged is Send's refusal when a variable changed between approval and send.
var ErrPlanChanged = errors.New("environment changed while waiting for approval; send again")

// Send re-plans in against the policy as it stands now, so a host removed or an opt-in cleared while
// the request waited for approval blocks it, then sends it. A non-nil approved is the plan the user saw:
// the request must still plan to the same method, URL, headers, query, body and form. The result is
// masked and capped.
func (s *Service) Send(ctx context.Context, in Input, approved *Planned) (Result, error) {
	p, err := s.plan(in, true)
	if err != nil {
		return Result{}, err
	}
	if p.MissingInputs() > 0 {
		return Result{}, errInputMissing
	}
	if approved != nil && !p.sameRequest(*approved) {
		return Result{}, ErrPlanChanged
	}
	// The values this plan read are the ones sent: an id that moved means a new value.
	if approved != nil {
		if err := p.checkKeptCurrent(*approved); err != nil {
			return Result{}, err
		}
	}
	// An allow rule removed since the plan would turn an unprompted send into one that needs a prompt.
	if approved != nil && approved.AllowedByRule && !p.AllowedByRule {
		return Result{}, ErrPlanChanged
	}
	g, err := s.Globals()
	if err != nil {
		slog.Warn("agent api: read settings", "scope", "agentapi", "err", err)
		return Result{}, errors.New("the API settings could not be read")
	}
	opened, err := s.Secrets.OpenAll()
	if err != nil {
		slog.Warn("agent api: open secrets", "scope", "agentapi", "err", err)
		return Result{}, errors.New("the saved secrets could not be read")
	}
	req, used, err := buildRequest(p, opened)
	if err != nil {
		return Result{}, err
	}
	m := newMask(opened, used, s.keptSecrets(), p.secretKeptTexts())
	return s.run(ctx, p, req, optionsFor(p, g, m), m)
}

// buildRequest resolves the secrets into the request. Only this function holds plaintext.
func buildRequest(p Planned, opened *apifiles.Opened) (httpclient.Request, []apivars.UsedSecret, error) {
	refs := make(map[string]string, len(p.creds))
	for _, c := range p.creds {
		refs[c.name] = c.secretName
	}
	r := apivars.NewResolverFor(refs, opened.Source())
	kr := p.keptResolver()
	var keptErr error
	field := func(text string, mode insertMode, lit func(string) string) string {
		out, err := kr.text(text, mode, lit)
		if err != nil && keptErr == nil {
			keptErr = err
		}
		return out
	}
	target := field(p.base, insertURL, r.URLText)
	extra := make([]NameValue, len(p.extra))
	for i, e := range p.extra {
		extra[i] = NameValue{Name: field(e.Name, insertScalar, r.Text), Value: field(e.Value, insertScalar, r.Text)}
	}
	req := httpclient.Request{Method: p.Method, URL: withQuery(target, extra, false)}
	for _, h := range p.Headers {
		req.Headers = append(req.Headers, httpclient.Header{Name: field(h.Name, insertScalar, r.Text), Value: field(h.Value, insertScalar, r.Text)})
	}
	if p.ContentType != "" {
		req.Headers = append(req.Headers, httpclient.Header{Name: "Content-Type", Value: field(p.ContentType, insertScalar, r.Text)})
	}
	switch {
	case p.Body != "":
		mode := insertRaw
		if isJSONType(requestContentType(p.ContentType, p.Headers)) {
			mode = insertJSON
		}
		req.Body = httpclient.Body{Mode: string(httpclient.BodyRaw), Raw: field(p.Body, mode, r.Text)}
	case len(p.Form) > 0:
		fields := make([]httpclient.Field, len(p.Form))
		for i, f := range p.Form {
			fields[i] = httpclient.Field{Name: field(f.Name, insertScalar, r.Text), Value: field(f.Value, insertScalar, r.Text)}
		}
		req.Body = httpclient.Body{Mode: string(httpclient.BodyURLEncoded), URLEncoded: fields}
	}
	if keptErr == nil && kr != nil {
		keptErr = checkFinal(req)
	}
	if keptErr != nil {
		return req, nil, keptErr
	}
	if err := r.Err(); err != nil {
		return req, nil, err
	}
	if err := sameTarget(p, req.URL); err != nil {
		return req, nil, err
	}
	return req, r.Used(), nil
}

// sameTarget is a last check that substituting a secret left the request on the planned host.
func sameTarget(p Planned, final string) error {
	u, err := url.Parse(final)
	if err != nil || u.Hostname() != p.Host {
		return errors.New("the request target changed while resolving credentials")
	}
	return nil
}

func optionsFor(p Planned, g Globals, m mask) httpclient.Options {
	timeoutMs := p.TimeoutSeconds * 1000
	maxMb := min(max(g.MaxResponseMb, 1), httpclientMaxMb)
	maxHops := min(max(g.MaxRedirects, 0), httpclientMaxHop)
	verify := true
	follow := p.FollowRedirects
	origin, _ := url.Parse(p.URL) // nil on a parse failure: no same-host allowance
	userOrigin := ""
	if p.UsesUserValue() {
		userOrigin = p.Origin
	}
	opts := httpclient.Options{
		RequestTimeoutMs: &timeoutMs, MaxResponseMb: &maxMb, SSLVerify: &verify,
		FollowRedirects: &follow, MaxRedirects: &maxHops, Ephemeral: true,
		WireMask: m.repl, RedirectGuard: redirectGuard(p.Method, p.creds, p.access, origin, !p.AllowedByRule, userOrigin),
	}
	if g.HTTPVersion != "" {
		opts.HTTPVersion = &g.HTTPVersion
	}
	return opts
}

func (s *Service) run(ctx context.Context, p Planned, req httpclient.Request, opts httpclient.Options, m mask) (Result, error) {
	_, value, err := s.Ops.RunOp(ctx, adapterhost.OpSpec{Kind: "http"}, func(runCtx context.Context, op *adapters.OpCtx) (any, error) {
		// The command is the template URL: secrets stay {{name}}, never a resolved value.
		op.SetCommand(m.text(p.Method + " " + p.URL))
		resp, err := httpclient.Send(runCtx, req, opts)
		if err != nil {
			return nil, errors.New(m.text(err.Error()))
		}
		op.SetCommand(m.text(fmt.Sprintf("%s %s → %d %s", p.Method, p.URL, resp.Status, resp.StatusText)))
		return resp, nil
	})
	if err != nil {
		return Result{}, errors.New(m.text(err.Error()))
	}
	resp, _ := value.(httpclient.Response)
	res := maskResult(resp, m, cmp.Or(p.bodyCap, p.MaxResponseBytes))
	res.Mask = m.text
	return res, nil
}

// mask removes secrets from agent-bound text: first the rendered forms of the secrets the request
// used become their {{name}}, then any plaintext of any Api secret becomes the mask.
type mask struct {
	repl   *strings.Replacer
	all    *secrets.Masker
	maxLen int
}

// keptSecrets is every live secret kept value of every scope: a response may echo a value another
// session supplied, so the mask does not stop at the values this request used.
func (s *Service) keptSecrets() []keptvars.Entry {
	if s.Kept == nil {
		return nil
	}
	return s.Kept.Secrets()
}

// newMask builds the request's masker over the Api secrets, the secret kept entries and extra raw texts.
// One replacer holds every pattern, longest first, so the httpclient wire capture masks with it too.
func newMask(opened *apifiles.Opened, used []apivars.UsedSecret, kept []keptvars.Entry, extra []string) mask {
	values := append(append(opened.Values(), keptvars.SecretRaw(kept)...), extra...)
	m := mask{repl: mergedReplacer(apivars.ReplacerPairs(used), keptvars.SecretPairs(kept)), all: secrets.NewMasker(values...)}
	for _, v := range values {
		m.maxLen = max(m.maxLen, len(v))
	}
	return m
}

// mergedReplacer joins pattern lists (old, new pairs) into one replacer, longest pattern first; nil when empty.
func mergedReplacer(lists ...[]string) *strings.Replacer {
	type pair struct{ old, new string }
	var ps []pair
	for _, l := range lists {
		for i := 0; i+1 < len(l); i += 2 {
			ps = append(ps, pair{l[i], l[i+1]})
		}
	}
	if len(ps) == 0 {
		return nil
	}
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].old) > len(ps[j].old) })
	args := make([]string, 0, 2*len(ps))
	for _, p := range ps {
		args = append(args, p.old, p.new)
	}
	return strings.NewReplacer(args...)
}

func (m mask) text(s string) string {
	if m.repl != nil {
		s = m.repl.Replace(s)
	}
	return m.all.Mask(s)
}

// maskResult masks every agent-visible field and applies the response cap.
func maskResult(resp httpclient.Response, m mask, capBytes int) Result {
	r := Result{
		Status: resp.Status, StatusText: m.text(resp.StatusText), BodyBytes: resp.BodyBytes, ElapsedMs: resp.ElapsedMs,
		FinalURL: m.text(resp.FinalURL), Headers: []NameValue{}, Redirects: []Redirect{},
	}
	for _, h := range resp.Headers {
		r.Headers = append(r.Headers, NameValue{Name: h.Name, Value: m.text(h.Value)})
		if strings.EqualFold(h.Name, "Content-Type") {
			r.ContentType = m.text(h.Value)
		}
	}
	for _, hop := range resp.Redirects {
		r.Redirects = append(r.Redirects, Redirect{Status: hop.Status, URL: m.text(hop.URL)})
	}
	if resp.BodyEncoding == "base64" {
		r.Binary, r.Truncated = true, resp.BodyTruncated
		return r
	}
	body := resp.Body
	if resp.BodyTruncated {
		// A secret cut by the client's own cap would leave a prefix the masker cannot match.
		body = body[:max(len(body)-6*m.maxLen, 0)]
	}
	r.Body, r.Truncated = capText(m.text(body), capBytes)
	r.Truncated = r.Truncated || resp.BodyTruncated
	return r
}

// capText cuts s to at most n bytes on a rune boundary.
func capText(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
}
