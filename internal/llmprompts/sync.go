package llmprompts

import (
	"encoding/json"
	"fmt"
	"time"
)

// DecodeOverrides returns the override texts in an overrides file, by prompt id. Empty data is no
// overrides.
func DecodeOverrides(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(data) == 0 {
		return out, nil
	}
	var d document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if d.Version != fileVersion {
		return nil, fmt.Errorf("unsupported version %d", d.Version)
	}
	for id, r := range d.Overrides {
		if r != nil {
			out[string(id)] = r.Text
		}
	}
	return out, nil
}

// EncodeOverrides renders texts as an overrides file built on base (the current file, may be empty).
// A record whose text is unchanged stays as it is; a new or changed text gets a fresh stamp and
// default hash; an id missing from texts drops. The revision rises when anything changed.
func EncodeOverrides(base []byte, texts map[string]string, now time.Time) ([]byte, error) {
	cur := document{Version: fileVersion, Overrides: map[ID]*record{}}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &cur); err != nil {
			return nil, err
		}
		if cur.Version != fileVersion {
			return nil, fmt.Errorf("unsupported version %d", cur.Version)
		}
		if cur.Overrides == nil {
			cur.Overrides = map[ID]*record{}
		}
	}
	next := make(map[ID]*record, len(texts))
	dirty := len(texts) != len(cur.Overrides)
	for sid, text := range texts {
		id := ID(sid)
		e := byID[id]
		if e == nil {
			return nil, fmt.Errorf("unknown prompt %q", sid)
		}
		if r := cur.Overrides[id]; r != nil && r.Text == text {
			next[id] = r
			continue
		}
		next[id] = &record{Text: text, DefaultHash: Hash(e.Default), UpdatedAt: now.UnixMilli()}
		dirty = true
	}
	rev := cur.Revision
	if dirty {
		rev++
	}
	return json.MarshalIndent(document{Version: fileVersion, Revision: rev, Overrides: next}, "", "  ")
}

// ValidateOverrides returns the first finding for texts taken together as overrides, "" when they
// all pass. The message never echoes a prompt text.
func ValidateOverrides(texts map[string]string) string {
	changed := make(map[ID]string, len(texts))
	for sid, text := range texts {
		id := ID(sid)
		if byID[id] == nil {
			return problem(id, "backend.llmPrompts.unknown.body", map[string]string{"name": sid}).Message
		}
		if ps := validateText(id, text, defaultText); len(ps) > 0 {
			return ps[0].Message
		}
		changed[id] = text
	}
	if ps := validateChanges(changed, defaultText); len(ps) > 0 {
		return ps[0].Message
	}
	return ""
}
