// setAllExpanded's collapse branch replaced state.expanded
// wholesale with a fresh map containing only the ids it was handed (the currently rendered page),
// discarding every `false` entry for a document on any other page — an absent key means expanded,
// so paging away and back re-expanded everything collapsed there. state.expanded is persisted tab
// state, so this survived a restart as the wrong value too.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';
import { seedConnectionRecord } from './support/connectionRecord.ts';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
// createTabsStore's saveIfChanged now serialises every save through one
// persistent chain — a real (never-settling in this harness) control.tabsSave triggered
// incidentally by opening a tab below would otherwise wedge every later spec's own tabsSave
// assertions for the rest of the process. This spec doesn't test persistence, so give it a
// benign default -- set *before* restoreAfterEach snapshots control, not in beforeEach: the
// enqueued save this triggers can settle on a tick past this file's own afterEach (confirmed
// via stream-count-honors-filter.spec.ts's identical hazard: only reproduces with ~90+ other
// spec files loaded, never in isolation), so a beforeEach/afterEach-scoped override races the
// chain back onto the hanging default and wedges it permanently for every later spec in the
// process.
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/studio/state/tabs');
const tabsStore = useTabsStore();
const { useDocumentViewStore } = await import('../../../frontend/src/studio/views/documents/state');
const documentViewStore = useDocumentViewStore();

describe('setAllExpanded merges rather than replaces state.expanded (finding 13)', () => {
  test('collapsing page 2 does not re-expand documents collapsed on page 1', () => {
    seedConnectionRecord(connectionsStore, 'conn-doc');
    const tabId = tabsStore.openDocumentTab('conn-doc', 'db/coll', { newTab: true })!.id;

    documentViewStore.setAllExpanded(tabId, ['p1-a', 'p1-b'], false); // collapse all on page 1
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-b')).toBe(false);

    documentViewStore.setAllExpanded(tabId, ['p2-a', 'p2-b'], false); // collapse all on page 2

    // Page 1's own collapsed documents must still be collapsed — the pre-fix bug replaced the
    // whole map with only page 2's ids, silently re-expanding page 1.
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-b')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-b')).toBe(false);
  });

  test('a single per-document collapse on page 1 survives a "collapse all" on page 2', () => {
    seedConnectionRecord(connectionsStore, 'conn-doc');
    const tabId = tabsStore.openDocumentTab('conn-doc', 'db/coll', { newTab: true })!.id;

    documentViewStore.toggleExpanded(tabId, 'p1-a'); // collapse just this one document, on page 1
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(false);

    documentViewStore.setAllExpanded(tabId, ['p2-a', 'p2-b'], false); // collapse all on page 2

    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-b')).toBe(false);
  });

  test('"expand all" still clears the whole map — every page, not just the current one', () => {
    seedConnectionRecord(connectionsStore, 'conn-doc');
    const tabId = tabsStore.openDocumentTab('conn-doc', 'db/coll', { newTab: true })!.id;

    documentViewStore.setAllExpanded(tabId, ['p1-a'], false);
    documentViewStore.setAllExpanded(tabId, ['p2-a'], false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(false);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-a')).toBe(false);

    documentViewStore.setAllExpanded(tabId, ['p2-a'], true); // "Expand all", called only with the current page's ids

    expect(documentViewStore.isDocumentExpanded(tabId, 'p1-a')).toBe(true);
    expect(documentViewStore.isDocumentExpanded(tabId, 'p2-a')).toBe(true);
  });
});
