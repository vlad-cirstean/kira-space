package issuetracker

import (
	"fmt"
)

// Registry holds the registered providers in registration order. A nil Registry is empty.
type Registry struct {
	byID map[string]Provider
	ids  []string
}

// NewRegistry registers ps in order. A duplicate ID panics.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{byID: make(map[string]Provider, len(ps))}
	for _, p := range ps {
		if _, dup := r.byID[p.ID()]; dup {
			panic(fmt.Sprintf("issuetracker: duplicate provider %q", p.ID()))
		}
		r.byID[p.ID()] = p
		r.ids = append(r.ids, p.ID())
	}
	return r
}

// Get returns the provider registered under id.
func (r *Registry) Get(id string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byID[id]
	return p, ok
}

// All lists providers in registration order.
func (r *Registry) All() []Provider {
	if r == nil {
		return nil
	}
	out := make([]Provider, 0, len(r.ids))
	for _, id := range r.ids {
		out = append(out, r.byID[id])
	}
	return out
}

// ParseRef asks each provider in order and returns the first match with its provider.
func (r *Registry) ParseRef(raw string) (Provider, Ref, bool) {
	for _, p := range r.All() {
		if ref, ok := p.ParseRef(raw); ok {
			return p, ref, true
		}
	}
	return nil, Ref{}, false
}

// Infos describes every provider in registration order.
func (r *Registry) Infos() []Info {
	all := r.All()
	out := make([]Info, 0, len(all))
	for _, p := range all {
		out = append(out, Info{ID: p.ID(), Terms: p.Terms(), Setup: p.Setup()})
	}
	return out
}
