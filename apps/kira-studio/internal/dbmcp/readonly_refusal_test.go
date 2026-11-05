package dbmcp

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
)

func TestIsReadOnlyRefusalMatchesStructuredErrorOnly(t *testing.T) {
	plain := adapters.New(adapters.CodeQuery, `relation "readonly_users" does not exist (1290)`, errors.New("x"))
	if isReadOnlyRefusal(plain) {
		t.Fatal("message text must not count as a refusal")
	}
	typed := adapters.New(adapters.CodeQuery, "cannot execute INSERT in a read-only transaction", adapters.RefusedWrite(errors.New("25006")))
	if !isReadOnlyRefusal(typed) {
		t.Fatal("tagged error must count as a refusal")
	}
}
