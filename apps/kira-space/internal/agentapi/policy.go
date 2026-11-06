package agentapi

import (
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
)

// available is a variable the user opted in for agents in one environment.
type available struct {
	name   string
	secret bool
	// secretName is the Api secret a secret variable points to.
	secretName string
	value      string
	hosts      []apifiles.HostRule
}

func (a available) hostStrings() []string {
	out := make([]string, len(a.hosts))
	for i, h := range a.hosts {
		out[i] = h.String()
	}
	return out
}

// availableOf collects an environment's opted-in variables. A secret whose reference does not parse
// is left out; a host entry that does not parse is dropped, which only narrows what the secret can reach.
func availableOf(ref apifiles.EnvironmentRef) map[string]available {
	out := map[string]available{}
	for _, v := range ref.File.Variables {
		if !v.Agent {
			continue
		}
		a := available{name: v.Name}
		if v.Secret != "" {
			a.secret, a.secretName = true, apifiles.SecretNameOf(v.Secret)
			if a.secretName == "" {
				continue
			}
			for _, h := range v.AgentHosts {
				if r, err := apifiles.ParseHostRule(h); err == nil {
					a.hosts = append(a.hosts, r)
				}
			}
		} else {
			a.value = v.Value
		}
		out[v.Name] = a
	}
	return out
}

func envInfoOf(ref apifiles.EnvironmentRef) EnvInfo {
	return EnvInfo{ID: ref.ID, Name: ref.Name, Color: ref.Color, AutoApproveReads: ref.File.AgentAutoApproveReads}
}

// Environments lists the environments that share at least one variable with agents. It never
// carries a value.
func (s *Service) Environments() ([]EnvView, error) {
	list, err := s.Envs.EnvironmentList()
	if err != nil {
		return nil, err
	}
	out := []EnvView{}
	for _, ref := range list {
		avail := availableOf(ref)
		if len(avail) == 0 {
			continue
		}
		v := EnvView{ID: ref.ID, Name: ref.Name, Description: ref.Description, Color: ref.Color, AutoApproveReads: ref.File.AgentAutoApproveReads, Variables: []VarView{}}
		names := make([]string, 0, len(avail))
		for n := range avail {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			a := avail[n]
			vv := VarView{Name: n, Secret: a.secret}
			if a.secret {
				vv.Hosts = a.hostStrings()
			}
			v.Variables = append(v.Variables, vv)
		}
		out = append(out, v)
	}
	return out, nil
}
