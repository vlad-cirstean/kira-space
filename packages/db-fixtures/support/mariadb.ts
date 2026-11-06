import { resolve } from 'node:path';
import { MariaDbContainer, type StartedMariaDbContainer } from '@testcontainers/mariadb';
import { createConnection, importFile } from 'mariadb';
import { Wait } from 'testcontainers';
import { baseConnectionConfig } from './common';
import type { ResolvedConnectionConfig } from './connectionConfig';
import { resolveDockerHost } from './docker';

resolveDockerHost();

const IMAGE = 'mariadb:11.4.13';
const ROOT_PASSWORD = 'kira';
const DATABASE = 'kira_test';
const ANALYTICS_DATABASE = 'kira_analytics';
const USERNAME = 'kira';
const PASSWORD = 'kira';
const STARTUP_TIMEOUT_MS = 120_000;
const BIG_ROWS = 1_000_000;
const SEED_SQL_PATH = resolve(__dirname, '../fixtures/0002_mariadb_seed.sql');

export interface MariaFixture {
  container: StartedMariaDbContainer;
  config: ResolvedConnectionConfig; // ready to hand to the adapter
  uri: string;
  stop(): Promise<void>;
}

// One container per test process, same discipline as support/postgres.ts (§11b).
let memoized: Promise<MariaFixture> | null = null;

export function startMariadb(opts?: { seedBigTable?: boolean }): Promise<MariaFixture> {
  if (!memoized) memoized = start(opts);
  return memoized;
}

async function start(opts?: { seedBigTable?: boolean }): Promise<MariaFixture> {
  // Unlike @testcontainers/postgresql, @testcontainers/mariadb sets no wait strategy or
  // healthcheck of its own (as of 12.1.0) — it only wires up the env vars/getters, so the
  // double-boot-during-init wait strategy below is still ours to provide.
  const container = await new MariaDbContainer(IMAGE)
    .withDatabase(DATABASE)
    .withUsername(USERNAME)
    .withUserPassword(PASSWORD)
    .withRootPassword(ROOT_PASSWORD)
    // performance_schema is off by default on MariaDB (unlike MySQL) — scenario 1 needs
    // performance_schema.SESSION_CONNECT_ATTRS to assert that a disconnected session's connect
    // attributes are gone.
    .withCommand(['--performance-schema=ON'])
    // The image ships healthcheck.sh; this is more reliable than a log match. (If a future pinned
    // tag drops the script, the fix is Wait.forLogMessage(/ready for connections/, 2) — MariaDB's
    // entrypoint starts the server twice for the same reason Postgres's does, and waiting for only
    // the first occurrence gets you a "connection refused" a moment later.)
    .withHealthCheck({
      test: ['CMD', 'healthcheck.sh', '--connect', '--innodb_initialized'],
      interval: 2000,
      timeout: 5000,
      retries: 60,
    })
    .withWaitStrategy(Wait.forHealthCheck())
    .withStartupTimeout(STARTUP_TIMEOUT_MS)
    .start();

  const host = container.getHost();
  const port = container.getPort();
  const uri = `mariadb://${USERNAME}:${PASSWORD}@${host}:${port}/${DATABASE}`;

  // Seeded as root (CREATE/GRANT need it); the returned config connects as the unprivileged
  // `kira` user — the entrypoint already grants it full rights on kira_test, so the read path
  // proves KILL QUERY on your own query needs no PROCESS privilege (§6e).
  await importFile({
    host,
    port,
    user: 'root',
    password: ROOT_PASSWORD,
    database: DATABASE,
    multipleStatements: true,
    file: SEED_SQL_PATH,
  });

  // `database` is required here even though every statement below qualifies its table names —
  // the SEQUENCE-engine `seq_1_to_N` pseudo-table used for the big_rows bulk insert is always
  // unqualified and resolves against whatever database the connection has selected, so a
  // connection with none throws ER_NO_DB_ERROR.
  const rootConn = await createConnection({
    host,
    port,
    user: 'root',
    password: ROOT_PASSWORD,
    database: DATABASE,
  });
  try {
    // A second database, mirroring the Postgres fixture's `analytics` schema — MariaDB has no
    // schema level, so a second *database* is the equivalent (§6d).
    await rootConn.query(`CREATE DATABASE IF NOT EXISTS \`${ANALYTICS_DATABASE}\``);
    await rootConn.query(
      `CREATE TABLE IF NOT EXISTS \`${ANALYTICS_DATABASE}\`.events (
         id INT AUTO_INCREMENT PRIMARY KEY,
         event_name VARCHAR(255) NOT NULL,
         occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
       )`,
    );
    await rootConn.query(
      `INSERT INTO \`${ANALYTICS_DATABASE}\`.events (event_name) VALUES ('signup'), ('login')`,
    );
    await rootConn.query(`GRANT SELECT ON \`${ANALYTICS_DATABASE}\`.* TO '${USERNAME}'@'%'`);
    await rootConn.query('FLUSH PRIVILEGES');

    // Gated: the harness only needs to prove it can build the table; P2's paging is what
    // actually consumes 1M rows. Default true, unlike P1, because P2 genuinely consumes them.
    if (opts?.seedBigTable ?? true) {
      await rootConn.query(
        `INSERT INTO \`${DATABASE}\`.big_rows (id, payload)
         SELECT seq, MD5(seq) FROM seq_1_to_${BIG_ROWS}`,
      );
      await rootConn.query(`ANALYZE TABLE \`${DATABASE}\`.big_rows`);
    }
  } finally {
    await rootConn.end();
  }

  const config: ResolvedConnectionConfig = baseConnectionConfig({
    id: 'test-mariadb',
    name: 'Test MariaDB',
    kind: 'mariadb',
    host,
    port,
    database: DATABASE,
    username: USERNAME,
    password: PASSWORD,
  });

  return {
    container,
    config,
    uri,
    async stop() {
      // Playwright's workers:1 config runs every UI spec file sequentially in the same worker
      // process, sharing this module's state — without resetting `memoized`, a later spec file's
      // startMariadb() would return this now-dead container instead of starting a fresh one.
      memoized = null;
      await container.stop();
    },
  };
}
