package model

import "github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"

// ConsoleRequest is domain/console.ts's ConsoleRequest — one execute() batch: path binds it to a
// connection and, optionally, a default database/schema; Statements is the pre-split list from
// sql-split.ts (one call covers both "Run statement" and "Run all").
type ConsoleRequest struct {
	Path       NodePath
	Statements []string
	// Cap bounds each statement's result; zero fields mean the adapters.DefaultConsoleCap. Go
	// callers only (dbmcp), never set from the renderer.
	Cap page.ResultCap
	// ReadOnly forces the adapter's read-only wrap even when the connection is not read-only. Set
	// by the DB MCP read lane (defence in depth; the lane's connection is read-only already).
	ReadOnly bool
}

// MongoConsoleMethods mirrors domain/console.ts's MONGO_CONSOLE_METHODS — the ten shell methods
// mongo/console.go dispatches. The renderer's own completion source (TypeScript) reads the TS list
// directly; this is the single Go-side copy every native Mongo console
// caller shares, so the method set is never spelled out twice within the Go codebase either.
var MongoConsoleMethods = []string{
	"find",
	"findOne",
	"insertOne",
	"insertMany",
	"updateOne",
	"updateMany",
	"deleteOne",
	"deleteMany",
	"countDocuments",
	"aggregate",
}
