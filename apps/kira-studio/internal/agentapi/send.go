package agentapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/httpclient"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

const (
	// httpclientMaxMb caps what the HTTP client reads, whatever the user's own setting allows.
	httpclientMaxMb  = 5
	httpclientMaxHop = 10
)

// Send re-plans in against the policy as it stands now, so a host removed or an opt-in cleared while
// the request waited for approval blocks it, then sends it. The result is masked and capped.
func (s *Service) Send(ctx context.Context, in Input) (Result, error) {
	p, err := s.Plan(in)
	if err != nil {
		return Result{}, err
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
	m := newMask(opened, used)
	return s.run(ctx, p, req, optionsFor(p, g, m), m)
}

// buildRequest resolves the secrets into the request. Only this function holds plaintext.
func buildRequest(p Planned, opened *apifiles.Opened) (httpclient.Request, []apivars.UsedSecret, error) {
	refs := make(map[string]string, len(p.creds))
	for _, c := range p.creds {
		refs[c.name] = c.secretName
	}
	r := apivars.NewResolverFor(refs, opened.Source())
	target := r.URLText(p.base)
	extra := make([]NameValue, len(p.extra))
	for i, e := range p.extra {
		extra[i] = NameValue{Name: r.Text(e.Name), Value: r.Text(e.Value)}
	}
	req := httpclient.Request{Method: p.Method, URL: withQuery(target, extra, false)}
	for _, h := range p.Headers {
		req.Headers = append(req.Headers, httpclient.Header{Name: r.Text(h.Name), Value: r.Text(h.Value)})
	}
	if p.ContentType != "" {
		req.Headers = append(req.Headers, httpclient.Header{Name: "Content-Type", Value: r.Text(p.ContentType)})
	}
	switch {
	case p.Body != "":
		req.Body = httpclient.Body{Mode: string(httpclient.BodyRaw), Raw: r.Text(p.Body)}
	case len(p.Form) > 0:
		fields := make([]httpclient.Field, len(p.Form))
		for i, f := range p.Form {
			fields[i] = httpclient.Field{Name: r.Text(f.Name), Value: r.Text(f.Value)}
		}
		req.Body = httpclient.Body{Mode: string(httpclient.BodyURLEncoded), URLEncoded: fields}
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
	opts := httpclient.Options{
		RequestTimeoutMs: &timeoutMs, MaxResponseMb: &maxMb, SSLVerify: &verify,
		FollowRedirects: &follow, MaxRedirects: &maxHops, Ephemeral: true,
		WireMask: m.repl, RedirectGuard: redirectGuard(p.creds),
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
	return maskResult(resp, m, p.MaxResponseBytes), nil
}

// mask removes secrets from agent-bound text: first the rendered forms of the secrets the request
// used become their {{name}}, then any plaintext of any Api secret becomes the mask.
type mask struct {
	repl   *strings.Replacer
	all    *secrets.Masker
	maxLen int
}

func newMask(opened *apifiles.Opened, used []apivars.UsedSecret) mask {
	values := opened.Values()
	m := mask{repl: apivars.Replacer(used), all: secrets.NewMasker(values...)}
	for _, v := range values {
		m.maxLen = max(m.maxLen, len(v))
	}
	return m
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
