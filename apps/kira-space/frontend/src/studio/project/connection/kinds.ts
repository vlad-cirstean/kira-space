import type { ConnectionKind } from '@shared/domain/connection';

export const KIND_LABEL: Record<ConnectionKind, string> = {
  postgres: 'PostgreSQL',
  mariadb: 'MariaDB',
  mysql: 'MySQL',
  sqlite: 'SQLite',
  clickhouse: 'ClickHouse',
  mongodb: 'MongoDB',
  redis: 'Redis',
  kafka: 'Kafka',
  sqs: 'SQS',
  s3: 'S3',
};

// Each engine owns a hue distinct from the engines it most often sits beside in the list.
export const KIND_ACCENT_CLASS: Record<ConnectionKind, string> = {
  postgres: 'text-conn-cyan',
  mariadb: 'text-conn-blue',
  mysql: 'text-conn-teal',
  sqlite: 'text-conn-violet',
  clickhouse: 'text-conn-orange',
  mongodb: 'text-conn-green',
  redis: 'text-conn-red',
  kafka: 'text-conn-amber',
  sqs: 'text-conn-magenta',
  s3: 'text-conn-olive',
};

export const SUPPORTED_KINDS: ReadonlySet<ConnectionKind> = new Set([
  'postgres',
  'mariadb',
  'mysql',
  'sqlite',
  'clickhouse',
  'mongodb',
  'redis',
  'kafka',
  'sqs',
  's3',
]);
