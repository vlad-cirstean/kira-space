package agentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"golang.org/x/net/http/httpguts"
)

// maxKeptNames bounds the distinct kept variables one request may use.
const maxKeptNames = 16

// Limits on the request once kept values are inserted.
const (
	maxFinalURLBytes  = 64 << 10
	maxFinalBodyBytes = 8 << 20
)

// KeptReader is the store view the service plans against; *keptvars.Store satisfies it.
type KeptReader interface {
	Get(scope, name string) (keptvars.Entry, bool)
	// List is the live entries of a scope (room checks for inputs).
	List(scope string) []keptvars.Meta
	// Secrets is the live secret entries of every scope, for response masks.
	Secrets() []keptvars.Entry
}

// KeptSite is one {{kept.<name>}} span in a field of the planned request. From and To are UTF-16 code
// units in that field's text as the plan carries it.
type KeptSite struct {
	Field string `json:"field"` // url, header, query, form, body or contentType
	Index int    `json:"index"` // pair index for header, query and form
	Part  string `json:"part"`  // name or value for pairs
	From  int    `json:"from"`
	To    int    `json:"to"`
}

// KeptUse is a kept variable a request uses, as it stood at plan time. The value is not part of it: only a
// preview, empty for a secret.
type KeptUse struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Items  int             `json:"items"`
	Bytes  int             `json:"bytes"`
	Secret bool            `json:"secret"`
	Source keptvars.Source `json:"source"`
	// Origin is the origin the value is bound to (see keptvars.Meta.Bound); empty for an unbound value. Not sent to the UI.
	Origin           string     `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	Preview          string     `json:"preview"`
	PreviewTruncated bool       `json:"previewTruncated"`
	Sites            []KeptSite `json:"sites"`
	// Pending marks a use whose value the user has yet to supply: ID is empty.
	Pending bool `json:"pending,omitempty"`
}

// Bound reports whether the use is a user value or one derived from it.
func (u KeptUse) Bound() bool { return u.Source.Kind == keptvars.SourceUser || u.Origin != "" }

// keptValue is a value held for one send only.
type keptValue struct {
	raw    []byte
	typ    string
	secret bool
}

// keptText is one request field that can carry a reference.
type keptText struct {
	field string
	index int
	part  string
	text  string
}

// keptTexts lists every field of a request that may carry a reference, in a stable order.
func keptTexts(urlText string, headers, query []NameValue, body string, form []NameValue, contentType string) []keptText {
	out := []keptText{{field: "url", text: urlText}}
	for _, g := range []struct {
		field string
		list  []NameValue
	}{{"header", headers}, {"query", query}} {
		out = appendPairs(out, g.field, g.list)
	}
	out = append(out, keptText{field: "body", text: body})
	out = appendPairs(out, "form", form)
	return append(out, keptText{field: "contentType", text: contentType})
}

func appendPairs(out []keptText, field string, list []NameValue) []keptText {
	for i, nv := range list {
		out = append(out, keptText{field, i, "name", nv.Name}, keptText{field, i, "value", nv.Value})
	}
	return out
}

func inputKeptTexts(in Input) []keptText {
	return keptTexts(in.URL, in.Headers, in.Query, in.Body, in.Form, in.ContentType)
}

// keptPlan is what the plan found out about the kept variables a request uses.
type keptPlan struct {
	order   []string
	entries map[string]keptvars.Entry
	// pending are the names of needed inputs: their entries are stand-ins.
	pending map[string]bool
	inputs  []InputUse
}

func keptErr(format string, args ...any) error { return fmt.Errorf(format, args...) }

// lookupKept finds the entry a name refers to: the request's one-time overlay first, then the store. once says
// the entry came from the overlay.
func (s *Service) lookupKept(in Input, name string) (e keptvars.Entry, ok, once bool) {
	if e, ok := in.Once[name]; ok {
		return e, true, true
	}
	e, ok = s.lookupStore(in.Scope, name)
	return e, ok, false
}

func (s *Service) lookupStore(scope, name string) (keptvars.Entry, bool) {
	if s.Kept == nil || scope == "" || keptvars.ValidName(name) != nil {
		return keptvars.Entry{}, false
	}
	return s.Kept.Get(scope, name)
}

// admit records the entry a reference names: a live entry, or the stand-in of a needed input.
func (s *Service) admit(kp *keptPlan, in Input, name string, decls map[string]InputDecl, origin string) error {
	e, ok, once := s.lookupKept(in, name)
	if d, declared := decls[name]; declared {
		use, err := judgeInput(d, e, ok, once, origin)
		if err != nil {
			return err
		}
		kp.inputs = append(kp.inputs, use)
		if !ok {
			e = pendingEntry(name, origin)
			kp.pending[name] = true
		}
	} else if !ok {
		return keptErr("no kept variable %q (it may have expired or been cleared); call list_kept_variables"+inputsDeclHint, name)
	} else if e.Bound() && origin != "" && e.Origin != origin {
		if e.Source.Kind != keptvars.SourceUser {
			return keptErr("the secret kept as %q belongs to %s and is not sent to %s", name, e.Origin, origin)
		}
		return otherOriginError(name, e.Origin, origin)
	}
	kp.entries[name] = e
	kp.order = append(kp.order, name)
	return nil
}

// admitUnreferenced judges the declared inputs the request text does not reference: each must be read by a
// script, else the declaration is dead.
func (s *Service) admitUnreferenced(kp *keptPlan, in Input, decls map[string]InputDecl, origin string) error {
	done := map[string]bool{}
	for _, u := range kp.inputs {
		done[u.Name] = true
	}
	for _, d := range in.Inputs {
		if done[d.Name] {
			continue
		}
		e, ok, once := s.lookupKept(in, d.Name)
		use, err := judgeInput(decls[d.Name], e, ok, once, origin)
		if err != nil {
			return err
		}
		if !slices.Contains(in.ScriptReads, d.Name) {
			return keptErr("input %q is declared but not used; write {{kept.%s}} where the value goes", d.Name, d.Name)
		}
		kp.inputs = append(kp.inputs, use)
	}
	// Declaration order, whatever order the references came in.
	slices.SortStableFunc(kp.inputs, func(a, b InputUse) int {
		return slices.IndexFunc(in.Inputs, func(d InputDecl) bool { return d.Name == a.Name }) -
			slices.IndexFunc(in.Inputs, func(d InputDecl) bool { return d.Name == b.Name })
	})
	return nil
}

// checkKept refuses a reference that cannot be used and returns the entries the request reads.
func (s *Service) checkKept(in Input) (*keptPlan, error) {
	decls, err := checkDecls(in.Inputs)
	if err != nil {
		return nil, err
	}
	origin := originOfText(in.URL)
	kp := &keptPlan{entries: map[string]keptvars.Entry{}, pending: map[string]bool{}}
	texts := inputKeptTexts(in)
	for _, t := range texts {
		for _, sp := range keptvars.RefSpans(t.text) {
			if sp.Pipeline {
				return nil, keptErr("kept variables take no transforms; write {{kept.%s}}", sp.Name)
			}
			if _, seen := kp.entries[sp.Name]; seen {
				continue
			}
			if err := s.admit(kp, in, sp.Name, decls, origin); err != nil {
				return nil, err
			}
		}
	}
	if err := s.admitUnreferenced(kp, in, decls, origin); err != nil {
		return nil, err
	}
	if err := s.checkRoom(in.Scope, len(kp.pending)+missingUnreferenced(kp)); err != nil {
		return nil, err
	}
	if len(kp.order) == 0 {
		if len(kp.inputs) == 0 {
			return nil, nil
		}
		return kp, nil
	}
	if len(kp.order) > maxKeptNames {
		return nil, errors.New("a request can use at most 16 kept variables")
	}
	isJSON := isJSONType(requestContentType(in.ContentType, in.Headers))
	for _, t := range texts {
		if err := kp.checkField(t, isJSON); err != nil {
			return nil, err
		}
	}
	return kp, nil
}

// missingUnreferenced counts needed inputs that only a script reads (they are not in pending).
func missingUnreferenced(kp *keptPlan) int {
	n := 0
	for _, u := range kp.inputs {
		if u.State == InputNeeded && !kp.pending[u.Name] {
			n++
		}
	}
	return n
}

func requestContentType(contentType string, headers []NameValue) string {
	if contentType != "" {
		return contentType
	}
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			return h.Value
		}
	}
	return ""
}

func isJSONType(ct string) bool { return strings.Contains(strings.ToLower(ct), "json") }

func (kp *keptPlan) checkField(t keptText, isJSON bool) error {
	spans := keptvars.RefSpans(t.text)
	if len(spans) == 0 {
		return nil
	}
	if t.field == "body" {
		return kp.checkBody(t.text, spans, isJSON)
	}
	if t.field == "url" {
		if end := authorityEnd(t.text); spans[0].From < end {
			return errors.New("a kept variable cannot be part of the URL's scheme, host or port")
		}
	}
	for _, sp := range spans {
		if err := kp.scalarOK(sp.Name); err != nil {
			return err
		}
	}
	if t.field != "header" {
		return nil
	}
	out, err := kp.expand(t.text, insertScalar, func(s string) string { return s })
	if err != nil {
		return err
	}
	switch {
	case strings.ContainsAny(out, "\r\n"):
		return keptErr("kept variable %q contains a line break; it cannot go in a header", spans[0].Name)
	case t.part == "name" && !httpguts.ValidHeaderFieldName(out), t.part == "value" && !httpguts.ValidHeaderFieldValue(out):
		return keptErr("kept variable %q makes the header %s invalid", spans[0].Name, t.part)
	}
	return nil
}

// authorityEnd is the byte offset where the URL's scheme, host and port end: the first /, ? or # after
// the scheme separator.
func authorityEnd(u string) int {
	start := 0
	if i := strings.Index(u, "://"); i >= 0 {
		start = i + 3
	}
	if i := strings.IndexAny(u[start:], "/?#"); i >= 0 {
		return start + i
	}
	return len(u)
}

func (kp *keptPlan) scalarOK(name string) error {
	switch kp.entries[name].Type {
	case "array":
		return keptErr("kept variable %q is a list; only text, number or boolean values fit in a URL, header, query or form field. Use it in a JSON body, or keep a joined string", name)
	case "object":
		return keptErr("kept variable %q is an object; only text, number or boolean values fit in a URL, header, query or form field. Use it in a JSON body, or keep a joined string", name)
	}
	return nil
}

func (kp *keptPlan) checkBody(text string, spans []keptvars.RefSpan, isJSON bool) error {
	if !isJSON {
		return nil
	}
	for _, sp := range spans {
		if insideJSONString(text, sp.From) {
			return errors.New("in a JSON body write {{kept." + sp.Name + "}} without quotes; Kira inserts the value as JSON")
		}
	}
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		b.WriteString(text[last:sp.From])
		b.WriteString("null")
		last = sp.To
	}
	b.WriteString(text[last:])
	if !json.Valid([]byte(b.String())) {
		return errors.New("body is not valid JSON once kept variables are inserted")
	}
	return nil
}

// insideJSONString reports whether byte offset at of text falls inside a JSON string literal.
func insideJSONString(text string, at int) bool {
	in := false
	for i := 0; i < at && i < len(text); i++ {
		switch c := text[i]; {
		case in && c == '\\':
			i++
		case c == '"':
			in = !in
		}
	}
	return in
}

type insertMode int

const (
	insertURL    insertMode = iota // path before ?, query after
	insertScalar                   // text in a header, query, form or content type
	insertJSON                     // compact JSON, any type
	insertRaw                      // body that is not JSON
)

// expand replaces each kept span of text with its value for mode. Text between spans goes through lit
// (the secret resolver); inserted values are never scanned again.
func (kp *keptPlan) expand(text string, mode insertMode, lit func(string) string) (string, error) {
	return expandWith(text, mode, lit, func(name string) (keptValue, bool) {
		e, ok := kp.entries[name]
		return keptValue{raw: e.Value, typ: e.Type, secret: e.Secret}, ok
	})
}

func expandWith(text string, mode insertMode, lit func(string) string, value func(string) (keptValue, bool)) (string, error) {
	spans := keptvars.RefSpans(text)
	if len(spans) == 0 {
		return lit(text), nil
	}
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		b.WriteString(lit(text[last:sp.From]))
		v, ok := value(sp.Name)
		if !ok {
			return "", keptErr("kept variable %q is not available", sp.Name)
		}
		piece, err := insertion(v, mode, strings.Contains(text[:sp.From], "?"), sp.Name)
		if err != nil {
			return "", err
		}
		b.WriteString(piece)
		last = sp.To
	}
	b.WriteString(lit(text[last:]))
	return b.String(), nil
}

func insertion(v keptValue, mode insertMode, afterQuery bool, name string) (string, error) {
	switch mode {
	case insertJSON:
		return string(v.raw), nil
	case insertRaw:
		if v.typ == "string" {
			return scalarText(v), nil
		}
		return string(v.raw), nil
	}
	if v.typ == "array" || v.typ == "object" {
		return "", keptErr("kept variable %q is a %s; it cannot go in a URL, header, query or form field", name, v.typ)
	}
	text := scalarText(v)
	if mode == insertURL {
		if afterQuery {
			return url.QueryEscape(text), nil
		}
		return url.PathEscape(text), nil
	}
	return text, nil
}

// scalarText is a string as is, a number or boolean as its JSON literal, null as empty.
func scalarText(v keptValue) string {
	switch v.typ {
	case "string":
		var s string
		_ = json.Unmarshal(v.raw, &s)
		return s
	case "null":
		return ""
	}
	return string(v.raw)
}

// apply records the uses on p, with sites computed on the text the plan carries. With values it also holds
// the values for this send.
func (kp *keptPlan) apply(p *Planned, withValues bool) {
	if kp == nil {
		return
	}
	p.Inputs = kp.inputs
	texts := keptTexts(p.URL, p.Headers, p.Query, p.Body, p.Form, p.ContentType)
	sites := map[string][]KeptSite{}
	for _, t := range texts {
		for _, sp := range keptvars.RefSpans(t.text) {
			sites[sp.Name] = append(sites[sp.Name], KeptSite{
				Field: t.field, Index: t.index, Part: t.part,
				From: keptvars.ToUTF16(t.text, sp.From), To: keptvars.ToUTF16(t.text, sp.To),
			})
		}
	}
	if withValues {
		p.keptValues = map[string]keptValue{}
	}
	for _, name := range kp.order {
		e := kp.entries[name]
		use := NewKeptUse(e, sites[name])
		use.Pending = kp.pending[name]
		p.Kept = append(p.Kept, use)
		if withValues && !use.Pending {
			p.keptValues[name] = keptValue{raw: e.Value, typ: e.Type, secret: e.Secret}
		}
	}
}

// checkKeptCurrent refuses a send whose kept variables changed since approved was planned.
func (p Planned) checkKeptCurrent(approved Planned) error {
	now := map[string]string{}
	for _, k := range p.Kept {
		now[k.Name] = k.ID
	}
	for _, k := range approved.Kept {
		if now[k.Name] != k.ID {
			return keptErr("kept variable %q changed while waiting for approval; nothing was sent. Produce it again, then repeat the request.", k.Name)
		}
	}
	return nil
}

// secretKeptTexts are the forms of the secret kept values a request used, for the response mask.
func (p Planned) secretKeptTexts() []string {
	var out []string
	for _, v := range p.keptValues {
		if v.secret {
			out = append(out, scalarText(v), string(v.raw))
			out = append(out, keptvars.SecretTexts(keptvars.Entry{Meta: keptvars.Meta{Type: v.typ}, Value: v.raw})...)
		}
	}
	return out
}

// keptResolver inserts the kept values into the fields of a request being built. Nil when it uses none.
type keptResolver struct{ values map[string]keptValue }

func (p Planned) keptResolver() *keptResolver {
	if len(p.keptValues) == 0 {
		return nil
	}
	return &keptResolver{values: p.keptValues}
}

func (k *keptResolver) text(text string, mode insertMode, lit func(string) string) (string, error) {
	if k == nil {
		return lit(text), nil
	}
	return expandWith(text, mode, lit, func(name string) (keptValue, bool) {
		v, ok := k.values[name]
		return v, ok
	})
}

// checkFinal refuses a request whose inserted kept values broke a header or grew it past a limit.
func checkFinal(req httpclient.Request) error {
	if len(req.URL) > maxFinalURLBytes {
		return errors.New("the url is longer than 64 KiB once kept variables are inserted")
	}
	if len(req.Body.Raw) > maxFinalBodyBytes {
		return errors.New("the body is larger than 8 MiB once kept variables are inserted")
	}
	for _, h := range req.Headers {
		if !httpguts.ValidHeaderFieldName(h.Name) || !httpguts.ValidHeaderFieldValue(h.Value) || len(h.Value) > maxHeaderBytes {
			return errors.New("a kept variable made a header invalid")
		}
	}
	return nil
}
