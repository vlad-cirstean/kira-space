import { type MessageKey, t } from '@theme/i18n';
import { copyOrReportError as sharedCopyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import type { MenuItem } from '@workbench/state/contextMenu';
import { beautifyJson } from '../../beautify';
import { parseIdLabel, toPlainJson, toRelaxedText, toShellText } from '../shared/document/ejson';
import { deleteDocument } from './mutations';
import { useDocumentViewStore } from './state';

// P19 D6 (parity half): the row's body is already canonical extended JSON (ejson.ts's own
// header rule) — re-indented through beautify.ts's JSON scanner, falling back to the raw body if
// it does not scan (a truncated body, say).
function prettyJson(text: string): string {
  const r = beautifyJson(text, 'indented');
  return r.ok ? r.text : text;
}

// A rejected clipboard write (denied permission, an unfocused window) must not vanish the same
// way an unhandled promise rejection would — ContextMenu.vue's `onItemClick` is never awaited by
// its own `@click` binding, so with nothing here to catch it, Copy would fail with zero visible
// feedback (the `actionError` exists for exactly this).
function copyOrReportError(tabId: string, text: string, announce: MessageKey): Promise<void> {
  return sharedCopyOrReportError(
    text,
    (message) => useDocumentViewStore().setActionError(tabId, message),
    () => {
      useDocumentViewStore().setActionError(tabId, null);
      announceApp(t(announce));
    },
  );
}

export function confirmDeleteDocument(id: string): Promise<boolean> {
  return useConfirmDialogStore().confirm({
    title: t('studio.documents.deleteConfirm.title'),
    message: [
      t('studio.documents.deleteConfirm.body', { id }),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
}

// §8.10's "Document" row: Expand all, Collapse all, Copy document, Copy _id, Edit, Delete —
// shared by the per-row context menu and (expand/collapse all only) the toolbar.
export function rowMenu(
  tabId: string,
  id: string,
  body: string,
  // A thunk, not an already-decoded array — right-click used
  // to decode every row's `_id` (idsOf, a full page walk through the page store's cached/
  // cachedView) up front regardless of whether Expand/Collapse all is ever picked, undoing the
  // visible-window pruning in one click. setAllExpanded's own `true` branch doesn't even read its
  // ids argument, so Expand all now costs nothing at all.
  allIds: () => string[],
  onEdit: () => void,
  // Edit is shown but disabled — with a label saying why — rather than omitted, mirroring
  // keyvalue/menu.ts's own `editable`/label pair: a row's own Edit icon and this menu entry must
  // agree, or right-clicking would offer an action the toolbar button already refused.
  editGate: { editable: boolean; label: string },
  // Same reasoning as editGate — the row's own Delete icon (DocumentView.vue's
  // canDelete/deleteTitle, itself caps.canDelete combined with the connection's readOnly flag)
  // used to gate only the toolbar button, leaving this menu's own Delete item always enabled and
  // able to fire a write the row button next to it already refuses.
  deleteGate: { deletable: boolean; label: string },
): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'expand-all',
      label: t('common.action.expandAll.label'),
      // setAllExpanded's own `true` branch never reads its ids argument — no reason to force
      // allIds()'s whole-page decode just to hand it a value it drops.
      run: () => useDocumentViewStore().setAllExpanded(tabId, [], true),
    },
    {
      type: 'item',
      id: 'collapse-all',
      label: t('common.action.collapseAll.label'),
      run: () => useDocumentViewStore().setAllExpanded(tabId, allIds(), false),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'toggle-expanded',
      label: t('studio.documents.menu.toggleExpanded.label'),
      run: () => useDocumentViewStore().toggleExpanded(tabId, id),
    },
    { type: 'separator' },
    {
      // One named-format submenu, mirroring the SQL grid's own `Copy row(s) ▸`
      // shape (console/menu.ts's tabularRowMenu) — a user choosing between mongosh syntax and
      // `{"$oid": …}` has to be told which is which, not read two unlabelled sibling items.
      // Leaf ids preserved (`copy-document`/`copy-as-json`) so existing specs keep passing.
      //
      // Real-interaction fix (reported bug — the normal/default copy button still produced
      // `$oid`/`$date`-wrapped JSON): Plain JSON is new and listed first — none of the three
      // existing formats could become it. Canonical Extended JSON must keep every wrapper intact
      // (interaction.spec.ts/autocomplete.spec.ts lock in that `copy-as-json` round-trips through
      // mongoimport/mongosh, and still contains `"$oid"`); Relaxed Extended JSON is still Extended
      // JSON per spec (autocomplete.spec.ts locks in that it keeps `$oid` wrapped too — "no relaxed
      // variant for ObjectId"); Shell mode is mongosh syntax, not JSON at all. ejson.ts's own
      // `toPlainJson` strips every `$`-prefixed wrapper down to its plain-JSON equivalent instead.
      type: 'submenu',
      id: 'copy-document-submenu',
      label: t('studio.copy.document.label'),
      items: [
        {
          type: 'item',
          id: 'copy-plain-json',
          label: t('studio.copy.plainJson.label'),
          run: () => copyOrReportError(tabId, toPlainJson(body), 'studio.copy.plainJson.announce'),
        },
        {
          type: 'item',
          id: 'copy-document',
          label: t('studio.copy.shellMode.label'),
          // P27 D12: the shell form (ObjectId(...), ISODate(...), ...) — what the tree already
          // shows, and what saveDocumentEdit/parseDocumentLiteral already accept back.
          run: () => copyOrReportError(tabId, toShellText(body), 'studio.copy.shellMode.announce'),
        },
        {
          type: 'item',
          id: 'copy-as-json',
          label: t('studio.copy.canonicalJson.label'),
          // P19 D6: canonical extended JSON, not relaxed and not shell — it's what the app
          // already has in hand (no re-encode) and what mongoimport/mongosh accept.
          run: () =>
            copyOrReportError(tabId, prettyJson(body), 'studio.copy.canonicalJson.announce'),
        },
        {
          type: 'item',
          id: 'copy-relaxed-json',
          label: t('studio.copy.relaxedJson.label'),
          run: () =>
            copyOrReportError(tabId, toRelaxedText(body), 'studio.copy.relaxedJson.announce'),
        },
      ],
    },
    {
      type: 'item',
      id: 'copy-id',
      label: t('common.action.copyId.label'),
      // The shell form is also what turns *Copy _id* into a working filter (F14, D12/D15): paste
      // it as `{ _id: ObjectId("...") }` and resolveEjsonWrappers/the shell constructor resolve it
      // to the same value either way.
      run: () => copyOrReportError(tabId, parseIdLabel(id).text, 'common.copy.copiedId.announce'),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'edit-document',
      label: editGate.editable ? t('common.action.edit.label') : editGate.label,
      disabled: !editGate.editable,
      run: onEdit,
    },
    {
      type: 'item',
      id: 'delete-document',
      label: deleteGate.deletable ? t('common.action.delete.label') : deleteGate.label,
      danger: true,
      disabled: !deleteGate.deletable,
      // P43 F6/D8: this runs inside contextMenu.ts's own `void item.run()` — an unhandled
      // rejection there is guaranteed, not merely possible, so the catch belongs here.
      run: async () => {
        if (!(await confirmDeleteDocument(id))) return;
        try {
          await deleteDocument(tabId, id);
          useDocumentViewStore().setActionError(tabId, null);
        } catch (err) {
          useDocumentViewStore().setActionError(tabId, errorText(err));
        }
      },
    },
  ];
}
