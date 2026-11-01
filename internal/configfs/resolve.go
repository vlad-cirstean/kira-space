package configfs

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrSecretMissing marks a secret that is not set. SecretSource errors wrap it when absent.
var ErrSecretMissing = errors.New("configfs: secret not set")

// SecretSource returns the plaintext of one app secret in a namespace (the owning module's).
type SecretSource interface {
	Secret(ctx context.Context, namespace, name string) (string, error)
}

// SecretSourceFunc adapts a function to SecretSource.
type SecretSourceFunc func(ctx context.Context, namespace, name string) (string, error)

func (f SecretSourceFunc) Secret(ctx context.Context, namespace, name string) (string, error) {
	return f(ctx, namespace, name)
}

// Used is one secret that went into a resolved string, for masking: every copyable surface shows
// Placeholder, never Value.
type Used struct{ Name, Value, Placeholder string }

// Resolver resolves references at use time. Namespace is the owning module's, fixed by the module and
// never written in a file. Env is injected: the GUI process env lacks the login shell's variables.
type Resolver struct {
	Env       func(string) (string, bool)
	Secrets   SecretSource
	Namespace string
}

// Resolve substitutes every reference in text. A missing env var or secret fails with an error naming
// the reference, never an empty string. Env values are not reported as Used.
func (r *Resolver) Resolve(ctx context.Context, text string) (string, []Used, error) {
	refs, p := Refs(text)
	if p != nil {
		return "", nil, errors.New(p.Message)
	}
	var b strings.Builder
	var used []Used
	last := 0
	for _, ref := range refs {
		b.WriteString(unescape(text[last:ref.Start]))
		last = ref.End
		switch ref.Kind {
		case RefEnv:
			v, ok := "", false
			if r.Env != nil {
				v, ok = r.Env(ref.Name)
			}
			if !ok {
				return "", nil, fmt.Errorf("configfs: environment variable %s is not set", ref.Name)
			}
			b.WriteString(v)
		case RefSecret:
			if r.Secrets == nil {
				return "", nil, fmt.Errorf("configfs: secret %s: %w", ref.Name, ErrSecretMissing)
			}
			v, err := r.Secrets.Secret(ctx, r.Namespace, ref.Name)
			if err != nil {
				return "", nil, fmt.Errorf("configfs: secret %s: %w", ref.Name, err)
			}
			b.WriteString(v)
			used = append(used, Used{Name: ref.Name, Value: v, Placeholder: "${secret:" + ref.Name + "}"})
		}
	}
	b.WriteString(unescape(text[last:]))
	return b.String(), used, nil
}
