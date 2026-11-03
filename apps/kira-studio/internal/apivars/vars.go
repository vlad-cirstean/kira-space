// Package apivars (it resolves gRPC targets and metadata too, so the name means the module, not
// the HTTP protocol) is the Go-side half: collection variables, top-level named environments, the
// gated secret reveal, and stage 2 of the two-stage {{name}} substitution — the half that resolves
// a secret, strictly after bridge/http.go's op.SetCommand call, so a decrypted credential never
// reaches op_log.command.
//
// Api-scoped Go, beside internal/httpclient, internal/grpcclient and internal/postman — imports
// internal/storage/repos, internal/secrets and internal/localauth, and deliberately not
// internal/connections (importing Studio's connections package from here would be exactly the
// Studio<->Api coupling docs/HISTORY.md (v1.2)'s module-boundary section exists to prevent) and
// not Wails.
package apivars

import (
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// SecretStore is the Api secret namespace: apifiles.Secrets satisfies it.
type SecretStore interface {
	// Snapshot resolves ${secret:name} for one send; it decrypts once.
	Snapshot() configfs.SecretSource
	// Reveal opens one value by name.
	Reveal(name string) (string, error)
}

// Authorizer is connections.Service's own seam, redeclared here for the identical reason: a test
// can inject a fake outcome sequence with no real clock or OS-auth probe wired through.
// *localauth.Authorizer satisfies this structurally.
type Authorizer interface {
	Authorize(reason string, confirmed bool) (localauth.Outcome, error)
}

// Deps is everything the service needs. Secret values come from Secrets by the ref a variable row
// names; Repo holds the Cipher only for a history entry's old sealed value.
type Deps struct {
	Repo    *repos.VariablesRepo
	Auth    Authorizer
	Secrets SecretStore
}

// Service is the Go analogue of a P5-scoped connections.Service: the bridge (bridge/variables.go)
// wraps every method here in a typed-struct arg and an ipcerr translation.
type Service struct {
	deps Deps
}

// New wires apivars against the SAME *localauth.Authorizer instance connections.Service already
// uses — main.go constructs exactly one Authorizer and passes it to both, which is what makes
// the 5-minute reveal grace genuinely shared between a connection-password reveal and a variable
// reveal.
func New(repo *repos.VariablesRepo, auth *localauth.Authorizer, secrets SecretStore) *Service {
	return &Service{deps: Deps{Repo: repo, Auth: auth, Secrets: secrets}}
}
