package apivars

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ReferenceKind classifies one {{name}} reference D17's grammar found.
type ReferenceKind string

const (
	KindResolved ReferenceKind = "resolved"
	KindDeferred ReferenceKind = "deferred"
	KindDynamic  ReferenceKind = "dynamic"
	KindUnknown  ReferenceKind = "unknown"
)

// IsDynamicReference is the Go twin of packages/api-core/src/http/substitute.ts's own
// isDynamicReference — a name is dynamic-shaped if it is `$`-prefixed (Postman's own
// spelling) or `fake.`-prefixed (this app's additive, permanent alias namespace, never a
// migration of the former).
func IsDynamicReference(name string) bool {
	return strings.HasPrefix(name, "$") || strings.HasPrefix(name, "fake.")
}

// Reference is one {{name}} reference Resolve found, and how it was classified.
type Reference struct {
	Name string
	Kind ReferenceKind
	// Pipeline is the transform names, left to right — nil for today's references (P17 D4), which
	// is what keeps the shared corpus (testdata/substitution.json) comparing field-by-field with no
	// edit for every case that predates the pipe grammar.
	Pipeline []string
}

// ParsedReference is ParseReference's answer: the bare reference name and its optional transform
// pipeline, split out of the already-extracted, already-trimmed text between `{{` and `}}`.
// The TS twin is packages/api-core/src/http/substitute.ts's own parseReference — same rules,
// pinned by the same corpus.
type ParsedReference struct {
	// Name is the bare reference name — what classifyReference, Names, and every downstream
	// consumer (the reveal loop, the masking replacer) key on. Unchanged from today for a
	// reference with no pipeline.
	Name string
	// Pipeline is the transform names, left to right. Empty for today's references.
	Pipeline []string
	// Normalized is the span text `{{name | a | b}}`, one space either side of each `|`,
	// regardless of how it was typed — D9's masking placeholder, and nothing else.
	Normalized string
}

// ParseReference splits inner — the already-extracted, already-trimmed text between `{{` and
// `}}` — into a bare name and an optional transform pipeline, by the same all-or-nothing
// rule the TS twin implements:
//
//  1. No `|` at all: today's behaviour exactly — {Name: inner, Pipeline: nil}.
//  2. Otherwise split on `|` and trim each segment. If the first segment is non-empty and every
//     segment after it is a known transform (IsTransformName), the parse succeeds.
//  3. Otherwise the whole of inner is the name, exactly as today, pipeline empty — the
//     backward-compatibility rule: a variable literally named `a|b` keeps resolving, and a
//     typo'd `{{token | base46}}` becomes an unknown reference named `token | base46` rather than
//     a half-parsed pipeline.
func ParseReference(inner string) ParsedReference {
	if !strings.Contains(inner, "|") {
		return ParsedReference{Name: inner, Normalized: "{{" + inner + "}}"}
	}
	rawSegments := strings.Split(inner, "|")
	segments := make([]string, len(rawSegments))
	for i, s := range rawSegments {
		segments[i] = strings.TrimSpace(s)
	}
	name := segments[0]
	rest := segments[1:]
	allTransforms := true
	for _, seg := range rest {
		if !IsTransformName(seg) {
			allTransforms = false
			break
		}
	}
	if name != "" && allTransforms {
		normalized := "{{" + strings.Join(append([]string{name}, rest...), " | ") + "}}"
		return ParsedReference{Name: name, Pipeline: rest, Normalized: normalized}
	}
	return ParsedReference{Name: inner, Normalized: "{{" + inner + "}}"}
}

// SubstitutionResult is Resolve's answer: the text with every resolvable reference substituted,
// and the classified list of every reference it found.
type SubstitutionResult struct {
	Text string
	Refs []Reference
}

// Resolve is the Go twin of packages/api-core/src/http/substitute.ts's own `resolve` — the identical
// two-token grammar (D17), pinned to the same behaviour by testdata/substitution.json (D18).
//
// The grammar, in full: scan for `{{`; from there scan for the next `}}`; no `}}` ⇒ the rest of
// the string is literal and the scan ends. The name is the text between, trimmed; an empty name
// is not a reference. Nesting is not a thing — `{{a{{b}}}}` takes `a{{b` as the name, finds
// nothing, and passes through literally. One pass only, within this one call: a resolved *plain*
// value that itself contains `{{other}}` is never re-expanded by this call (the shared corpus's
// own "a resolved value carrying {{x}} is not re-expanded" case).
//
// F6: that "one pass" guarantee is per call, not end to end. ResolveRequest (stage 2, below) runs
// a second Resolve-shaped pass over this function's own combined output, so a plain value that
// itself contains a verbatim `{{secretName}}` span — typed or pasted — is resolved there, on
// purpose: this is what lets a header like `auth = Bearer {{apiKey}}` work at all. This call
// still reports that nested name as a KindDeferred Reference (only when the outer reference had
// no pipeline, since a pipeline would consume the raw text before stage 2 ever sees it), so a
// caller deciding what to reveal from refs alone does not disagree with what stage 2 actually
// resolves.
func Resolve(text string, values map[string]string, secretNames []string) SubstitutionResult {
	return resolveWithSanitizer(text, values, secretNames, nil, nil)
}

// urlUnsafeReplacer is the Go twin of packages/api-core/src/http/substitute.ts's own
// sanitizeUrlSpan — percent-encodes only the characters that would
// otherwise break url.Parse's RawQuery or the request line itself (a space is what turns
// a send into a 400; &/#/= are the query string's own structural delimiters), leaving `{{`/`}}`
// and the reference name's ordinary characters exactly as typed. `|` joins the set — it
// is not a legal URL character.
var urlUnsafeReplacer = strings.NewReplacer(
	" ", "%20", "\t", "%09", "\r", "%0D", "\n", "%0A",
	"&", "%26", "#", "%23", "=", "%3D", "|", "%7C",
)

// resolveWithSanitizer is Resolve's real body — sanitize, when non-nil, is applied to a reference
// span left literal because it will never be resolved by anyone downstream: KindUnknown (no such
// name — including a secret whose decrypt failed, since Resolver.Text always calls this with
// secretNames nil, so a still-missing secret name falls into this same branch) and KindDynamic (Go
// never generates a `{{$name}}` value itself — P6's generator is JS-only). Deliberately never
// applied to KindDeferred: a later pass elsewhere still has to find that span by its exact,
// untouched name and pipeline.
//
// P17 D6/D9: onResolved, when non-nil, is called exactly once per KindResolved reference with the
// *rendered* text this function actually wrote (the pipeline already applied) and the
// *normalized* placeholder it should be masked back to (ParseReference's own Normalized —
// `{{name}}` or `{{name | base64}}`, one space either side of `|` regardless of how it was
// typed). It exists so Resolver.text (below) can record what reached the wire without ever
// putting that text on a Reference, which is reported outward. D8: dynamic's generator is still
// invoked once per occurrence, at this same point in the walk — the pipeline wraps only its
// return value.
func resolveWithSanitizer(text string, values map[string]string, secretNames []string, sanitize func(string) string, onResolved func(ref Reference, rendered, placeholder string)) SubstitutionResult {
	secrets := make(map[string]bool, len(secretNames))
	for _, n := range secretNames {
		secrets[n] = true
	}
	sanitizeOrVerbatim := func(span string) string {
		if sanitize != nil {
			return sanitize(span)
		}
		return span
	}

	refs := []Reference{}
	var out strings.Builder
	i := 0
	for i < len(text) {
		open := strings.Index(text[i:], "{{")
		if open == -1 {
			out.WriteString(text[i:])
			break
		}
		open += i
		closeAt := strings.Index(text[open+2:], "}}")
		if closeAt == -1 {
			out.WriteString(text[i:])
			break
		}
		closeAt += open + 2
		out.WriteString(text[i:open])
		inner := strings.TrimSpace(text[open+2 : closeAt])
		span := text[open : closeAt+2]
		i = closeAt + 2

		if inner == "" {
			out.WriteString(span)
			continue
		}
		parsed := ParseReference(inner)
		written, spanRefs := resolveOneSpan(parsed, span, values, secrets, secretNames, sanitizeOrVerbatim, onResolved)
		refs = append(refs, spanRefs...)
		out.WriteString(written)
	}
	return SubstitutionResult{Text: out.String(), Refs: refs}
}

// resolveOneSpan classifies and resolves one already-parsed `{{...}}` occurrence — the per-span
// body resolveWithSanitizer's own walk delegates to, pulled out so the walk itself stays under the
// repo's gocognit threshold. Returns the text to write in the span's place and every Reference the
// span produces (itself, plus — for a resolved plain value — the nested-deferred-secret reports
// from nestedDeferredSecretRefs).
func resolveOneSpan(
	parsed ParsedReference,
	span string,
	values map[string]string,
	secrets map[string]bool,
	secretNames []string,
	sanitizeOrVerbatim func(string) string,
	onResolved func(ref Reference, rendered, placeholder string),
) (string, []Reference) {
	name := parsed.Name
	pipeline := parsed.Pipeline

	switch {
	case IsDynamicReference(name):
		// D6 dynamic row: Go never generates a `{{$name}}`/`{{fake.x.y}}` value itself (P6's
		// generator is JS-only) — so a dynamic reference here is always left verbatim, with its
		// own pipeline untouched, exactly as stage 1's own uncatalogued-generator branch does.
		ref := Reference{Name: name, Kind: KindDynamic, Pipeline: pipeline}
		return sanitizeOrVerbatim(span), []Reference{ref}
	case secrets[name]:
		// Never sanitized, never transformed here: stage 2 (Resolver.text below) is the pass
		// that finds this exact span, decrypts the secret and applies its pipeline.
		ref := Reference{Name: name, Kind: KindDeferred, Pipeline: pipeline}
		return span, []Reference{ref}
	default:
		return resolvePlainOrUnknown(parsed, span, values, secrets, secretNames, sanitizeOrVerbatim, onResolved)
	}
}

// resolvePlainOrUnknown handles resolveOneSpan's default case: a name that names neither a dynamic
// generator nor a secret, so it is looked up in values and either resolved or classified unknown.
func resolvePlainOrUnknown(
	parsed ParsedReference,
	span string,
	values map[string]string,
	secrets map[string]bool,
	secretNames []string,
	sanitizeOrVerbatim func(string) string,
	onResolved func(ref Reference, rendered, placeholder string),
) (string, []Reference) {
	name := parsed.Name
	pipeline := parsed.Pipeline

	value, ok := values[name]
	if !ok {
		ref := Reference{Name: name, Kind: KindUnknown, Pipeline: pipeline}
		return sanitizeOrVerbatim(span), []Reference{ref}
	}

	rendered, applyOk := ApplyPipeline(pipeline, value)
	if !applyOk {
		// D5: a transform that cannot be applied leaves the entire span verbatim and classifies
		// the reference unknown.
		ref := Reference{Name: name, Kind: KindUnknown, Pipeline: pipeline}
		return sanitizeOrVerbatim(span), []Reference{ref}
	}

	ref := Reference{Name: name, Kind: KindResolved, Pipeline: pipeline}
	if onResolved != nil {
		onResolved(ref, rendered, parsed.Normalized)
	}
	refs := append([]Reference{ref}, nestedDeferredSecretRefs(value, pipeline, secrets, secretNames)...)
	return rendered, refs
}

// nestedDeferredSecretRefs: a plain value can itself contain a `{{secretName}}` span
// verbatim (typed or pasted, e.g. an "auth" variable whose value is "Bearer {{apiKey}}"). Stage 2
// (ResolveRequest below) rescans this function's own combined output and resolves such a span
// there — a real second pass across the pipeline as a whole, even though a single
// resolveWithSanitizer call never re-expands its own output (the "one pass only" line on Resolve
// above is about that one call). A caller that decides what to reveal from refs alone (curl.ts's
// Copy-as-curl dialog) must be told about that name too, or its preview disagrees with what Send
// sends. Only meaningful with no pipeline: a transform on the outer reference consumes
// the raw text, so any nested `{{...}}` syntax would already be mangled before stage 2 ever sees
// it. Classification-only — the recursive call is made with a nil values map so nothing here is
// itself substituted; a genuinely nested *plain* reference still stays unknown, matching the
// one-pass contract.
func nestedDeferredSecretRefs(value string, pipeline []string, secrets map[string]bool, secretNames []string) []Reference {
	if len(pipeline) != 0 || len(secrets) == 0 || !strings.Contains(value, "{{") {
		return nil
	}
	nested := resolveWithSanitizer(value, nil, secretNames, nil, nil)
	var out []Reference
	for _, nref := range nested.Refs {
		if nref.Kind == KindDeferred {
			out = append(out, nref)
		}
	}
	return out
}

// Names returns every distinct {{name}} reference in text, in first-seen order — the same scan
// with no lookup and no classification. ResolveRequest uses it to decide, before ever querying a
// secret, whether a field references anything at all. The *bare* name (ParseReference's
// own) — a pre-send "does this field reference anything" check counts `{{token | base64}}` as
// referencing `token`.
func Names(text string) []string {
	seen := map[string]bool{}
	out := []string{}
	i := 0
	for i < len(text) {
		open := strings.Index(text[i:], "{{")
		if open == -1 {
			break
		}
		open += i
		closeAt := strings.Index(text[open+2:], "}}")
		if closeAt == -1 {
			break
		}
		closeAt += open + 2
		inner := strings.TrimSpace(text[open+2 : closeAt])
		i = closeAt + 2
		if inner == "" {
			continue
		}
		name := ParseReference(inner).Name
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// D7's exact field list, restated here as the fields ResolveRequest walks — everything stage 1
// (the renderer) already resolved every non-secret reference in, so any reference still present
// here either names a secret or names nothing at all.
func referencedFields(url string, headers []httpclient.Header, body httpclient.Body) []string {
	fields := []string{url}
	for _, h := range headers {
		fields = append(fields, h.Name, h.Value)
	}
	switch body.Mode {
	case string(httpclient.BodyRaw):
		fields = append(fields, body.Raw)
	case string(httpclient.BodyCode):
		fields = append(fields, body.Code)
	case string(httpclient.BodyURLEncoded):
		for _, f := range body.URLEncoded {
			fields = append(fields, f.Name, f.Value)
		}
	case string(httpclient.BodyFormData):
		for _, f := range body.FormData {
			fields = append(fields, f.Name, f.ContentType)
			if f.Kind == "text" {
				fields = append(fields, f.Value)
			}
			// D7: a form-data file row's path is never substituted, so it is not walked here.
		}
	}
	return fields
}

// Resolver is the reusable half of stage 2 — fetch every secret reachable from one scope once, then
// substitute text and accumulate what was used. Two protocols share it now (ResolveRequest below,
// and bridge/grpc.go's own resolution closure via NewResolver), and gain no import of each other's
// protocol package for it: apivars gains no gRPC import, which is what keeps it protocol-neutral
// rather than becoming a two-protocol module.
type Resolver struct {
	refs         map[string]string // secret variable name to secret ref
	src          configfs.SecretSource
	secretValues map[string]string
	used         []UsedSecret
	usedSeen     map[string]bool
	err          error
}

// UsedSecret is one substituted secret span: the text written into the request, and the
// placeholder it is masked back to. Rendered is a secret's plaintext ONLY
// when the span carried no pipeline — with one, it is the transformed form (base64, upper-cased,
// URL-encoded…), which is exactly as sensitive and is what a caller's masking replacer must
// register instead of the plaintext.
type UsedSecret struct {
	// Name is the secret's variable name — used for the Debug log (names only, D5) and for
	// counting distinct secrets (Wire.MaskedSecrets counts names, not entries, D9(e)).
	Name string
	// Rendered is the exact text this substitution wrote onto the wire — the plaintext when the
	// span had no pipeline, the transformed form otherwise. Comes from the substitution walk
	// itself (the onResolved callback), never re-derived, so the masking input and the wire
	// content share one source by construction.
	Rendered string
	// Placeholder is ParseReference's own Normalized span — "{{name}}" or
	// "{{name | base64}}" — what Rendered is masked back to. Normalized (single spaces) rather
	// than the user's exact spacing, so two spellings of one span do not produce two placeholders
	// for identical wire bytes.
	Placeholder string
}

// NewResolver names every secret variable reachable from collectionID/environmentID
// (repos.VariablesRepo.SecretRefsFor already applies D2's environment-over-collection precedence)
// and returns a resolver that substitutes text and accumulates what it actually used. A secret's
// value is opened only when a text references it.
func (s *Service) NewResolver(collectionID, environmentID string) (*Resolver, error) {
	refs, err := s.deps.Repo.SecretRefsFor(collectionID, environmentID)
	if err != nil {
		return nil, err
	}
	return &Resolver{refs: refs, src: s.deps.Secrets.Snapshot(), secretValues: map[string]string{}, usedSeen: map[string]bool{}}, nil
}

// NewResolverFor is NewResolver over an explicit variable-name to secret-name map and secret source,
// for a caller whose variables do not come from the api_variables rows.
func NewResolverFor(refs map[string]string, src configfs.SecretSource) *Resolver {
	return &Resolver{refs: refs, src: src, secretValues: map[string]string{}, usedSeen: map[string]bool{}}
}

// Any reports whether this scope has any secret at all — false means there is nothing this
// resolver could ever substitute, so a caller can skip walking its own fields entirely.
func (r *Resolver) Any() bool {
	return len(r.refs) > 0
}

// Err is the first secret a resolved text referenced that cannot be used: not set, or not readable.
// The send must fail with it; the reference is never left literal and never empty.
func (r *Resolver) Err() error { return r.err }

// Text resolves every {{name}} reference in text that names one of this scope's secrets, and
// records each one substituted into Used(). A secret that is unset or unreadable sets Err().
func (r *Resolver) Text(text string) string {
	return r.text(text, nil)
}

// URLText is Text's twin for the one field with query-string delimiter syntax to break: an
// unknown span is left verbatim, but must not inject a raw space/&/#/= into url.Parse's RawQuery
// or the request line itself.
func (r *Resolver) URLText(text string) string {
	return r.text(text, urlUnsafeReplacer.Replace)
}

// load opens the value of every referenced secret name once.
func (r *Resolver) load(text string) {
	for _, name := range Names(text) {
		ref, ok := r.refs[name]
		if !ok {
			continue
		}
		if _, done := r.secretValues[name]; done {
			continue
		}
		v, err := r.src.Secret(context.Background(), "", ref)
		switch {
		case err == nil:
			r.secretValues[name] = v
		case r.err != nil:
		case errors.Is(err, configfs.ErrSecretMissing):
			r.err = ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.apiFiles.secretUnset.body", map[string]string{"name": ref})
		default:
			slog.Warn("an api secret could not be read", "scope", "apivars", "name", ref, "err", err)
			r.err = ipcerr.T(ipcerr.CodeSecretStore, "backend.studio.apiFiles.secretUnreadable.body", map[string]string{"name": ref})
		}
	}
}

func (r *Resolver) text(text string, sanitize func(string) string) string {
	// The rendered text and its placeholder reach this resolver through
	// resolveWithSanitizer's own onResolved callback, called from inside the substitution walk —
	// never through a field on Reference, which is reported outward and must never carry a
	// secret's rendered form. One entry per distinct (Name, Placeholder) pair: a request
	// using both {{token}} and {{token | base64}} records two entries, so both wire forms get
	// masked — a name-keyed map could not represent this.
	onResolved := func(ref Reference, rendered, placeholder string) {
		key := ref.Name + "\x00" + placeholder
		if r.usedSeen[key] {
			return
		}
		r.usedSeen[key] = true
		r.used = append(r.used, UsedSecret{Name: ref.Name, Rendered: rendered, Placeholder: placeholder})
	}
	r.load(text)
	result := resolveWithSanitizer(text, r.secretValues, nil, sanitize, onResolved)
	return result.Text
}

// Used is every secret span substituted so far, one entry per distinct (Name,
// Placeholder) pair — what a caller's own masking replacer (apivars.Replacer,
// shared unchanged by bridge/grpc.go) is built from, so a secret's plaintext or any
// transformed form of it a pipe produced is never left in a copyable surface unmasked.
// nil when nothing was substituted, matching ResolveRequest's own pre-extraction contract.
func (r *Resolver) Used() []UsedSecret {
	if len(r.used) == 0 {
		return nil
	}
	return r.used
}

// ResolveRequest is the stage 2, called from bridge/http.go strictly *after* op.SetCommand
// — the URL/headers/body it is given must never feed back into anything logged or persisted.
// It opens every secret a field references, reachable from collectionID/environmentID
// (repos.VariablesRepo.SecretRefsFor already applies the environment-over-collection precedence),
// and resolves every remaining {{name}} reference across the same field list as stage 1. A
// reference that names no secret — a stale id, a typo — is left verbatim; a secret that is unset
// or unreadable fails the send, naming it.
//
// The fourth return, `used`, is every secret span substituted — Resolver.Used()'s own
// []UsedSecret, one entry per distinct (name, placeholder) pair. bridge/http.go builds a
// strings.Replacer from it and masks the rendered exchange back to each entry's Placeholder, so
// neither a secret's plaintext nor any transformed form of it a pipe produced is ever left in a
// copyable surface unmasked.
func (s *Service) ResolveRequest(
	url string, headers []httpclient.Header, body httpclient.Body, collectionID, environmentID string,
) (string, []httpclient.Header, httpclient.Body, []UsedSecret, error) {
	hasAnyReference := false
	for _, field := range referencedFields(url, headers, body) {
		if len(Names(field)) > 0 {
			hasAnyReference = true
			break
		}
	}
	if !hasAnyReference {
		return url, headers, body, nil, nil
	}

	resolver, err := s.NewResolver(collectionID, environmentID)
	if err != nil {
		return url, headers, body, nil, err
	}
	if !resolver.Any() {
		return url, headers, body, nil, nil
	}

	resolvedURL := resolver.URLText(url)
	resolvedHeaders := make([]httpclient.Header, len(headers))
	for i, h := range headers {
		resolvedHeaders[i] = httpclient.Header{Name: resolver.Text(h.Name), Value: resolver.Text(h.Value)}
	}

	resolvedBody := body
	switch body.Mode {
	case string(httpclient.BodyRaw):
		resolvedBody.Raw = resolver.Text(body.Raw)
	case string(httpclient.BodyCode):
		resolvedBody.Code = resolver.Text(body.Code)
	case string(httpclient.BodyURLEncoded):
		fields := make([]httpclient.Field, len(body.URLEncoded))
		for i, f := range body.URLEncoded {
			fields[i] = httpclient.Field{Name: resolver.Text(f.Name), Value: resolver.Text(f.Value)}
		}
		resolvedBody.URLEncoded = fields
	case string(httpclient.BodyFormData):
		fields := make([]httpclient.FormField, len(body.FormData))
		for i, f := range body.FormData {
			out := f
			out.Name = resolver.Text(f.Name)
			out.ContentType = resolver.Text(f.ContentType)
			if f.Kind == "text" {
				out.Value = resolver.Text(f.Value)
			}
			// D7: a form-data file row's path is never substituted.
			fields[i] = out
		}
		resolvedBody.FormData = fields
	}

	if err := resolver.Err(); err != nil {
		return url, headers, body, nil, err
	}
	used := resolver.Used()
	// D5: the count and the *names* of the secrets resolved (deduplicated — a secret used both
	// plainly and piped is still one name), never their values or rendered forms, and only at
	// Debug — connections.Service.Reveal's own "the subject, not the secret" precedent.
	if len(used) > 0 {
		seenNames := make(map[string]bool, len(used))
		names := make([]string, 0, len(used))
		for _, u := range used {
			if seenNames[u.Name] {
				continue
			}
			seenNames[u.Name] = true
			names = append(names, u.Name)
		}
		slog.Debug("resolved secret references for a send", "scope", "apivars", "count", len(names), "names", names)
	}

	return resolvedURL, resolvedHeaders, resolvedBody, used, nil
}
