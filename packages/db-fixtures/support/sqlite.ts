import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import type { ResolvedConnectionConfig } from './connectionConfig';

const BIG_ROWS = 1_000_000;
const SEED_SQL_PATH = resolve(__dirname, '../fixtures/0009_sqlite_seed.sql');
const DATABASE_FILE = 'kira_test.sqlite';

export interface SqliteFixture {
  path: string;
  /** The temp directory `path` lives in — sibling paths for a missing-file/directory/roundtrip
   *  connect-failure test live here too, rather than each test managing its own mkdtemp. */
  dir: string;
  config: ResolvedConnectionConfig; // ready to hand to the adapter
  stop(): Promise<void>;
}

// D33: this suite's one environment dependency — everything else it needs (a temp directory, a
// file) runs anywhere. Mirrors support/docker.ts's isDockerAvailable()/DOCKER_UNAVAILABLE_MESSAGE
// discipline: a legible skip naming the runtime requirement, not a bare module-resolution error.
export async function sqliteAvailable(): Promise<boolean> {
  try {
    await import('node:sqlite');
    return true;
  } catch {
    return false;
  }
}

export const SQLITE_UNAVAILABLE_MESSAGE =
  'node:sqlite is unavailable in this runtime — Bun 1.4+ (or Electron/Node 22.5+) is required ' +
  'to run packages/db-fixtures/sqlite.spec.ts.';

// One temp database per test process, same discipline as support/mariadb.ts (§11b) — though here
// it buys nothing but the same memoization shape, since there is no container to reuse.
let memoized: Promise<SqliteFixture> | null = null;

export function startSqlite(opts?: { seedBigTable?: boolean }): Promise<SqliteFixture> {
  if (!memoized) memoized = start(opts);
  return memoized;
}

async function start(opts?: { seedBigTable?: boolean }): Promise<SqliteFixture> {
  // D32: a temp-file fixture, not a container — no image pin, no healthcheck, no wait strategy,
  // no root-versus-app-user split, no resolveDockerHost(). A file-based engine makes this the
  // simplest fixture of the eight.
  const { DatabaseSync } = await import('node:sqlite');
  const dir = mkdtempSync(join(tmpdir(), 'kira-sqlite-'));
  const path = join(dir, DATABASE_FILE);

  const db = new DatabaseSync(path);
  try {
    db.exec(readFileSync(SEED_SQL_PATH, 'utf8'));

    // Gated: the harness only needs to prove it can build the table; the paging is what
    // consumes 1M rows. Default true because the paging genuinely consumes them —
    // same rule support/mariadb.ts's own gate follows.
    //
    // Unlike MariaDB's SEQUENCE-engine seq_1_to_N or MySQL's chunked digits cross join, one plain
    // WITH RECURSIVE CTE inserts all 1,000,000 rows directly (~1s) — no chunking, no PRAGMA
    // adjustment needed.
    if (opts?.seedBigTable ?? true) {
      db.exec(
        `WITH RECURSIVE seq(n) AS (
           SELECT 1
           UNION ALL
           SELECT n + 1 FROM seq WHERE n < ${BIG_ROWS}
         )
         INSERT INTO big_rows (id, payload)
         SELECT n, hex(randomblob(16)) FROM seq`,
      );
      // ANALYZE on big_rows only, mirroring 0002_mariadb_seed.sql's own note — every other table
      // is left with no sqlite_stat1 row, which is what scenario 6 needs to assert a null estimate.
      db.exec('ANALYZE big_rows');
    }
  } finally {
    db.close();
  }

  const now = new Date().toISOString();
  const config: ResolvedConnectionConfig = {
    id: 'test-sqlite',
    sortOrder: 0,
    createdAt: now,
    updatedAt: now,
    name: 'Test SQLite',
    kind: 'sqlite',
    color: 'violet',
    mode: 'fields',
    readOnly: false,
    host: null,
    port: null,
    database: path,
    username: null,
    uri: null,
    options: {},
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReviewResults: 'inherit',
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    password: null,
  };

  return {
    path,
    dir,
    config,
    async stop() {
      // Playwright's workers:1 config runs every UI spec file sequentially in the same worker
      // process, sharing this module's state — support/mariadb.ts's own reason for resetting the
      // memo before the next spec file's startSqlite() call.
      memoized = null;
      rmSync(dir, { recursive: true, force: true });
    },
  };
}
