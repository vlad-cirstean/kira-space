package agentcfg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// legacySecretsFile is the v0.3.1 file that held agent credentials beside kira.db.
const legacySecretsFile = "agent-secrets.db"

// MoveSecretsFile is a one-time move: it copies the sealed rows of <home>/agent-secrets.db into db's
// secret_values, then parks the file under <home>/backups/p557-<UTC time>/. Values stay sealed: scope
// is the cipher's only AAD and does not change. It is idempotent and safe to interrupt; a repeat after
// a crash skips rows already copied and verifies them. Any mismatch keeps the file and returns an
// error. Logs carry counts only.
func MoveSecretsFile(db *sql.DB, home string) error {
	src := filepath.Join(home, legacySecretsFile)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("agentcfg: stat %s: %w", legacySecretsFile, err)
	}
	if err := checkpointFile(src); err != nil {
		return err
	}
	total, copied, err := copySecretRows(context.Background(), db, src)
	if err != nil {
		return err
	}
	dir, err := parkSecretsFile(home, src)
	if err != nil {
		return err
	}
	slog.Info("agent secrets moved into kira.db", "scope", "startup", "rows", total, "copied", copied, "backup", dir)
	return nil
}

// checkpointFile folds the file's WAL into it so the read-only attach sees every row.
func checkpointFile(path string) error {
	old, err := sqlitex.Open(path)
	if err != nil {
		return fmt.Errorf("agentcfg: open %s: %w", legacySecretsFile, err)
	}
	_, err = old.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	if cerr := old.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("agentcfg: checkpoint %s: %w", legacySecretsFile, err)
	}
	return nil
}

// copySecretRows copies the file's rows in one immediate transaction and returns the source row
// count and the number newly inserted.
func copySecretRows(ctx context.Context, db *sql.DB, src string) (total, copied int, err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("agentcfg: secrets connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS legacy`, sqlitex.ReadOnlyURI(src)); err != nil {
		return 0, 0, fmt.Errorf("agentcfg: attach %s: %w", legacySecretsFile, err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE legacy`) }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return 0, 0, fmt.Errorf("agentcfg: begin: %w", err)
	}
	done := false
	defer func() {
		if !done {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	var qerr error
	count := func(q string, args ...any) (n int) {
		if qerr == nil {
			qerr = conn.QueryRowContext(ctx, q, args...).Scan(&n)
		}
		return n
	}
	total = count(`SELECT count(*) FROM legacy.secret_values`)
	foreign := count(`SELECT count(*) FROM legacy.secret_values WHERE scope <> ? OR owner <> ?`, string(secrets.ScopeAgentCredential), SecretOwner)
	if qerr != nil {
		return 0, 0, fmt.Errorf("agentcfg: read %s: %w", legacySecretsFile, qerr)
	}
	if foreign > 0 {
		return 0, 0, fmt.Errorf("agentcfg: %s holds %d rows that are not agent credentials", legacySecretsFile, foreign)
	}
	res, err := conn.ExecContext(ctx, `INSERT INTO main.secret_values (scope, owner, name, value, updated_ms)
		SELECT scope, owner, name, value, updated_ms FROM legacy.secret_values WHERE true ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, 0, fmt.Errorf("agentcfg: copy secrets: %w", err)
	}
	n, _ := res.RowsAffected()
	copied = int(n)
	missing := count(`SELECT count(*) FROM legacy.secret_values l WHERE NOT EXISTS (
		SELECT 1 FROM main.secret_values m WHERE m.scope = l.scope AND m.owner = l.owner AND m.name = l.name AND m.value = l.value)`)
	if qerr != nil {
		return 0, 0, fmt.Errorf("agentcfg: verify secrets: %w", qerr)
	}
	if missing > 0 {
		return 0, 0, fmt.Errorf("agentcfg: %d of %d agent secrets differ from kira.db; %s kept", missing, total, legacySecretsFile)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return 0, 0, fmt.Errorf("agentcfg: commit secrets: %w", err)
	}
	done = true
	return total, copied, nil
}

// parkSecretsFile moves the file and its WAL siblings into a new backup folder and returns its name.
func parkSecretsFile(home, src string) (string, error) {
	dir := filepath.Join(home, "backups", "p557-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("agentcfg: create backup folder: %w", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		err := os.Rename(src+suffix, filepath.Join(dir, legacySecretsFile+suffix))
		if err != nil && (suffix == "" || !errors.Is(err, os.ErrNotExist)) {
			return "", fmt.Errorf("agentcfg: move %s%s aside: %w", legacySecretsFile, suffix, err)
		}
	}
	return filepath.Base(dir), nil
}
