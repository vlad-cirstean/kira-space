package connections

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestReadyAddr(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	tests := []struct {
		name string
		cfg  model.ResolvedConnectionConfig
		want string
	}{
		{"fields localhost", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "fields", Host: strPtr("localhost"), Port: intPtr(5432)}, "localhost:5432"},
		{"fields 127.0.0.2", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "fields", Host: strPtr("127.0.0.2"), Port: intPtr(1)}, "127.0.0.2:1"},
		{"fields ipv6 loopback", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "fields", Host: strPtr("::1"), Port: intPtr(5432)}, "[::1]:5432"},
		{"fields remote", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "fields", Host: strPtr("db.example.com"), Port: intPtr(5432)}, ""},
		{"fields no port", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "fields", Host: strPtr("localhost")}, ""},
		{"file kind", model.ResolvedConnectionConfig{Kind: "sqlite", Mode: "fields", Host: strPtr("localhost"), Port: intPtr(1)}, ""},
		{"uri loopback", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "uri", URI: strPtr("postgresql://u:p@127.0.0.1:6543/db")}, "127.0.0.1:6543"},
		{"uri ipv6", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "uri", URI: strPtr("postgresql://u@[::1]:6543/db")}, "[::1]:6543"},
		{"uri no port", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "uri", URI: strPtr("postgresql://u@localhost/db")}, ""},
		{"uri remote", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "uri", URI: strPtr("postgresql://u@db.example.com:5432/db")}, ""},
		{"uri multi-host", model.ResolvedConnectionConfig{Kind: "mongodb", Mode: "uri", URI: strPtr("mongodb://localhost:1,localhost:2/db")}, ""},
		{"uri unparsable", model.ResolvedConnectionConfig{Kind: "postgres", Mode: "uri", URI: strPtr("://bad")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readyAddr(tt.cfg); got != tt.want {
				t.Errorf("readyAddr = %q, want %q", got, tt.want)
			}
		})
	}
}
