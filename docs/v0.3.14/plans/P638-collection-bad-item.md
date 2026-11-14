# P638 Collection file with a bad item (F-A15, J-COLL-003)

A hand-edited `<id>.collection.yaml` with one bad item loads its valid items. Each skipped item is
reported with its line and reason. Removes the ARCHITECTURE.md Known open items entry "Collection
file with one bad item serves no rows on first load (v0.3.13)". Flow half plus UI half.

Discovery: `codegraph_explore` (`configfs.Folder`/`entryFor`/`lastGood`/`record`, `YAMLSchema`,
`problemFor`, apifiles `Store`/`syncLocked`/`noteProblems`/`View`/`editLocked`/`projection`,
`itemKindRule`/`itemUniqueRule`, `CollectionsPanel`, `ConfigFileProblem`), then reads of
`apifiles/{store,rules,project,form,format}.go`, `configfs/schema.go`, `collection.schema.json`,
`apiflow/configfiles_test.go`, `ApiConfigProblems.vue`, `api-config-files.spec.ts`, contract
`api-files.json`, v0.3.13 journey text (`git show 440706b3e^:docs/v0.3.13/plans/...`).

## Current behavior

- `apifiles.Store` holds `configfs.Folder[CollectionFile]` with `Decode: dec.collection`
  (`rules.go` `newDecoders`: `configfs.YAMLSchema` plus `itemKindRule`, `itemUniqueRule`,
  `varUniqueRule`, `varKindRule`, wrapped by `withStem`).
- `configfs.YAMLSchema` decode: parse, walk to `any` tree, JSON Schema `validate`, `refChecker`,
  strict JSON decode into T, rules. First problem fails the whole file.
- `Folder.entryFor`: problem means no `record` (no projection), `Entry.Value` = last good JSON from
  `config_last_valid` if any. First load bad means no last good, so no rows (F-A15).
- `noteProblems` keeps `file -> "line:message"`; a change flags `Changes.Config` (UI reloads
  `ApiConfigService.View`). `View` answers `FileView{problem, lastGood}`.
- `editLocked` refuses app edits on a file with a problem (`BrokenError`, bridge maps to
  `backend.studio.apiFiles.fileBroken.body`), and refuses an edit whose result fails `dec.collection`
  (`InvalidError`).
- `projection.Record` -> `recordCollection` -> `syncItems`: rows keyed by `item_key`; rows whose
  key left the file are deleted (response history cascades). `flatten` sets `sort_order` = slice
  index. `flatten` also runs `SavedRequest.Validate`/`SavedGrpcRequest.Validate`; every check there
  is already a schema enum or `minLength`, so a schema-valid item never fails it (no new rule).
- UI: `ApiConfigProblems.vue` (mounted in `CollectionsPanel.vue` twice and `EnvironmentsView.vue`)
  shows one `ConfigFileBar` per file with `problem !== null` plus the last good hint.
- Tests: `apiflow.TestHandEditedCollectionWithBadEntry` pins today's whole-file behavior.
  `TestApiConfigFiles` step 3 breaks the file by inserting an item that duplicates id `list`; with
  this phase that becomes an item-level skip, so step 3 must switch to a whole-file break.

## Design

### What counts as an item problem

A problem is item-scoped when its JSON pointer starts with one or more `/items/<n>` pairs. The
skipped element is the deepest such prefix: `/items/2/http/headers/0/name` skips `/items/2`;
`/items/2/items/0/id` skips `/items/2/items/0` only (folder and siblings load); `/items/2/items`
(children not an array) skips folder `/items/2`. Anything else stays whole-file, exactly as today:
YAML syntax error, anchors or merge keys, file over cap, bad file name, `/name`, `/variables/...`,
unknown top-level key, `/items` not an array, strict-JSON-decode failure (no path).

Sources covered: schema problems (`problemFor`; Required sets `Path` to the missing field, still
under the item), `refChecker` problems, rule problems (`itemKindRule` at `/items/n`,
`itemUniqueRule` at `/items/n/id`).

Duplicate ids: `itemUniqueRule` already reports the later occurrence, so first in file order wins,
later ones are skipped. Same "first wins" precedent as `repocfg.duplicateProblem`. Rule unchanged.

### configfs: skip loop (generic, additive)

New in `internal/configfs/schema.go` (helpers in new `skip.go`):

```go
// Skip lets a YAMLSchemaSkip decode drop bad array elements instead of failing the file.
type Skip struct {
	// Element is the pointer of the droppable element path falls in; false keeps the problem whole-file.
	Element func(path string) (string, bool)
	// StandIn is a valid value put in a dropped element's place, so later elements keep their
	// pointers and lines. doc is the whole parsed document; n counts drops from 0.
	StandIn func(doc any, n int) any
	// Max is the most elements dropped; one more problem fails the file.
	Max int
}

// Skipped is one dropped element: its pointer, its parsed value and the first problem found in it.
type Skipped struct {
	Pointer string
	Value   any
	Problem Problem
}

// YAMLSchemaSkip is YAMLSchema with Skip. The returned T still holds the stand-ins at the skipped
// pointers; the caller drops them. On a whole-file problem T is zero and skipped keeps what was
// dropped before it.
func YAMLSchemaSkip[T any](schemaJSON []byte, skip Skip, rules ...Rule[T]) (
	func(file string, src []byte) (T, []Skipped, *Problem), DocModel, error)
```

- `YAMLSchema` becomes a thin adapter over the shared core with a zero `Skip` (`Element == nil`
  never skips), returning `(T, *Problem)`. Behavior of every other module unchanged.
- Core: parse and walk once (problems there are whole-file). Then loop: `check(val)` = validate,
  `rc.check`, strict decode, rules (rule `Line` fill as today). `nil` returns. Else
  `ptr, ok := skip.Element(p.Path)`; return whole-file when `!ok`, when `len(skipped) == skip.Max`,
  when `ptr` equals or sits under an already skipped pointer (stand-in invalid: guard against an
  endless loop), or when `ptr` cannot be resolved/set in `val`. Otherwise append
  `Skipped{ptr, valueAt(val, ptr), *p}`, `setAt(val, ptr, skip.StandIn(val, n))`, loop.
- `nodeIndex`/`Locator` are built from the original YAML and never touched: stand-ins keep indices
  stable, so every later problem gets its true file line.
- `valueAt`/`setAt` walk `map[string]any` and `[]any` (walker output, `yamlnode.go`) by unescaped
  pointer tokens (`~1`, `~0`).
- Cost: each drop re-validates the document. `Max` bounds it (apifiles: 100).

### apifiles: two collection decoders

`rules.go`:

- `collectionElement(path)`: regex `^(?:/items/\d+)+`, longest match.
- `collectionStandIn(doc, n)`: `map[string]any{"id": free, "name": "skipped", "items": []any{}}`
  (folder: satisfies `itemKind`). `free` = `kira-skipped-<n>`, suffixed until no `id` string anywhere
  under `doc`'s `items` trees uses it (so `itemUniqueRule` never trips on it).
- `const maxSkippedItems = 100`.
- `decoders.collectionLoad` (lenient, the Folder's decode): run `YAMLSchemaSkip`; on whole-file
  problem return it. Else build `CollectionFile.Skipped` (below) from `[]Skipped` and the T that
  still holds stand-ins, then prune stand-ins from T: outermost skipped pointers only (a pointer
  under another skipped pointer goes with it), removed in descending token order (numeric compare,
  token by token) so earlier pointers stay valid.
- `decoders.collection` (strict; used by `editLocked`, `AddItem`'s `fn`, `Form`, `import.go`):
  lenient core, then `len(skipped) > 0` returns `skipped[0].Problem` as the file problem, else the
  whole-file problem, else T. Equals today's first-problem behavior: the first loop iteration is the
  old single pass.
- Both wrapped in `withStem`.
- `cdoc.Rules` gains one doc line (AGENTS.md): `ruleItemSkip = "An item that breaks a rule is
  skipped: the rest of the file loads and the Api panel lists the item with its line. Of two items
  with the same id, the first one loads."`

`format.go`:

```go
// CollectionFile.Skipped lists items the load left out; empty for a fully valid file.
Skipped []SkippedItem `json:"skipped,omitempty"`

// SkippedItem is one item the load left out, with the first problem found in it.
type SkippedItem struct {
	Pointer   string   `json:"pointer"`
	ParentKey string   `json:"parentKey,omitempty"` // id of the enclosing folder; "" at top level
	Index     int      `json:"index"`               // position among its siblings in the file
	ID        string   `json:"id,omitempty"`        // the item's id when it is a string
	Name      string   `json:"name,omitempty"`      // the item's name when it is a string
	Keys      []string `json:"keys,omitempty"`      // every string id in the item's subtree, its own first
	Line      int      `json:"line"`
	Message   string   `json:"message"`
}
```

- `ID`, `Name`, `Keys` from `Skipped.Value`. `Index` = last pointer token. `ParentKey` = `ID` of the
  item at the parent pointer (tokens minus last two) in the stand-in-holding T; when that parent is
  itself skipped, the parent's raw id from its own `Skipped.Value`. Sorted by `Line`, then pointer.
- Rides in the projected JSON (`config_last_valid` and `recordCollection`'s input). Never rendered
  to YAML: every `RenderYAML(CollectionFile)` caller builds a fresh value with no skipped items.
  The schema's `additionalProperties: false` refuses a user-written top-level `skipped` key first.

### Projection: valid items load, a skipped item keeps its last good row

Load stays one transaction per file (`projection.Record` -> `s.tx`): valid items commit together.

- `recordCollection` passes `f.Skipped` to `syncItems`.
- `flatten(items, parentKey, skippedAt, out)`: `skippedAt` = `map[parentKey][]int` of skipped
  indices. A valid item's `order` is its file index: walk siblings with a position counter that
  steps over skipped indices. Keeps tree order identical to the file and leaves room for kept rows.
- `syncItems` keep set: after the main loop, for each skipped item, every key in `Keys` with an
  existing row not already kept is kept (not deleted). The row of `ID` itself, when kept here, gets
  `sort_order = Index` (one `UPDATE`); content, parent and name stay last good. A key a valid item
  already holds (duplicate case) is untouched by this step.
- Why keep: deleting a row cascades its response history and orphans open tabs. A typo in one
  item must not erase that; mirrors the file-level last-good rule. A skipped item with no prior row
  (first load) gets none.
- App edits stay refused while any item is skipped (next section), so `sort_order` gaps never meet
  a dense rewrite from `MoveItems`.

### Store: report, refresh, refuse edits

`store.go`:

- `Folder.Decode` = `dec.collectionLoad`.
- Signatures: `problemSig(p *configfs.Problem) string` (today's `"%d:%s"`, `""` for nil) and
  `collectionSig(e configfs.Entry[CollectionFile]) string` (problem sig; else, with skipped items,
  `"skip:"` + `line:message` joined by `\n`). `noteProblems` uses them, so a change in the skipped
  set flags `Changes.Config` and the UI refetches `View`. `noteProblems1(file, sig string)` takes the
  sig; callers: `editLocked` (two sites, `collectionSig(e)`), `form.go` `saveLocked` (collection
  `collectionSig(e)`, environment `problemSig(e.Problem)`).
- One `slog.Warn("api: items skipped", "scope", "config", "file", f, "count", n, "line", first)`
  in `noteProblems` when a file's sig changed and it has skipped items. Never item content.
- `FileView.Skipped []SkippedView` (`json:"skipped"`, always non-nil):
  `SkippedView{Line int, Index int, ID string, Name string, Message string}` (`pointer`, `keys`,
  `parentKey` stay internal). Filled only when `e.Problem == nil`: with a whole-file problem the
  value is last good and its old list is stale. `LastGood` stays `e.Problem != nil && e.Value != nil`.
- `editLocked`: after the `e.Problem` branch, `len(e.Value.Skipped) > 0` returns
  `&BrokenError{File: file, Line: e.Value.Skipped[0].Line}` (bridge already maps it to "Kira can't
  save into {file}. Line {line} has an error. Fix the file first."). The post-edit strict decode
  already turns an edit that would add a bad item into `InvalidError`. File never rewritten.
- `FileDetail` (`form.go`, variables dialog): unchanged. `Form` decodes strictly, so a draft with a
  bad item shows the item problem as today.

### UI

`packages/shared/domain/apiConfig.ts`: `ApiSkippedItem {line; index; id; name; message}` and
`ApiConfigFile.skipped: ApiSkippedItem[]` (required; Go always sends it).

`apps/kira-space/frontend/src/studio/api/ApiConfigProblems.vue` (stays `<script setup lang="ts">`,
Tailwind only, server state already through `useApiConfig` TanStack query, no new store):

- `partial = files.filter(f => f.problem === null && f.skipped.length > 0)`. Section shows when
  `broken.length + partial.length > 0`.
- Per partial file: shadcn-vue `Alert variant="warn"` `data-testid="api-config-skipped"` with
  `AlertTitle` (`studio.api.config.skipped.title`, count and file name), `AlertDescription` holding
  the hint and a `<ul>` of `<li data-testid="api-config-skipped-item">` rows
  (`studio.api.config.skipped.item`: line, label, message; label = `name || id || '#' + (index + 1)`),
  and `AlertAction` with a `Button variant="link" size="inline"` `data-testid="api-config-skipped-open"`
  calling `control.configFilesOpen({ folder: 'api', file })` (label `studio.api.config.openFile.label`).
- Tree unchanged: valid rows plus kept last good rows come from `CollectionsService.List`.

`packages/theme/src/i18n/en.json`, new keys:

- `studio.api.config.skipped.title`: `"{count} item in {file} isn’t loaded. | {count} items in {file} aren’t loaded."`
- `studio.api.config.skipped.hint`: `"The rest of the file is loaded. An item that loaded before keeps its last good version. Fix the file to load the rest."`
- `studio.api.config.skipped.item`: `"Line {line}, {label}: {message}"`

## Files

Expected touch list (exact paths):

- `internal/configfs/schema.go`
- `internal/configfs/skip.go` (new)
- `internal/configfs/skip_test.go` (new)
- `apps/kira-space/internal/apifiles/rules.go`
- `apps/kira-space/internal/apifiles/format.go`
- `apps/kira-space/internal/apifiles/store.go`
- `apps/kira-space/internal/apifiles/form.go`
- `apps/kira-space/internal/apifiles/project.go`
- `apps/kira-space/internal/flows/apiflow/configfiles_test.go`
- `apps/kira-space/tests/contract/api-files.json` (regenerated)
- `apps/kira-space/tests/contract/api-collection-skipped.json` (new, generated)
- `packages/shared/domain/apiConfig.ts`
- `apps/kira-space/frontend/src/studio/api/ApiConfigProblems.vue`
- `apps/kira-space/tests/ui/studio/api-collection-skipped.spec.ts` (new)
- `apps/kira-space/tests/ui/studio/api-config-files.spec.ts` (only if its local `ApiFile` interface
  or `valid` mapping needs `skipped`; spread already carries it)
- `packages/theme/src/i18n/en.json`
- `docs/ARCHITECTURE.md`
- `docs/v0.3.14/plans/P638-collection-bad-item.md` (result notes only, if any)

Not touched: `apps/kira-space/internal/bridge/*` (`ApiConfigService.View` returns `apifiles.View`
as is; `BrokenError` mapping exists), `tests/ui/support/ipcChannels.ts` and `mockRuntime.ts` (no
new IPC channel; `apiConfigView` exists), `CollectionsTree.vue`, `SPEC.md`, `batch-2.md`.

Likely shared with other batch-2 streams (additive hunks, rebase cleanly):

- `packages/theme/src/i18n/en.json`: P639 toasts and P637 mobile approval likely add keys. Mine sit
  under `studio.api.config.skipped.*` only.
- `docs/ARCHITECTURE.md`: every phase records facts. Mine: delete one Known open items bullet, edit
  bullets in "Api config files (P473)" only.
- `packages/shared/domain/*`: P637 may touch other domain files; mine is `apiConfig.ts` only.

## Implementer steps

Commit per group (Conventional Commits). Fast checks per commit: `go build ./...`, `go vet` on
touched packages, `bun run typecheck:web`, `bun run lint`.

1. configfs skip loop. Add `Skip`, `Skipped`, `YAMLSchemaSkip`; refactor `YAMLSchema` onto the
   shared core; `skip.go` with `valueAt`, `setAt`, pointer token split/unescape, `under(ptr, base)`.
   Unit test `skip_test.go` (justified: loop with stand-ins, nested pointers, guard and cap interact).
   Small inline schema: `name` required, `items` array of `{id (pattern), name, items?, kind?}`,
   recursive; one uniqueness rule reporting the later id. Table cases, each asserting skipped
   pointers, lines and the whole-file problem (or nil):
   - bad element mid-array: `/items/1` skipped, line of its `- id`, T holds stand-in at index 1;
   - nested: `/items/0/items/1` only;
   - parent and child both bad: both reported, discovery order;
   - duplicate id: later occurrence skipped;
   - `Max` 2 with 3 bad elements: whole-file problem = third, `skipped` has 2;
   - top-level `/name` missing after one skip: whole-file;
   - YAML syntax error: whole-file, no skipped;
   - zero `Skip` (`YAMLSchema`): first problem, identical to before.
   Run `go test ./internal/configfs/ -count=1`. Commit `feat: configfs skips bad array elements`.
2. apifiles decoders and format. `SkippedItem`, `CollectionFile.Skipped`, `collectionElement`,
   `collectionStandIn`, `maxSkippedItems`, `collectionLoad`, strict `collection`, prune, rule doc.
   Commit.
3. Projection. `flatten` file-index orders with `skippedAt`, `syncItems` keep set and `sort_order`
   update, `recordCollection` passes skipped. Commit.
4. Store. Folder decode, sigs, `noteProblems1(file, sig)`, Warn, `FileView.Skipped`/`SkippedView`,
   `editLocked` refusal, `form.go` caller. `go test ./apps/kira-space/internal/apifiles/... -count=1`
   (existing `edit_test.go`). Commit.
5. Flow half (`apiflow/configfiles_test.go`).
   - `TestApiConfigFiles` step 3: replace the duplicate-item insertion with a whole-file break that
     touches no item: insert top-level line `bogus: true` before `items:`. Keep every step-3 oracle
     (problem with line and message, last good row, `SaveRequest` `E_BAD_REQUEST`, fix clears it).
     Add: `view` file entry `Skipped` is empty. Regenerate `api-files.json`.
   - Rewrite `TestHandEditedCollectionWithBadEntry` (keep the name; fix its header comment: drop the
     F-A15 sentence). Fixture `hand.collection.yaml`, lines numbered:
     ```
     1 # hand written
     2 name: Hand
     3 items:
     4   - id: ok1
     5     name: Good one
     6     http:
     7       method: GET
     8       url: https://x.test/ok1
     9   - id: bad
     10    name: No protocol
     11  - id: ok2
     12    name: Good two
     13    http:
     14      method: GET
     15      url: https://x.test/ok2
     ```
     a. First write is the bad text, no good load before. Oracles: rows = `Good one,Good two`;
        `sortOrder` 0 and 2; `View` entry `Problem == nil`, `LastGood == false`, `Skipped ==
        [{Line: 9, Index: 1, ID: "bad", Name: "No protocol", Message: <itemKind text>}]` (message
        via `i18n.T("backend.studio.apiFiles.itemKind.body", nil)`). Contract scenario
        `api-collection-skipped`, keys `CollectionsService.List` and `ApiConfigService.View`, same
        `paths` replacement as `TestApiConfigFiles`.
     b. `SaveRequest` on `ok1` returns `E_BAD_REQUEST` naming line 9; file bytes unchanged.
     c. `app.Restart()`: bytes unchanged; same rows and same `Skipped`.
     d. Fix: give `bad` an `http` block (`method: GET`, `url: https://x.test/bad`). Eventually rows =
        `Good one,Good two,No protocol`, `Skipped` empty, orders 0,1,2.
     e. Item turns bad after a good load: send one request on `ok2` so it has a response history row
        (existing `send`/`get` helpers and a local `httptest` server, as `TestApiConfigFiles` does),
        then write text where `ok2` lost its `http` block and `ok1` is renamed `Good one renamed`.
        Oracles: `ok1` row renamed (valid items follow the file); `ok2` row id unchanged, name
        `Good two`, still listed; its history still 1 row; `Skipped` lists `ok2` at its line.
     f. Second file `nest.collection.yaml`, lines numbered:
        ```
        1 name: Nest
        2 items:
        3   - id: dir
        4     name: Folder
        5     items:
        6       - id: child-bad
        7         name: Bad child
        8         http:
        9           method: GET
        10          bogus: 1
        11      - id: child-ok
        12        name: Good child
        13        http:
        14          method: GET
        15          url: https://x.test/c
        16  - id: dup
        17    name: First
        18    http:
        19      method: GET
        20  - id: dup
        21    name: Second
        22    http:
        23      method: GET
        ```
        Oracles: rows `Folder`, `Good child` (parent `Folder`), one `dup` row named `First`;
        `Skipped` = `[{Line: 10, Index: 0, ID: "child-bad", Name: "Bad child", Message: <additional
        text for field items.0.items.0.http.bogus>}, {Line: 20, Index: 2, ID: "dup", Name: "Second",
        Message: <itemDuplicate with id dup>}]`. Build expected messages with `i18n.T` and the same
        vars `problemFor` uses, never hand-typed text.
   Run `KIRA_CONTRACT=write go test ./internal/flows/apiflow -run 'TestApiConfigFiles|TestHandEditedCollectionWithBadEntry' -count=1`
   from `apps/kira-space`, then rerun without `KIRA_CONTRACT`. Commit.
6. UI half. Domain type, `ApiConfigProblems.vue`, i18n keys. New spec
   `apps/kira-space/tests/ui/studio/api-collection-skipped.spec.ts`, modeled on
   `api-config-files.spec.ts` (contract reader, `CONTROL` snapshots, `openHttpMode`,
   `collectionRow`). Mocks: `collectionsList` -> contract `CollectionsService.List`,
   `variablesListEnvironments` -> `[]`, `apiConfigView` -> contract `ApiConfigService.View`,
   `configFilesOpen` -> `{}`. Same result as flow step a: rows `Good one`, `Good two` visible, `No
   protocol` absent; `api-config-skipped` visible with title naming `hand.collection.yaml`; exactly
   one `api-config-skipped-item` containing `Line 9`, `No protocol` and the itemKind message;
   `api-config-last-good` count 0; no `ConfigFileBar` error for the file; clicking
   `api-config-skipped-open` records `ConfigFilesService.Open` with `{folder: 'api', file:
   'hand.collection.yaml'}` (assert as `api-config-files.spec.ts` asserts Open). Run
   `api-config-files.spec.ts` too (its contract changed). Commit.
7. Docs (`docs/ARCHITECTURE.md`):
   - Delete the Known open items bullet "**Collection file with one bad item serves no rows on first
     load (v0.3.13).** ..." entirely.
   - "Api config files (P473)": the `apifiles` bullet gains: item-scoped problem (pointer under
     `/items/<n>`) skips that item only, valid items load in the same transaction, file order kept
     (`sort_order` = file index), first of duplicate ids wins, skipped item keeps its last good row
     (history survives), at most 100 skipped then whole-file, app edits refused while any item is
     skipped (`BrokenError`), file never rewritten. UI bullet: `ApiConfigProblems.vue` also shows a
     warn alert per partially loaded file listing each skipped item (line, name, reason, Open file).
     Tests bullet: contract `api-collection-skipped.json`, UI `api-collection-skipped.spec.ts`.
   - Under "DB sync, one mechanism" (configfs): one sentence on `YAMLSchemaSkip` (stand-ins keep
     pointers and lines; caller prunes).
   Commit `docs: ...`.

## Oracles (phase done when all hold)

- `go test ./internal/configfs/ ./apps/kira-space/internal/apifiles/...` green.
- `apiflow` `TestHandEditedCollectionWithBadEntry` and `TestApiConfigFiles` green without
  `KIRA_CONTRACT`; both contract files committed and consistent.
- UI specs `api-collection-skipped.spec.ts` and `api-config-files.spec.ts` green.
- `grep -n "serves no rows on first load" docs/ARCHITECTURE.md` returns nothing.
- `grep -rn "YAMLSchemaSkip" apps/kira-space/internal/apifiles` finds a real caller.
- `grep -n "dec.collectionLoad\|collectionLoad" apps/kira-space/internal/apifiles/store.go` finds
  the Folder wiring.
- No scoped `<style>` in `ApiConfigProblems.vue`; Alert/Button from `@theme/components/ui/*`.

## Verification commands

From repo root unless noted:

```sh
go test ./internal/configfs/ -count=1
(cd apps/kira-space && go test ./internal/apifiles/... -count=1)
(cd apps/kira-space && go test ./internal/flows/apiflow -run 'TestApiConfigFiles|TestHandEditedCollectionWithBadEntry|TestApiAgentsDocCoversSchemas' -count=1)
(cd apps/kira-space && go test ./internal/flows/apiflow -count=1)   # whole package once, near phase end
bun run typecheck:web && bun run typecheck:tests && bun run lint
bun run build:test && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui tests/ui/studio/api-collection-skipped.spec.ts tests/ui/studio/api-config-files.spec.ts
```

Check `docs/DEV_ENVIRONMENT.md` for the exact module path of `internal/configfs` (repo-root module)
and the UI spec invocation before running.

## Edge cases

- Bad item inside a skipped folder: both reported; only the folder pruned; child's `ParentKey` =
  folder's raw id.
- Bad item whose `id` is not a string or breaks the pattern: reported with `ID` empty (label falls
  back to name or `#n`); no row to keep.
- All items bad: collection row exists with no items (or kept last good rows); not whole-file.
- Stand-in id collision: `free` checks every id string in the document.
- File that also has a whole-file problem after some skips: whole-file wins (last good applies);
  `View` shows only the bar, no stale skipped list.
- Strict decode equals old behavior, so `Form`, import and edit validation messages are unchanged.
