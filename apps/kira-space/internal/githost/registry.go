package githost

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

// Setting values besides a provider id.
const (
	SettingAuto = "auto" // detect from the origin remote's host
	SettingOff  = "off"
)

// Registry holds the registered providers in registration order.
type Registry struct {
	byID map[string]Provider
	ids  []string
}

// NewRegistry registers ps in order. A duplicate ID panics.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{byID: make(map[string]Provider, len(ps))}
	for _, p := range ps {
		if _, dup := r.byID[p.ID()]; dup {
			panic(fmt.Sprintf("githost: duplicate provider %q", p.ID()))
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

// Labels joins the providers' labels with " or ", in registration order.
func (r *Registry) Labels() string {
	all := r.All()
	labels := make([]string, 0, len(all))
	for _, p := range all {
		labels = append(labels, p.Terms().Label)
	}
	return strings.Join(labels, " or ")
}

// ChangeRequestLabels joins "<label> <change request noun>" per provider with " or ", in
// registration order: "GitHub pull request".
func (r *Registry) ChangeRequestLabels() string {
	all := r.All()
	labels := make([]string, 0, len(all))
	for _, p := range all {
		t := p.Terms()
		labels = append(labels, t.Label+" "+t.ChangeRequestNoun)
	}
	return strings.Join(labels, " or ")
}

// LinkLabels joins "<label> <short> or issue link" per provider with " or ", in registration order:
// "GitHub PR or issue link".
func (r *Registry) LinkLabels() string {
	all := r.All()
	labels := make([]string, 0, len(all))
	for _, p := range all {
		t := p.Terms()
		labels = append(labels, t.Label+" "+t.ChangeRequestShort+" or issue link")
	}
	return strings.Join(labels, " or ")
}

// Infos describes every provider in registration order.
func (r *Registry) Infos() []Info {
	all := r.All()
	out := make([]Info, 0, len(all))
	for _, p := range all {
		out = append(out, Describe(p))
	}
	return out
}

// ParseLink asks each provider in order.
func (r *Registry) ParseLink(raw string) (Link, bool) {
	for _, p := range r.All() {
		if l, ok := p.ParseLink(raw); ok {
			return l, true
		}
	}
	return Link{}, false
}

// ParseBranchLink asks each provider in order that resolves branch links.
func (r *Registry) ParseBranchLink(ctx context.Context, raw string) (Provider, BranchLinks, BranchLink, bool) {
	for _, p := range r.All() {
		bl, ok := p.(BranchLinks)
		if !ok {
			continue
		}
		if l, parsed := bl.ParseBranchLink(ctx, raw); parsed {
			return p, bl, l, true
		}
	}
	return nil, nil, BranchLink{}, false
}

// BranchLinkLabels joins "<label> <short> or branch link" per branch-link provider with " or ", in
// registration order: "GitHub PR or branch link".
func (r *Registry) BranchLinkLabels() string {
	var labels []string
	for _, p := range r.All() {
		if _, ok := p.(BranchLinks); !ok {
			continue
		}
		t := p.Terms()
		labels = append(labels, t.Label+" "+t.ChangeRequestShort+" or branch link")
	}
	return strings.Join(labels, " or ")
}

// ParseChangeRequestLink asks each provider in order that recognises change request links.
func (r *Registry) ParseChangeRequestLink(ctx context.Context, raw string) (Provider, Repo, int, bool) {
	for _, p := range r.All() {
		cl, ok := p.(ChangeRequestLinks)
		if !ok {
			continue
		}
		if repo, n, parsed := cl.ParseChangeRequestLink(ctx, raw); parsed {
			return p, repo, n, true
		}
	}
	return nil, Repo{}, 0, false
}

// ChangeRequestViewLabels joins "<label> <short> link" per detail provider with " or ", in
// registration order: "GitHub PR link or Azure Repos PR link".
func (r *Registry) ChangeRequestViewLabels() string {
	var labels []string
	for _, p := range r.All() {
		if _, ok := p.(ChangeRequestDetails); !ok {
			continue
		}
		t := p.Terms()
		labels = append(labels, t.Label+" "+t.ChangeRequestShort+" link")
	}
	return strings.Join(labels, " or ")
}

// IsChangeRequestURL is true when any provider accepts u.
func (r *Registry) IsChangeRequestURL(ctx context.Context, u *url.URL) bool {
	for _, p := range r.All() {
		if p.IsChangeRequestURL(ctx, u) {
			return true
		}
	}
	return false
}

// Selector picks the host for one repository.
type Selector struct{ Registry *Registry }

// Resolve picks the host for one repository. off yields none. A registered id yields that provider
// when it parses remoteURL. auto, "" or an unknown id (warned) yields the first provider, in
// registration order, that parses remoteURL and claims its host.
func (s Selector) Resolve(ctx context.Context, setting, remoteURL string) (Provider, Repo, bool) {
	if s.Registry == nil || setting == SettingOff {
		return nil, Repo{}, false
	}
	if setting != "" && setting != SettingAuto {
		if p, ok := s.Registry.Get(setting); ok {
			repo, parsed := p.ParseRemote(remoteURL)
			return p, repo, parsed
		}
		slog.Warn("githost: unknown provider, detecting from remote", "scope", "githost", "provider", setting)
	}
	for _, p := range s.Registry.All() {
		if repo, ok := p.ParseRemote(remoteURL); ok && p.Claims(ctx, repo.Host) {
			return p, repo, true
		}
	}
	return nil, Repo{}, false
}
