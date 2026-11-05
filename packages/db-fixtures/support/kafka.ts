import { KafkaContainer, type StartedKafkaContainer } from '@testcontainers/kafka';
import { seedKafka } from '../fixtures/0005_kafka_seed';
import type { ResolvedConnectionConfig } from './connectionConfig';
import { resolveDockerHost } from './docker';

resolveDockerHost();

// confluentinc/cp-kafka, not apache/kafka — the latter's listener never
// opens under Testcontainers' bootstrap. `.withKraft()` is explicit rather than relying on
// KafkaContainer's own `>=8.0.0` auto-enable (harmless either way, and explicit beats implicit) —
// a single-container KRaft broker is both simpler and faster to boot than a two-container
// broker+ZooKeeper pair.
// Bumped from 7.6.1 (Kafka 3.6) to the 8.0 line (Apache Kafka 4.0) — a phase whose entire
// premise is Kafka 4 protocol compatibility that only ever ran against Kafka 3.6 verified
// nothing. Pinned to the newest published 8.0.x patch at the time of this change, matching how
// 7.6.1 was pinned; a later 8.x (4.1/4.2/4.3) is a deliberate future bump, not a drift.
const IMAGE = 'confluentinc/cp-kafka:8.0.7';
// KafkaContainer's own internal PLAINTEXT listener port — not exported by @testcontainers/kafka,
// so pinned here (kafka-container.js's own `KAFKA_PORT` constant).
const KAFKA_PORT = 9093;
const STARTUP_TIMEOUT_MS = 120_000;

export interface KafkaFixture {
  container: StartedKafkaContainer;
  config: ResolvedConnectionConfig; // ready to hand to the adapter
  host: string;
  port: number;
  stop(): Promise<void>;
}

// One container per test process, same discipline as support/redis.ts (§11b).
let memoized: Promise<KafkaFixture> | null = null;

export function startKafka(): Promise<KafkaFixture> {
  if (!memoized) memoized = start();
  return memoized;
}

async function start(): Promise<KafkaFixture> {
  const container = await new KafkaContainer(IMAGE)
    .withKraft()
    // KafkaContainer's own KRaft defaults (kafka-container.js) set the offsets-topic replication
    // factor and the transaction-log min ISR down to 1 for a single-broker cluster, but miss the
    // transaction-log replication factor itself — that stays at Kafka's default of 3. With only
    // one broker to replicate to, __transaction_state can never be created, so any transactional
    // producer (test 21's commit-marker seed) wedges the broker's InitProducerId handling
    // indefinitely. Nothing to do with Electron or the client library — a bare single-node Kafka
    // gotcha.
    .withEnvironment({ KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: '1' })
    .withStartupTimeout(STARTUP_TIMEOUT_MS)
    .start();

  const host = container.getHost();
  const port = container.getMappedPort(KAFKA_PORT);

  await seedKafka(container);

  const now = new Date().toISOString();
  const config: ResolvedConnectionConfig = {
    id: 'test-kafka',
    sortOrder: 0,
    createdAt: now,
    updatedAt: now,
    name: 'Test Kafka',
    kind: 'kafka',
    color: 'orange',
    mode: 'fields',
    readOnly: false,
    host,
    port,
    database: null,
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
    mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit' },
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
    password: null,
  };

  return {
    container,
    config,
    host,
    port,
    async stop() {
      // Playwright's workers:1 config runs every UI spec file sequentially in the same worker
      // process, sharing this module's state — without resetting `memoized`, a later spec file's
      // startKafka() would return this now-dead container instead of starting a fresh one.
      memoized = null;
      await container.stop();
    },
  };
}
