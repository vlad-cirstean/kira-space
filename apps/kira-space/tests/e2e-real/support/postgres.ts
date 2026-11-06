// Re-exports the DB suite's Testcontainers harness so tests/e2e-real/*.spec.ts can start the same
// real Postgres container without duplicating it, mirroring tests/e2e/support/pg.ts's own
// precedent. Must run under plain Node — @testcontainers/postgresql's wait strategy hangs
// indefinitely under Bun in this sandbox (CLAUDE.md's Docker section) — so this project's own tests
// must be invoked via the Playwright CLI's Node entrypoint, not `bunx playwright test`.
export {
  DOCKER_UNAVAILABLE_MESSAGE,
  isDockerAvailable,
} from '../../../../../packages/db-fixtures/support/docker';
export {
  type PgFixture,
  startPostgres,
} from '../../../../../packages/db-fixtures/support/postgres';
