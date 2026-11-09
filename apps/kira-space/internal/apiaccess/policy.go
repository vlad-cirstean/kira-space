package apiaccess

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	maxRules   = 50
	maxHosts   = 20
	anyToken   = "*"
	effectDeny = "deny"
	effectOK   = "allow"
)

// Methods are the HTTP methods the api_request tool accepts.
var Methods = []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}

// Rule is one stored API access rule. Hosts hold "*" or HostRule entries without a scheme; Methods hold "*"
// or distinct members of Methods.
type Rule struct {
	Effect  string   `json:"effect"`
	Hosts   []string `json:"hosts"`
	Methods []string `json:"methods"`
}

type compiledRule struct {
	rule    Rule
	deny    bool
	anyHost bool
	hosts   []HostRule
	anyVerb bool
}

// Policy is a validated rule list. The zero value is empty and allows everything.
type Policy struct {
	rules []compiledRule
}

// Empty reports whether the policy has no rules, which allows every request.
func (p Policy) Empty() bool { return len(p.rules) == 0 }

// Compile validates rules and prepares them for matching. Errors name the 1-based rule and the bad value.
func Compile(rules []Rule) (Policy, error) {
	if len(rules) > maxRules {
		return Policy{}, fmt.Errorf("at most %d rules, got %d", maxRules, len(rules))
	}
	var p Policy
	for i, r := range rules {
		c, err := compileRule(r)
		if err != nil {
			return Policy{}, fmt.Errorf("rule %d: %w", i+1, err)
		}
		p.rules = append(p.rules, c)
	}
	return p, nil
}

func compileRule(r Rule) (compiledRule, error) {
	c := compiledRule{rule: r}
	switch r.Effect {
	case effectOK:
	case effectDeny:
		c.deny = true
	default:
		return c, fmt.Errorf("effect %q: must be allow or deny", r.Effect)
	}
	if len(r.Hosts) < 1 || len(r.Hosts) > maxHosts {
		return c, fmt.Errorf("needs 1 to %d hosts, got %d", maxHosts, len(r.Hosts))
	}
	for _, h := range r.Hosts {
		if h == anyToken {
			c.anyHost = true
			continue
		}
		hr, err := ParseHostRule(h)
		if err != nil || hr.HTTP {
			return c, fmt.Errorf("host %q: not a host, host:port, *.domain or * entry", h)
		}
		c.hosts = append(c.hosts, hr)
	}
	if len(r.Methods) == 1 && r.Methods[0] == anyToken {
		c.anyVerb = true
		return c, nil
	}
	if len(r.Methods) < 1 || len(r.Methods) > len(Methods) {
		return c, fmt.Errorf("needs * or 1 to %d methods, got %d", len(Methods), len(r.Methods))
	}
	seen := map[string]bool{}
	for _, m := range r.Methods {
		if !slices.Contains(Methods, m) {
			return c, fmt.Errorf("method %q: must be * alone or one of %s", m, strings.Join(Methods, ", "))
		}
		if seen[m] {
			return c, fmt.Errorf("method %q: listed twice", m)
		}
		seen[m] = true
	}
	return c, nil
}

func (c compiledRule) matches(method string, u *url.URL) bool {
	if !c.anyVerb && !slices.Contains(c.rule.Methods, method) {
		return false
	}
	if c.anyHost {
		return true
	}
	for _, h := range c.hosts {
		if h.MatchesHost(u) {
			return true
		}
	}
	return false
}

func (c compiledRule) verbs() string {
	if c.anyVerb {
		return "any method"
	}
	return strings.Join(c.rule.Methods, ", ")
}

func (c compiledRule) hostList() string { return strings.Join(c.rule.Hosts, ", ") }

// String is the rule as shown to the agent, for example "deny DELETE on *".
func (c compiledRule) String() string {
	verbs := c.verbs()
	return c.rule.Effect + " " + verbs + " on " + c.hostList()
}

// Check decides one request. A deny match refuses; otherwise a non-empty policy needs an allow match. The error
// names the method and scheme://host only, never path or query.
func (p Policy) Check(method string, u *url.URL) error {
	if p.Empty() {
		return nil
	}
	target := method + " " + u.Scheme + "://" + u.Host
	allowed := false
	for _, c := range p.rules {
		if !c.matches(method, u) {
			continue
		}
		if c.deny {
			return fmt.Errorf("%s is denied by the API access rule %q", target, c.String())
		}
		allowed = true
	}
	if allowed {
		return nil
	}
	return errors.New(target + " is not allowed by the API access rules in Kira Space's database MCP settings. " +
		p.allowedText() + ". Ask the user to add a rule")
}

func (p Policy) allowedText() string {
	var parts []string
	for _, c := range p.rules {
		if !c.deny {
			parts = append(parts, c.verbs()+" on "+c.hostList())
		}
	}
	if len(parts) == 0 {
		return "Allowed: nothing, every rule is a deny"
	}
	return "Allowed: " + strings.Join(parts, "; ")
}
