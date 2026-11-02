package agentport

import (
	"strconv"
	"strings"
)

// nativeServer is a provider's MCP server in Kira's shape, values still native text.
type nativeServer struct {
	URL      string
	Headers  map[string]string
	Command  string
	Args     []string
	Env      map[string]string
	Disabled bool
}

// addServer adds the item of one native MCP server, or a skipped item with the reason.
func (im *importer) addServer(name string, s nativeServer) {
	id := "mcp/" + name
	switch {
	case !serverNameRe.MatchString(name):
		im.skip(id, "mcp", im.skipReason("badName", ""))
		return
	case strings.HasPrefix(name, "kira"):
		im.skip(id, "mcp", im.skipReason("kiraName", "kira"))
		return
	}
	for k := range s.Env {
		if !envNameRe.MatchString(k) || strings.HasPrefix(k, "KIRA_") {
			im.skip(id, "mcp", im.skipReason("badName", ""))
			return
		}
	}
	var (
		slots []SecretSlot
		pend  []pendingSecret
		out   omap
		fail  string
	)
	val := func(l loc, field, nameArg, raw, base string) string {
		text, sl, pe, why := im.value(l, nameArg, raw, name+"-"+base, "mcp_servers."+name+"."+field)
		if why != "" && fail == "" {
			fail = why
		}
		slots, pend = append(slots, sl...), append(pend, pe...)
		return text
	}
	vals := func(field string, l loc, m map[string]string) omap {
		var o omap
		for _, k := range sortedKeys(m) {
			o = append(o, kv{k, val(l, field+"."+k, k, m[k], k)})
		}
		return o
	}
	if s.URL != "" {
		out = append(out, kv{"url", val(locURL, "url", "", s.URL, "url")})
		if h := vals("headers", locHeader, s.Headers); len(h) > 0 {
			out = append(out, kv{"headers", h})
		}
	} else {
		out = append(out, kv{"command", val(locCommand, "command", "", s.Command, "command")})
		if len(s.Args) > 0 {
			args := make([]any, len(s.Args))
			prev := ""
			for i, a := range s.Args {
				args[i] = val(locArg, "args."+strconv.Itoa(i), prev, a, "arg"+strconv.Itoa(i))
				prev = a
			}
			out = append(out, kv{"args", args})
		}
		if e := vals("env", locEnv, s.Env); len(e) > 0 {
			out = append(out, kv{"env", e})
		}
	}
	if s.Disabled {
		out = append(out, kv{"enabled", false})
	}
	if fail != "" {
		im.skip(id, "mcp", fail)
		return
	}
	im.add(id, "mcp", []op{{path: []string{"mcp_servers", name}, value: out}}, slots, pend)
}
