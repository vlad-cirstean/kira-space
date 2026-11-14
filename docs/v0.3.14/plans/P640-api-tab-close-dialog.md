# P640: API module, dialog when closing an unsaved tab

Stream B of batch 3 (`batch-3.md`). Base `ee9e0d597`. Worktree `/home/user/kira-space-stream-b`,
branch `stream-b-v0.3.14-3`.

Spec row: closing an API-module tab asks to confirm when the tab is brand new and unsaved, and when
it holds unsaved changes to a saved request. Dialog offers Save, Discard, Cancel. Tab closes only
after Save or Discard. Dialog is shadcn-vue. Flow half: save then close persists, discard leaves the
saved file unchanged. UI spec for both cases.

## Current behavior

- Every close path funnels into `createTabsStore` (`packages/workbench/src/state/createTabsStore.ts`):
  tab `x` and middle click (`TabStrip.vue` `onClose`/`onMiddleClick` call `host.tabs.closeTab`),
  Ctrl+W (`closeActiveTab` in `apps/kira-space/frontend/src/state/tabs.ts` calls `closeTab`), and the
  tab menu's Close, Close others, Close to the right, Close all (`closeOthers`, `closeToTheRight`,
  `closeAll`). `dropTabs` is system cleanup (deleted connection) and never asks.
- Each path counts dirty tabs through `host.kinds[kind].isDirty(tabId)` (`dirtyCount`) and, when any,
  asks `askDiscard(count)`: `useConfirmDialogStore().confirm` with one Discard button and Cancel.
- `STUDIO_TAB_KINDS['http-request']` and `['grpc-request']`
  (`apps/kira-space/frontend/src/studio/state/tabKinds.ts`) declare no `isDirty`. So an API tab
  always closes silently, edited or not.
- Dirty is computed only inside the mounted view: `HttpRequestView.vue` `dirty =
  isDirty(tab.state, saved)` with `saved` from `useSavedRequest(itemId)` (TanStack Query, key
  `apiSavedRequestKey(itemId)`); `GrpcRequestView.vue` same with `isGrpcDirty` and
  `apiSavedGrpcRequestKey`. Both `isDirty` helpers return false for `saved === null`, so a tab never
  bound to a saved row (`itemId === null`) never reads dirty.
- Save: `useRequestTabSave.onSave` writes a bound tab through `collectionsStore.saveRequest(itemId,
  name, body, seenSaved)` (or `saveGrpcRequest`); an unbound or orphan tab goes to Save as… via
  `useSaveRequestDialogStore().openSaveDialog` (`openSaveGrpcDialog`), whose dialog
  (`SaveRequestDialog.vue`) calls `collectionsStore.submitSaveDialog`, which creates the row, binds
  the tab (`itemId`, `name`) and calls `closeSaveDialog()`. `saveRequest` swallows failures into
  `state.error` and returns early (writes nothing) when the user declines an overwrite in
  `guardedSavedWrite` (that confirm uses the same `useConfirmDialogStore`).
- `seenSaved` (`useSeenSaved`) is the Decision 3 guard: the saved request the tab last synced with.
  It lives only while the view is mounted; a remount restarts it from the current cache.
- Incognito tabs cannot save (`onSave`/`onSaveAs` no-op) and are never persisted.
- App quit and window close: tab state (unsaved edits included) persists in the `tabs` table and is
  restored, so nothing is lost there. Out of scope, no prompt on quit.

## Design

### Which tabs ask

API tab = `http-request` or `grpc-request` kind. New module computes a three-way state per tab:

- `clean`: incognito (no Save route; ephemeral by choice), or bound and equal to its saved row, or
  unbound and equal to a fresh default request (an untouched new tab holds nothing to save).
- `dirty`:
  - modified: `itemId` set, saved row loaded (non-null), `isDirty`/`isGrpcDirty` true;
  - new unsaved: `itemId === null`, or `itemId` set but the row is gone (orphan, saved query data
    `null`), and the request differs from a default one (`toSavedRequest(state)` vs
    `toSavedRequest(defaultHttpRequestTabState())` through `sameRequest`-equivalent compare; gRPC with
    `toSavedGrpcRequest`/`defaultGrpcRequestTabState`). Compare through the existing helpers: export
    `sameRequest` from `packages/api-core/src/http/saved.ts` and `sameGrpcRequest` from
    `packages/api-core/src/grpc/saved.ts` (both module-private today) via `packages/api-core/src/index.ts`.
- `unknown`: `itemId` set and no cache entry yet (`queryClient.getQueryData(key) === undefined`), e.g.
  a restored background tab never mounted since start. Its persisted state can hold unsaved edits, so
  silence would lose them.

Decision (state it in the Result): an untouched blank new tab closes without asking. Reason: nothing
to save or lose; asking there turns every open-and-close into a dialog. Spec wording "brand new and
unsaved" is read as a new tab holding content. User may override.

Kind `isDirty(tabId)` returns `state !== 'clean'` (sync, from tab state plus query cache). Kind's new
`confirmClose(tabId)` resolves `unknown` first: `await loadSavedRequest(itemId)` /
`loadSavedGrpcRequest(itemId)` (exist in `apiQueries.ts`), recompute, and if now `clean` resolve true
with no dialog.

### Generic seam: kind-owned close question

`packages/workbench/src/tabs/types.ts` `TabKindDef` and `createTabsStore.ts` `KindLookup` gain:

```ts
/** Asks this kind's own close question for a dirty tab (Save, Discard, Cancel). True closes it.
 *  Absent: the generic discard confirm asks. Called only when `isDirty` is true. */
confirmClose?(tabId: string): Promise<boolean>;
```

`createTabsStore` gets one helper used by all four close paths in place of `dirtyCount` +
`askDiscard`:

```ts
async function confirmCloses(ids: Iterable<string>): Promise<boolean> {
  const own: string[] = [];
  let generic = 0;
  for (const id of ids) {            // tab order
    const tab = find(id); const kind = tab && host.kinds[tab.kind];
    if (kind?.isDirty?.(id) !== true) continue;
    if (kind.confirmClose) own.push(id); else generic++;
  }
  for (const id of own) if (!(await host.kinds[find(id)!.kind]!.confirmClose!(id))) return false;
  return generic === 0 || askDiscard(generic);
}
```

Keep the documented contract "with none dirty they close synchronously": each path computes whether
any id is dirty first and awaits only then (as today with `dirty > 0 &&`). After the await, each path
already recomputes its close set (keep that). Cancel on any question closes nothing; a tab saved
earlier in the same bulk close stays open but clean. Iterate `own` in strip order
(`tabsState.tabs` order filtered by the set), so the user is asked tab by tab, left to right.

### Three-choice confirm in the existing store and dialog

Reuse the one confirm look (`ConfirmDialogView.vue`, shadcn-vue `Dialog`/`Button`) and store; no new
dialog component.

- `packages/workbench/src/state/confirmDialog.ts`: add `secondaryLabel: ''` to state; internal
  resolver becomes `(choice: ConfirmChoice) => void` with `type ConfirmChoice = 'confirm' |
  'secondary' | 'cancel'`. New action:

  ```ts
  export interface ChooseOptions {
    title: string; message: string; confirmLabel: string; secondaryLabel: string;
    tone: 'primary' | 'danger';
  }
  choose(options: ChooseOptions): Promise<ConfirmChoice>
  ```

  `confirm()` keeps its signature and `Promise<boolean>` (wraps: `choice === 'confirm'`), clears
  `secondaryLabel`. F9 rule holds for both: a second call settles the pending one as `cancel`/false.
  Add `acceptSecondary()` (finish `'secondary'`, ignored while `busy`). `run` stays confirm-only;
  `choose` never sets it.
- `ConfirmDialogView.vue`: optional prop `secondaryLabel?: string` (default `''`), emit `secondary`,
  testid key `secondary` (default `confirm-dialog-secondary`). Footer order: Cancel, then secondary
  (`variant="dialog"`, rendered only when `secondaryLabel`), then confirm. One emphasised button rule
  (ARCHITECTURE "a dialog footer has one emphasised button") holds: only confirm is
  `dialog-primary`. Touch sizing applies to the new button as to the others.
- `ConfirmDialog.vue`: pass `:secondary-label="confirmDialogStore.secondaryLabel"`,
  `@secondary="confirmDialogStore.acceptSecondary()"`.

Esc, header X and outside click map to `cancel` (existing `update:open` handling). Primary tone puts
initial focus on Save, so Enter saves.

### API close store

New `apps/kira-space/frontend/src/studio/api/state/tabClose.ts`: Pinia store `useApiTabCloseStore`,
one concern (API tab close guard).

- State: `seen: Record<string, HttpSavedRequest | GrpcSavedRequest>`; actions `publishSeen(tabId,
  value)`, `forgetSeen(tabId)`. Composable `usePublishSeenSaved(tabId: () => string, seen:
  Ref<T | null>)` in the same file: `watchEffect` publishes non-null, `onScopeDispose` forgets (shape
  of `views/shared/editDirty.ts`). `HttpRequestView.vue` and `GrpcRequestView.vue` call it with their
  `seenSaved`. Close path uses `seen[tabId]` and falls back to the cached saved row (what a remount
  would seed `useSeenSaved` with), so the Decision 3 guard stays for the mounted tab.
- `closeState(tabId): 'clean' | 'dirty' | 'unknown'` per rules above. Tab lookup:
  `useTabsStore().tabs` by id (add `findHttpRequestTab`/`findGrpcRequestTab` beside
  `findConsoleTab` in `studioTabs.ts` only if the plain lookup cannot narrow the type cleanly).
  Incognito: `useTabIncognitoStore().isIncognito(tabId)`.
- `confirmClose(tabId): Promise<boolean>`:
  1. `unknown`: load saved row, recompute. `clean`: return true.
  2. `useConfirmDialogStore().choose({ title: t('studio.api.tabClose.title', { name: title }),
     message: t(modified ? 'studio.api.tabClose.modified.body' : 'studio.api.tabClose.unsaved.body'),
     confirmLabel: t('common.action.save.label'), secondaryLabel: t('common.action.discard.label'),
     tone: 'primary' })`. `title` from `httpRequestTitle(state)` / `grpcRequestTitle(state)`.
  3. `cancel`: false. `secondary` (Discard): true; nothing written.
  4. `confirm` (Save), after the choice dialog has closed (the overwrite confirm reuses the store):
     - modified: `await collectionsStore.saveRequest(itemId, state.name || title,
       toSavedRequest(state), seen)` (gRPC: `saveGrpcRequest` with `toSavedGrpcRequest`); return its
       boolean. False (write failed, error in `collectionsStore.error` as today, or overwrite
       declined) keeps the tab open with the edit.
     - new unsaved: `saveRequestDialog.openSaveDialog(tabId, state.name || title,
       toSavedRequest(state))` (gRPC: `openSaveGrpcDialog`), then `return await
       saveRequestDialog.waitForClose()`. True only after `submitSaveDialog` created and bound the
       row; dismissing the Save as… dialog returns false, tab stays.
- `saveRequestDialog.ts`: add `waitForClose(): Promise<boolean>` (settles at the next
  `closeSaveDialog`; a newer wait or open settles a pending one as false) and change
  `closeSaveDialog(saved = false)`. `submitSaveDialog` (`collections.ts`) calls
  `closeSaveDialog(true)` in both protocol branches. `SaveRequestDialog.vue` keeps
  `closeSaveDialog()` (false) on dismiss.
- `collections.ts`: `saveRequest` and `saveGrpcRequest` return `Promise<boolean>`: true after the
  write and cache update, false on declined overwrite or caught failure. `useRequestTabSave.ts`
  widens `save` to `(itemId, name, body) => unknown`; the views keep ignoring the result.
- `tabKinds.ts`: both API kinds get `isDirty: (id) => useApiTabCloseStore().closeState(id) !==
  'clean'` and `confirmClose: (id) => useApiTabCloseStore().confirmClose(id)`.

No new IPC, no Go change. Backend behavior used (create, save, tabs save) exists.

### Copy

`packages/theme/src/i18n/en.json`, under `studio.api`:

- `tabClose.title`: `Save changes to {name}?`
- `tabClose.modified.body`: `This request has unsaved changes. Closing without saving loses them.`
- `tabClose.unsaved.body`: `This request was never saved. Closing without saving loses it.`

Buttons reuse `common.action.save.label`, `common.action.discard.label`, `common.action.cancel.label`.
Run the repo copy checks (`scripts/copy`); adjust wording only if a rule there refuses it.

## Files

| File | Change |
|---|---|
| `packages/workbench/src/state/confirmDialog.ts` | `choose`, `secondaryLabel`, `acceptSecondary`, choice resolver |
| `packages/workbench/src/components/ConfirmDialogView.vue` | optional secondary button, emit, testid |
| `packages/workbench/src/components/ConfirmDialog.vue` | wire secondary |
| `packages/workbench/src/tabs/types.ts` | `TabKindDef.confirmClose` |
| `packages/workbench/src/state/createTabsStore.ts` | `KindLookup.confirmClose`, `confirmCloses`, four close paths |
| `packages/api-core/src/http/saved.ts`, `grpc/saved.ts`, `index.ts` | export `sameRequest`, `sameGrpcRequest` |
| `apps/kira-space/frontend/src/studio/api/state/tabClose.ts` (new) | store and `usePublishSeenSaved` |
| `apps/kira-space/frontend/src/studio/api/state/saveRequestDialog.ts` | `waitForClose`, `closeSaveDialog(saved)` |
| `apps/kira-space/frontend/src/studio/api/state/collections.ts` | boolean returns, `closeSaveDialog(true)` |
| `apps/kira-space/frontend/src/studio/views/shared/request/useRequestTabSave.ts` | widen `save` type |
| `apps/kira-space/frontend/src/studio/views/httprequest/HttpRequestView.vue` | publish seen |
| `apps/kira-space/frontend/src/studio/views/grpcrequest/GrpcRequestView.vue` | publish seen |
| `apps/kira-space/frontend/src/studio/state/tabKinds.ts` | `isDirty`, `confirmClose` on both API kinds |
| `apps/kira-space/frontend/src/studio/state/studioTabs.ts` | only if a typed finder is needed |
| `packages/theme/src/i18n/en.json` | `studio.api.tabClose.*` (shared file, see batch-3) |
| `docs/ARCHITECTURE.md` | shared file, see Docs |
| `apps/kira-space/internal/flows/apiflow/tabclose_test.go` (new) | flow half |
| `apps/kira-space/tests/contract/api-tab-close.json` (new, written by the flow test) | contract |
| `apps/kira-space/tests/ui/studio/api-tab-close.spec.ts` (new) | UI half |
| `apps/kira-space/tests/ui/studio/support/dialogs.ts` | `chooseSecondary(page)` helper beside `acceptConfirm` |

## Tests

IPC split applies: Save and Discard outcomes cross the IPC boundary (create/save item, tabs save).
Backend code does not change; the flow half pins the backend results the UI drives, and the contract
feeds the UI spec its args and responses.

### Flow half: `apiflow.TestTabCloseSaveOrDiscard`, scenario `api-tab-close`

Follow `twowindows_test.go` (contract keys, `flowharness.Mask`) and `windowflow/tabs_scope_test.go`
(valid window key for `app.W.Tabs`). One test, three parts:

1. Modified, Save: collection, item `shared` with URL `https://x.test/saved`. `Tabs.Save` the window
   with one `http-request` tab bound to the item. Save the edit: `SaveRequest{ItemID, Name: "shared",
   Request: url https://x.test/edited, Expect: <the read>, ExpectName: "shared"}`; contract
   `args:SaveRequest#close` and `SaveRequest#close` (masked ids and times). Close: `Tabs.Save` with no
   tabs. Oracles: `Tabs.List` empty; `GetRequest` URL is `https://x.test/edited`; collection file on
   disk contains it.
2. Modified, Discard: second item URL `https://x.test/kept`, tab bound to it with an edited state.
   Read the collection file bytes, close (`Tabs.Save` without the tab), no save call. Oracles: file
   bytes identical; `GetRequest` URL `https://x.test/kept`; tab gone.
3. New unsaved, Save: unbound tab. `CreateItem{Kind: "request", Name: "Fresh", Request: url
   https://x.test/fresh}`; contract `args:CreateItem#close` and `CreateItem#close`. Close. Oracles:
   `List` holds item `Fresh`; file contains the URL; tab gone. New unsaved, Discard writes nothing:
   no backend call exists to assert beyond the tab row going away; covered by the UI half (no
   create call logged).

Run once with `KIRA_CONTRACT=write` (from `apps/kira-space`, see DEV_ENVIRONMENT "Contract
fixtures") to write `tests/contract/api-tab-close.json`, then once without to pass against it.

### UI half: `tests/ui/studio/api-tab-close.spec.ts`

Header comment names the backend half. Mock control as in `api-request-conflict.spec.ts`
(`collectionsList`, `variablesListEnvironments`, `collectionsGetRequest`, save/create snapshots with
contract args and responses). Use `openHttpMode`, `collectionRow`, `openHttpModeAndNewRequest`.
Cases:

1. Modified tab, tab `x`: dialog (`confirm-dialog`) shows title `Save changes to shared?`, the
   modified body, buttons Save (`confirm-dialog-confirm`), Discard (`confirm-dialog-secondary`),
   Cancel (`confirm-dialog-cancel`).
   - Cancel: dialog gone, tab present, `http-dirty` visible, no `collectionsSaveRequest` in
     `control.log()`.
   - Discard: tab gone, no save call, last `tabsSave` args hold no tab with that id.
   - Save: one `collectionsSaveRequest` whose args match `args:SaveRequest#close` (URL, name,
     guard), then tab gone.
2. New unsaved tab (new request, URL typed), Ctrl+W: dialog with the unsaved body.
   - Save: `save-request-dialog` opens with the suggested name; submit creates through
     `collectionsCreateItem` matching `args:CreateItem#close`; tab gone. Dismissing the Save as…
     dialog instead leaves the tab open.
   - Discard: tab gone, no create call.
   - Cancel: tab stays.
3. Untouched new tab closes with no dialog.
4. Middle click on a modified tab shows the dialog (path coverage).
5. Close all with two edited API tabs: Cancel on the first question closes nothing; Discard on both
   closes both.
6. Save that fails (`collectionsSaveRequest` error snapshot): tab stays open, error shows in the
   view's existing error strip.
7. gRPC modified tab (`grpcTab` helper bound to a saved gRPC item): close asks; Discard closes.

No unit test: logic is a short decision table covered by both halves.

## Implementer steps

1. Workbench seam: `confirmDialog.ts` `choose`, `ConfirmDialogView.vue`, `ConfirmDialog.vue`.
   Commit `feat: three-choice confirm dialog`.
2. `types.ts` and `createTabsStore.ts` `confirmClose` + `confirmCloses` on all four close paths.
   Commit `feat: kind-owned tab close question`.
3. api-core exports; `collections.ts`, `saveRequestDialog.ts`, `useRequestTabSave.ts`; new
   `tabClose.ts`; views publish seen; `tabKinds.ts`; en.json keys. Commit `feat: ask Save, Discard or
   Cancel when closing an unsaved API tab`.
4. Flow test, generate contract, rerun. Commit `test: api tab close flow half`.
5. UI spec and dialog helper. Commit `test: api tab close UI spec`.
6. ARCHITECTURE.md (see Docs). Commit `docs: API tab close question`.
7. Fix whatever verification finds as follow-up commits. No `--no-verify` on a final commit.

## Docs

`docs/ARCHITECTURE.md` (shared file; own hunks only):

- UI bullet "Each host mounts one ConfirmDialog … Tabs ask before closing a dirty tab
  (TabKindDef.isDirty)." extends: a kind with `confirmClose` asks its own question instead; the
  confirm store's `choose()` adds a secondary button (one emphasised button stays).
- API module section (where Save / Save as… and Decision 3 are described): closing a dirty API tab
  asks Save, Discard, Cancel; dirty rules (modified, new with content, unknown resolved by loading);
  untouched blank and incognito tabs close silently; quit keeps tab state, so no quit prompt.

## Verification

- `bun run typecheck`, `bun run lint`, copy checks.
- `go test ./apps/kira-space/internal/flows/apiflow/ -run TestTabCloseSaveOrDiscard` passes against
  the committed contract.
- Playwright: `api-tab-close.spec.ts`, plus touched neighbours: `api-request-conflict.spec.ts`,
  `grpc-request-conflict.spec.ts`, `http-request.spec.ts`, `grpc-request.spec.ts`, `tabs.spec.ts`,
  `consistency-git.spec.ts` (generic discard confirm unchanged), `copy-wording.spec.ts`.
- Grep oracles: `confirmClose` defined in `types.ts` and `createTabsStore.ts` and used for both API
  kinds in `tabKinds.ts`; `choose(` has a real caller in `tabClose.ts`; `closeSaveDialog(true)` in
  both `submitSaveDialog` branches; no new `<style>` block, no new dialog component.
- Orchestrator checks Result states the untouched-blank-tab decision.
