package appwire

import (
	"context"
	"errors"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configsync"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/appevent"
)

// windowPrompter routes a credential prompt to one window through the relay.
type windowPrompter struct {
	relay *gitcred.Relay
	key   string
}

func (p windowPrompter) Ask(ctx context.Context, req gitaskpass.Request) (string, bool) {
	return p.relay.AskFrom(ctx, p.key, "Shared config", req)
}

// wireConfigSync builds the shared-config sync service. Start runs from Wired.Start.
func (w *Wired) wireConfigSync(git gitWired, repositories *repos.Repos, home string, emitter appevent.Emitter) {
	gitPath := adeGitPathSetting(repositories)
	o := configsync.Options{
		Home:   home,
		Runner: git.runner,
		GitPath: func(ctx context.Context) (string, error) {
			st := git.discovery.Status(ctx, gitPath())
			if st.Kind != "ok" {
				return "", errors.New(st.Kind)
			}
			return st.Path, nil
		},
		Validate: map[string]configsync.ValidateFunc{
			"steps": func(_ string, data []byte) string {
				if v := adeflow.ValidateStepYaml(string(data)); v.Error != nil {
					return v.Error.Message
				}
				return ""
			},
		},
		Emit: func(st configsync.Status) { emitter.Emit(bridge.ChannelConfigSyncChanged, st) },
	}
	if w.AgentSettings != nil {
		o.Validate["agent"] = func(file string, data []byte) string {
			if file != "settings.yaml" {
				return ""
			}
			if p := w.AgentSettings.Check(data); p != nil {
				return p.Message
			}
			return ""
		}
	}
	if w.AgentSecrets != nil {
		o.SecretUnset = func(module, name string) bool {
			if module != "agent" {
				return false
			}
			names, err := w.AgentSecrets.Names()
			if err != nil {
				return false
			}
			for _, n := range names {
				if n == name {
					return false
				}
			}
			return true
		}
	}
	if git.askpassBroker != nil {
		o.Auth = func(ctx context.Context, key string, fn func(env []string) error) error {
			if !git.askpassBroker.ShouldInterpose("") {
				return fn(nil)
			}
			return git.askpassBroker.WithOp(ctx, windowPrompter{relay: w.CredentialRelay, key: key}, func(opEnv []string) error {
				return fn(append(git.askpassBroker.Env(), opEnv...))
			})
		}
	}
	w.ConfigSync = configsync.New(o)
	w.ConfigSyncSvc = &bridge.ConfigSyncService{Sync: w.ConfigSync}
}
