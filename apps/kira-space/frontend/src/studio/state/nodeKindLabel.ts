import type { NodeKind } from '@shared/domain/tree';
import { type MessageKey, t } from '@theme/i18n';

const KEYS: Record<NodeKind, MessageKey> = {
  connection: 'studio.common.nodeKind.connection.label',
  database: 'studio.project.connectionDialog.database.label',
  schema: 'studio.common.nodeKind.schema.label',
  table: 'studio.definition.table.label',
  view: 'common.view.label',
  matview: 'studio.common.nodeKind.matview.label',
  function: 'studio.common.nodeKind.function.label',
  sequence: 'studio.common.nodeKind.sequence.label',
  column: 'studio.project.connectionDialog.column.placeholder',
  collection: 'common.collection.label',
  namespace: 'studio.common.nodeKind.namespace.label',
  key: 'common.field.key.label',
  topic: 'studio.common.nodeKind.topic.label',
  partition: 'studio.common.nodeKind.partition.label',
  consumerGroup: 'studio.common.nodeKind.consumerGroup.label',
  queue: 'studio.common.nodeKind.queue.label',
  bucket: 'studio.common.nodeKind.bucket.label',
  prefix: 'studio.common.nodeKind.prefix.label',
  object: 'studio.common.nodeKind.object.label',
};

export function nodeKindLabel(kind: NodeKind): string {
  return t(KEYS[kind]);
}
