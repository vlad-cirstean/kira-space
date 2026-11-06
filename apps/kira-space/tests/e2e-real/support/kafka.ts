// Re-exports the DB suite's Testcontainers harness so tests/e2e-real/*.spec.ts can start the same
// real Kafka container without duplicating it, mirroring support/postgres.ts's own precedent.
export {
  type KafkaFixture,
  startKafka,
} from '../../../../../packages/db-fixtures/support/kafka';
