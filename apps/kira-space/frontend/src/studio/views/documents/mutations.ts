import { pinia } from '../../state/pinia';
import { useTabsStore } from '../../state/tabs';
import { createImmediateMutator } from '../shared/immediateMutation';
import { useDocumentViewStore } from './state';

// Documents mutate immediately — no pendingChanges.ts-style staged plan, no preview step.
// A thunk, not a direct `useTabsStore().findDocumentTab` reference — this module evaluates before
// `app.use(pinia)` (main.ts's own static import chain), so the store must not resolve until
// `findTab` is called (always post-mount, from a real mutation).
const mutate = createImmediateMutator({
  findTab: (tabId: string) => useTabsStore().findDocumentTab(tabId),
  reload: (tabId: string) => useDocumentViewStore(pinia).reload(tabId),
  isStale: (tabId: string) => useDocumentViewStore(pinia).isStale(tabId),
});

export async function saveDocumentEdit(
  tabId: string,
  id: string,
  newBodyEjson: string,
): Promise<void> {
  // The '$document' sentinel (P8 ground rules): a whole-document replace expressed through the
  // existing relational-shaped MutationRowOp rather than widening the shared mutation schema.
  await mutate(tabId, [{ kind: 'update', key: { _id: id }, changes: { $document: newBodyEjson } }]);
}

export async function saveNewDocument(tabId: string, bodyEjson: string): Promise<void> {
  // The same '$document' sentinel saveDocumentEdit uses above, on the insert variant of
  // MutationRowOp (mongo/mutate.ts's insertOne branch) — no `key`, since a new document has no
  // existing `_id` to address by; the server assigns one when the body omits it.
  await mutate(tabId, [{ kind: 'insert', values: { $document: bodyEjson } }]);
}

export async function deleteDocument(tabId: string, id: string): Promise<void> {
  await mutate(tabId, [{ kind: 'delete', key: { _id: id } }]);
}
