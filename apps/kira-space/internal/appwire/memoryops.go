package appwire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/mcpcall"
	"github.com/kirathecat/kira-studio/internal/memory/memorycli"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// opOwnersDir holds the owner lock files of processes that write op_log from outside the app.
func opOwnersDir() string { return filepath.Join(config.KiraSpaceHome(), "op-owners") }

// memoryOps records each memory MCP tool call in the app's op_log. The app owns the schema: a
// missing kira.db or op_log table records nothing, and nothing is created or migrated. The app
// ingests the rows by polling data_version.
func memoryOps(dbPath, ownersDir string) memorycli.OpsRecorder {
	return func(project string, importMode bool) (mcpcall.Hook, func()) {
		none := func() {}
		if _, err := os.Stat(dbPath); err != nil {
			return nil, none
		}
		db, err := sqlitex.Open(dbPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "kira-memory: op log off:", err)
			return nil, none
		}
		if _, err := sqlb.Exec(db, sqlb.SQLite.Select("1").From(kiraschema.OpLog.Table).Limit(0)); err != nil {
			fmt.Fprintln(os.Stderr, "kira-memory: op log off:", err)
			_ = db.Close()
			return nil, none
		}
		owner, err := oplog.ClaimOwner(ownersDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "kira-memory: op log off:", err)
			_ = db.Close()
			return nil, none
		}
		store := &repos.OpLogRepo{DB: db}
		log := oplog.New(oplog.WithStore(store), oplog.WithIDPrefix(owner.Token()+"."))
		if err := store.Prune(time.Now().Add(-oplog.Retention), oplog.MaxRows); err != nil {
			fmt.Fprintln(os.Stderr, "kira-memory: op log prune:", err)
		}
		trigger := oplog.TriggerMCP
		if importMode {
			trigger = oplog.TriggerAuto
		}
		hook := func(ctx context.Context, c mcpcall.Call) (context.Context, func(bool, string, error)) {
			ctx = oplog.WithOrigin(ctx, oplog.Origin{Module: oplog.ModuleMemory, Trigger: trigger, Label: "MCP " + c.Tool})
			op := log.Start(ctx, oplog.Meta{
				Category: oplog.CategoryMCP, Kind: c.Tool, Source: "MCP " + c.Tool, Subject: project, Background: importMode,
			})
			op.SetCommand(c.Tool + " " + oplog.RedactJSON(c.Args, 2<<10))
			return ctx, func(isError bool, text string, err error) { oplog.FinishCall(op, isError, text, err) }
		}
		// Drain the writer before the owner lock goes: a running row with a free lock is orphaned.
		return hook, func() {
			log.Close()
			_ = db.Close()
			owner.Release()
		}
	}
}
