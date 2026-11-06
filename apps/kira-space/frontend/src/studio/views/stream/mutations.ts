import { pinia } from '../../state/pinia';
import { useTabsStore } from '../../state/tabs';
import { createImmediateMutator } from '../shared/immediateMutation';
import { useStreamViewStore } from './state';

// Mutate immediately, no staging/preview step — documents/mutations.ts's precedent, extended to
// streams. Each op uses the `$`-prefixed sentinel fields
// kafka/produce.ts and sqs/mutate.ts's adapters agree on (mirrors mongo/mutate.ts's `$document`).
// A thunk, not a direct `useTabsStore().findStreamTab` reference — see documents/mutations.ts's
// own comment (this module evaluates before `app.use(pinia)` too).
const mutate = createImmediateMutator({
  findTab: (tabId: string) => useTabsStore().findStreamTab(tabId),
  reload: (tabId: string) => useStreamViewStore(pinia).reload(tabId),
  isStale: (tabId: string) => useStreamViewStore(pinia).isStale(tabId),
});

export async function produceKafkaMessage(
  tabId: string,
  fields: { key: string | null; body: string; headers: string | null },
): Promise<void> {
  const values: Record<string, string | null> = { $key: fields.key, $body: fields.body };
  if (fields.headers !== null) values.$headers = fields.headers;
  await mutate(tabId, [{ kind: 'insert', values }]);
}

export async function sendSqsMessage(
  tabId: string,
  body: string,
  headers: string | null,
): Promise<void> {
  const values: Record<string, string | null> = { $body: body };
  if (headers !== null) values.$headers = headers;
  await mutate(tabId, [{ kind: 'insert', values }]);
}

// `messageId` mirrors sqs/mutate.ts's ID_FIELD — the row's own `key` column, which read.ts
// already sets to the SQS MessageId (P10's row shape), echoed straight back.
export async function deleteSqsMessage(tabId: string, messageId: string): Promise<void> {
  await mutate(tabId, [{ kind: 'delete', key: { messageId } }]);
}
