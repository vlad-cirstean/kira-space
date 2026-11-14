package mobileflow_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// J-SEC-001 mobile half: a stored agent secret is live (store API), yet no phone read of the task,
// board, sessions or run log carries its plaintext. The store file is obfuscated, so the store API
// is the truthful liveness check.
func TestPhoneReadsNeverCarrySecrets(t *testing.T) {
	const marker = "MARK-mobile-secret-77ac"
	f := newFixture(t)
	svc := f.app.W.AgentSettingsSvc
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "mobile-ref", Value: marker}); err != nil {
		t.Fatal(err)
	}
	cur, err := svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(agentcfg.SaveArgs{BaseHash: cur.Hash, Text: "env:\n  MOBILE_REF: ${secret:mobile-ref}\n"}); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
	vals, _, err := f.app.W.AgentSecrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
	if err != nil || vals["mobile-ref"] != marker {
		t.Fatalf("store = %v, %v, want the live secret", vals, err)
	}
	p, _ := pairedPhone(t, f)
	for _, path := range []string{
		"/api/ade/board", "/api/ade/workflows", "/api/ade/sessions", "/api/ade/repos",
		"/api/ade/log?kind=run&id=" + f.runID, "/api/agent/sessions",
	} {
		r := p.get(path)
		if r.Status != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, r.Status, r.Body)
		}
		if strings.Contains(string(r.Body), marker) {
			t.Errorf("GET %s carries the secret: %s", path, r.Body)
		}
	}
}
