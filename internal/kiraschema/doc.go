// Package kiraschema holds the table and column names of kira.db as Go identifiers.
// kiraschema_gen.go is generated from a freshly migrated database by TestKiraSchemaGenerated.
package kiraschema

//go:generate go test ../../apps/kira-space/internal/storage -run "TestKiraSchema|TestSchemaSnapshot" -update
