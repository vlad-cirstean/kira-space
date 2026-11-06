package llmprompts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Bound is the bound-service surface shared by both apps; each embeds it.
type Bound struct {
	Store *Store
	// App limits List, Save and Preview to this app's entries; empty serves every entry.
	App App
	// Changed runs with the new revision after a save changed the overrides file.
	Changed func(revision uint64)
}

// VarView is one declared template variable.
type VarView struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Example  string `json:"example"`
}

// PromptView is one entry for Settings. Text is the editor's starting text: the stored override,
// valid or not, else the default; Hash is its hash, echoed back as a change's baseHash.
type PromptView struct {
	ID             ID        `json:"id"`
	Kind           Kind      `json:"kind"`
	Parent         ID        `json:"parent"`
	Field          string    `json:"field"`
	Name           string    `json:"name"`
	TitleKey       string    `json:"titleKey"`
	PurposeKey     string    `json:"purposeKey"`
	UsedByKey      string    `json:"usedByKey"`
	Applies        Applies   `json:"applies"`
	Vars           []VarView `json:"vars"`
	Fragments      []ID      `json:"fragments"`
	Schema         string    `json:"schema"`
	DefaultText    string    `json:"defaultText"`
	Text           string    `json:"text"`
	Hash           string    `json:"hash"`
	Overridden     bool      `json:"overridden"`
	Invalid        string    `json:"invalid"`
	DefaultChanged bool      `json:"defaultChanged"`
	UpdatedAt      int64     `json:"updatedAt"`
}

// PromptList is this app's entries plus the store state.
type PromptList struct {
	Prompts []PromptView `json:"prompts"`
	// StoreError is why the overrides file was ignored, "" when it is fine.
	StoreError string `json:"storeError"`
	Revision   uint64 `json:"revision"`
}

// SaveArgs is a batch of changes saved all or nothing.
type SaveArgs struct {
	Changes []Change `json:"changes"`
}

// PreviewArgs asks for a render. Text defaults to the effective text, Vars to the example values.
type PreviewArgs struct {
	ID   ID              `json:"id"`
	Text *string         `json:"text,omitempty"`
	Vars json.RawMessage `json:"vars,omitempty"`
}

// PromptPreview is a render: Errors when the text is invalid, else Parts.
type PromptPreview struct {
	Errors []Problem `json:"errors"`
	Parts  []Part    `json:"parts"`
}

// List returns this app's entries.
func (b *Bound) List() (PromptList, error) { return b.list(b.Store.Snapshot()), nil }

func (b *Bound) list(snap Snapshot) PromptList {
	out := PromptList{Prompts: []PromptView{}, Revision: snap.Revision}
	if snap.Error != "" {
		out.StoreError = i18n.T("backend.llmPrompts.unreadable.body", map[string]string{"cause": snap.Error})
	}
	for _, e := range registry {
		if !e.hasApp(b.App) {
			continue
		}
		st := snap.States[e.ID]
		out.Prompts = append(out.Prompts, PromptView{
			ID: e.ID, Kind: e.Kind, Parent: e.Parent, Field: e.Field, Name: e.Name,
			TitleKey: e.TitleKey, PurposeKey: e.PurposeKey, UsedByKey: e.UsedByKey, Applies: e.Applies,
			Vars: varViews(e), Fragments: fragmentsOf(e), Schema: e.Schema,
			DefaultText: e.Default, Text: st.Text, Hash: Hash(st.Text),
			Overridden: st.Overridden, Invalid: st.Invalid, DefaultChanged: st.DefaultChanged, UpdatedAt: st.UpdatedAt,
		})
	}
	return out
}

func fragmentsOf(e *entry) []ID {
	if e.RequiredFragments == nil {
		return []ID{}
	}
	return e.RequiredFragments
}

func varViews(e *entry) []VarView {
	out := make([]VarView, 0, len(e.fields))
	ex := reflect.ValueOf(e.example)
	for _, f := range e.fields {
		out = append(out, VarView{Name: f.name, Required: f.required, Example: exampleText(ex.FieldByName(f.name))})
	}
	return out
}

func exampleText(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool, reflect.Int, reflect.Int64:
		return fmt.Sprint(v.Interface())
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return ""
	}
	return string(b)
}

// Save applies the changes all or nothing and returns the new list.
func (b *Bound) Save(args SaveArgs) (PromptList, error) {
	for _, c := range args.Changes {
		if e := byID[c.ID]; e == nil || !e.hasApp(b.App) {
			return PromptList{}, problemsError([]Problem{problem(c.ID, "backend.llmPrompts.unknown.body", map[string]string{"name": string(c.ID)})})
		}
	}
	before := b.Store.Revision()
	snap, err := b.Store.Save(args.Changes)
	var pe *ProblemsError
	switch {
	case errors.Is(err, ErrConflict):
		return PromptList{}, ipcerr.T(ipcerr.CodeConflict, "backend.llmPrompts.conflict.body", nil)
	case errors.As(err, &pe):
		return PromptList{}, problemsError(pe.Problems)
	case err != nil:
		return PromptList{}, ipcerr.T(ipcerr.CodeInternal, "backend.llmPrompts.saveFailed.body", map[string]string{"cause": err.Error()})
	}
	if snap.Revision != before && b.Changed != nil {
		b.Changed(snap.Revision)
	}
	return b.list(snap), nil
}

func problemsError(ps []Problem) error {
	e := ipcerr.T(ipcerr.CodeBadRequest, "backend.llmPrompts.invalid.body", nil)
	if raw, err := json.Marshal(map[string][]Problem{"problems": ps}); err == nil {
		e.Details = raw
	}
	return e
}

// Preview validates and renders one entry, optionally with draft text and caller vars.
func (b *Bound) Preview(args PreviewArgs) (PromptPreview, error) {
	e := byID[args.ID]
	if e == nil || !e.hasApp(b.App) {
		return PromptPreview{}, ipcerr.T(ipcerr.CodeNotFound, "backend.llmPrompts.unknown.body", map[string]string{"name": string(args.ID)})
	}
	texts := b.Store.texts()
	text := texts(e.ID)
	if args.Text != nil {
		text = *args.Text
	}
	vars := e.example
	if len(args.Vars) > 0 {
		var err error
		if vars, err = decodeVars(e, args.Vars); err != nil {
			return PromptPreview{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.llmPrompts.badVars.body", map[string]string{"cause": err.Error()})
		}
	}
	out := PromptPreview{Errors: b.Store.Validate(e.ID, text), Parts: []Part{}}
	if out.Errors == nil {
		out.Errors = []Problem{}
	}
	if len(out.Errors) > 0 {
		return out, nil
	}
	draft := func(id ID) string {
		if id == e.ID {
			return text
		}
		return texts(id)
	}
	parts, err := renderPartsWith(e, draft, vars)
	if err != nil {
		out.Errors = append(out.Errors, templateProblem(e.ID, "backend.llmPrompts.render.body", err))
		return out, nil
	}
	out.Parts = parts
	return out, nil
}

func decodeVars(e *entry, raw json.RawMessage) (any, error) {
	v := reflect.New(e.varsType)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v.Interface()); err != nil {
		return nil, errors.New(strings.TrimPrefix(err.Error(), "json: "))
	}
	return v.Elem().Interface(), nil
}
