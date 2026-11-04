# P509 Part 1 plan: free-form link editing in task Details

Ask 1 of P509 only. Part 2 (Main tab row layout) is planned and implemented separately, concurrently,
in its own worktree. P508 (class-only restyle) also runs concurrently. One sequential Sonnet
implementer runs this plan.

## Goal

Details tab links behave like Notion: one always-present add line, type or paste, Enter files it under
the right category. Edit any user-owned row in place with the same free text. Esc cancels. Remove is
one click. No dialogs or multi-field forms for the common case.

## What restricts add/edit today (found via CodeGraph)

1. Classification is split and partial. `details/AdeLinkGroups.vue` `addValue` (l.98-138) decides on
   the frontend: Jira key/URL (via `jira.ts` `parseJira`, `isJiraIssueUrl`), then tries the task host
   slot by calling `UpdateTask` and catching `E_BAD_REQUEST`, else `AddArtifact` with kind `link` or
   `issue`. Backend `ade/artifacts.go` `normalizeArtifact` (l.383) only upgrades host-provider links to
   `changeRequest`/`issue`. Docs, branches and deployments are never recognised.
2. `branch` is not storable. `bridge/adetask.go` `AddArtifact` (l.976) and `agentmcp.CleanArtifact`
   refuse any kind outside `ReportableArtifactKind`. `details/linkGroups.ts` (l.82) hides every
   `branch` row from Related.
3. Edit is rename-only. `UpdateArtifactArgs` carries `title` only (`adewire/wire.go` l.1346,
   `wire.ts` l.501); `repos.UpdateArtifactTitle` writes only the title. A stored row's URL and kind
   can never change; pasting a new URL into an edit renames the row to the URL string.
   `linkGroups.ts` `fromArtifact` sets `editText` to the title only.
4. Title plus link is impossible. Text that is not a bare URL or key is refused
   (`space.ade.details.paste.notLink.error`). `"Design notes https://…"` and `[t](url)` both fail.
5. Paste discards typed text. `onPaste` (l.164) always `preventDefault`s and submits only the
   clipboard, so typing a title and then pasting the URL loses the title.
6. Slot edits are rigid. Jira slot edit only accepts a Jira key/URL (`useLinkFields.saveJira`); host
   slot edit only accepts a provider-recognised link (`AdeTaskService.validateHostLink`, l.394).
   Replacing the Jira link with, say, a PR link errors instead of refiling.
7. No way to fix a wrong category.

Detected rows (host change requests, tracker issue, git branches) stay read-only by design; they
mirror external state.

## Design

### Backend owns classification

Move all recognition to Go, one place, so the flow test proves filing end to end and the frontend
holds no rules. Two new bound methods replace `AddArtifact` and `UpdateArtifact`:

- `AdeTaskService.AddLink(AddLinkArgs{taskId, text}) -> AddLinkResult{filed, kind, artifact}`.
  `filed` is `jira` | `host` | `stored` | `known`. `artifact` is the stored row when `filed=stored`,
  else null. `known` means the task already shows that exact item; nothing is written.
- `AdeTaskService.EditLink(EditLinkArgs{taskId, slot, id, text, kind}) -> EditLinkResult{filed, kind, artifact}`.
  `slot` is `jira` | `host` | `stored`; `id` is required only for `stored`. Exactly one of `text`
  and `kind` is non-empty. `kind` (recategorise) is valid only for `stored`.

`RemoveArtifact` stays as is. Slot clearing stays `UpdateTask` (`clearJira`, `hostUrl: ''`), as
today. An empty edit still means remove; the frontend routes it there, never to `EditLink`.

Delete `AddArtifact`, `UpdateArtifact` (bridge, engine, wire types both sides, frontend bridge entry,
queries, mock maps) and `repos.UpdateArtifactTitle`. Nothing else calls them; keeping them would be
dead surface (knip, `lint:dead`). Agent MCP reporting (`ReportArtifacts`, `storeArtifacts`) is
unchanged.

### Text parsing (`parseLinkText`, pure)

Input is one trimmed line, already free of control characters (bridge check).

1. Markdown `[title](url)` whole line: title and URL.
2. Else split on whitespace. Tokens that parse as absolute `http`/`https` URLs with a host are links.
   More than one: refuse `oneLink`. Exactly one: that is the URL; the other tokens, joined by one
   space, are the title, with leading/trailing separators `- – — : |` and surrounding `<>` trimmed.
3. No URL: the whole text is `words`.

Result: `{title, url, words}`. Length caps reuse `agentmcp.MaxArtifactTitle` and `MaxArtifactURL`;
over-cap refuses with the existing caps' wording.

### Recognition (`classifyLink`, first match wins)

For a URL:

1. Issue tracker: `deps.Trackers.ParseRef(url)` matches, or the URL host equals the host of the task's
   stored `JiraURL` and its path holds a key (ports `isJiraIssueUrl`'s known-host rule). Kind `issue`,
   origin `tracker`, ref = key.
2. Code host link: `b.hostLink(url)` (`Registry.ParseLink`). Kind `changeRequest` or `issue` per the
   provider, origin `host`, ref = provider ref.
3. Code host branch: `Registry.ParseBranchLink(ctx, url)` with `Number == 0`, `!Issue`, `TreePath != ""`.
   Kind `branch`, ref = `TreePath`. GitHub `/tree/…`, Azure `?version=GB…`. `Claims` may consult `gh`
   hosts; `AddHostLink` already accepts that cost.
4. Doc: a table in `linkclassify.go`, nothing else. Host `docs.google.com`; host suffix `.notion.so`,
   `.notion.site`, `.sharepoint.com`, `.readthedocs.io`, `.gitbook.io`; host `paper.dropbox.com`;
   `*.atlassian.net` with path prefix `/wiki/`; host starting `confluence.`; a path segment `wiki`
   on a host a code-host provider claims; path ending `.md .mdx .pdf .doc .docx .rst .adoc .txt`
   (case-insensitive). Kind `doc`.
5. Else kind `link`.

For `words` (no URL):

- `deps.Trackers.ParseRef(words)` matches a bare key: kind `issue`, origin `tracker`.
- Else refuse `notLink` (wording points to Notes for free text). Plain prose is notes, not links;
  Details already has the Notes editor above.

Deployment is never auto-detected (no reliable signal); the kind menu can set it.

### Filing (`TaskBoard.AddLink`)

- Tracker issue: task has no Jira → set Jira slot (`UpdateTask` patch `jira{key,url}`), `filed=jira`.
  Same key as the slot → `known`. Else store kind `issue`, `url` (may be ''), `ref=key`,
  title = given title or key.
- Host `changeRequest`/`issue`: task has no `HostURL` → set host slot, `filed=host`. Same URL
  (trailing slash ignored) → `known`. Else store.
- Branch, doc, link: store.
- Store goes through `storeArtifacts` (source `user`). An existing stored row with that URL is upserted
  (today's behaviour); report `filed=stored`. Empty title → `pastedArtifactTitle(url)` (today's rule).
- `storeArtifacts` gains a `kindOK func(string) bool` parameter. Agent path passes
  `agentmcp.ReportableArtifactKind`; user path passes `userArtifactKind` (reportable kinds plus
  `branch`). Add `agentmcp.CleanArtifactAs(in, kindOK)`; `CleanArtifact` becomes
  `CleanArtifactAs(in, ReportableArtifactKind)`.
- Archived or missing task refuses, as `AddArtifact` does now (`loadTaskCtx`).

### Editing (`TaskBoard.EditLink`)

- `stored` + `kind`: kind must be a `userArtifactKind`; set kind, keep the rest.
- `stored` + `text`: parse.
  - `words` only (no URL, not a key): rename. Title = words.
  - URL: new URL, `kind`/`ref` from `classifyLink` (no slot moves for stored rows; a tracker or host
    match just sets that kind and ref on the row). Title: the given title; else the old title, unless
    the old title equals `pastedArtifactTitle(oldURL)` or `oldRef` (an auto title), then
    `pastedArtifactTitle(newURL)`. A URL already stored on another row of the task refuses
    `alreadyLinked` (pre-check under the task lock; the partial unique index
    `ade_task_artifacts_url` is the backstop).
  - Bare key: kind `issue`, ref = key, URL cleared only when the text has no URL.
- `jira` slot: text classifies as a tracker issue → update the slot. Anything else → `AddLink(text)`
  first, then clear the slot. Add-then-clear means a failed add leaves the slot intact.
- `host` slot: text classifies as a host `changeRequest`/`issue` link → update the slot. Anything
  else → add first, then clear the slot.
- Unknown id, a detected row's id, or a row of another task → refuse (today's
  `artifact %s is not stored for this task`).
- Writes: replace `repos.UpdateArtifactTitle` with `repos.UpdateArtifact(taskID, id, kind, title,
  url, ref, now)`; same `RequireOneRow` guard. Engine calls `notifyBoard()` after every write.

### Bridge (`internal/bridge/adetask.go`)

`AddLink`, `EditLink` follow the existing pattern: `adeUserCtx`, `validateAdeTaskID`, then
`validateLinkText`: trimmed non-empty, at most `MaxArtifactURL + MaxArtifactTitle + 16` bytes, no
control characters (newlines included: one line per call). `EditLink` also checks `slot` enum,
`validateAdeItemID(id)` when `stored`, `kind` in `userArtifactKind`, and exactly one of `text`/`kind`.
Contract failures use `ipcerr.Contract`; classification refusals are `ade.invalid(...)` with catalog
text, so they map to `E_BAD_REQUEST` through `adeTaskError` and show as plain text.

Regenerate bindings: `wails3 task common:generate:bindings` (or `bash scripts/setup.sh`).

### Wire (`adewire/wire.go`, `frontend/src/ade/v2/wire.ts`)

```ts
export interface AddLinkArgs { taskId: string; text: string }
export interface EditLinkArgs {
  taskId: string;
  slot: 'jira' | 'host' | 'stored';
  id: string /* '' unless slot is 'stored' */;
  text: string /* '' when kind is set */;
  kind: '' | 'changeRequest' | 'issue' | 'branch' | 'deployment' | 'doc' | 'link';
}
export interface LinkResult {
  filed: 'jira' | 'host' | 'stored' | 'known';
  kind: Artifact['kind'];
  artifact: Artifact | null;
}
```

Go mirrors with the same JSON keys (`LinkResult.Artifact *Artifact`). Remove `AddArtifactArgs`,
`UpdateArtifactArgs` from both. `Artifact.kind` already includes `branch`.

### Frontend

- `queries.ts`: replace `useAddArtifact`/`useUpdateArtifact` with `useAddLink`/`useEditLink`
  (TanStack `useMutation`, `onSuccess: refetchArtifacts`; the board push refreshes the task's slots).
  Keep `useRemoveArtifact`.
- `bridge/index.ts`: `adeTaskAddLink`, `adeTaskEditLink` entries; drop the two old ones.
- `details/linkGroups.ts`:
  - Related skips only detected branches (`a.kind === 'branch' && a.id === ''`); stored branch links
    show.
  - `editText`: stored row with URL → `title url` when title is set and differs from the URL, else the
    URL; stored row without URL → title or ref. Jira slot → `jira.url || jira.key`. Host slot →
    `hostUrl`.
- New `details/AdeLinkAddLine.vue` (`<script setup lang="ts">`, Tailwind, shadcn `Input` with
  `appearance="inline"`, `CodiconIcon` `add`): the always-present last row of the list, aligned with
  link rows. `v-model` text. Enter submits (ignores IME composition, like `useInlineEdit`). Esc
  clears and blurs. Paste: input empty → `preventDefault`, submit the clipboard text at once; input
  non-empty → default paste (title typed first, then pasted URL), Enter submits. Multi-line text is
  split per line and submitted in order; on the first failure the unsent lines go back into the input
  and the error shows (today's resume behaviour). `filed=known` shows a quiet one-line note
  (`alreadyLinked` copy), cleared on the next input. Emits `submit(text)` to the parent; the parent
  owns the mutation and returns the error text.
- New `details/AdeLinkKindMenu.vue`: shadcn `DropdownMenu` around the kind chip of a stored row. Items:
  the six kinds, current one checked. Select → `EditLink{slot:'stored', id, kind}`. Non-stored rows
  keep the plain chip.
- `details/AdeDetailsLink.vue`: kind chip slot uses `AdeLinkKindMenu` when `origin === 'stored'` and not
  readonly. Edit keeps pencil and double-click; add F2 on the focused row (`tabindex="0"` on the `li`,
  `@keydown.f2`). Remove stays the X button.
- `details/AdeDetailsLinkEdit.vue`: unchanged behaviour (Enter/blur commit, Esc cancel, empty removes).
  Placeholder copy hints "title and link".
- `details/AdeLinkGroups.vue`: drop `useLinkFields`, `isJiraIssueUrl`, `parseJira`, `isUrl`, `addValue`,
  the host-slot try/catch. `submit(line)` → `addLink.mutateAsync({taskId, text: line})`. `commit(item,
  text)`: empty → remove path (stored: `RemoveArtifact`; jira: `UpdateTask clearJira`; host:
  `UpdateTask hostUrl ''`); else `editLink.mutateAsync({taskId, slot: item.origin, id:
  item.artifactId ?? '', text, kind: ''})`. Renders `AdeLinkAddLine` instead of the bare `Input`.
  Keep the existing test ids `ade-details-paste`, `ade-details-paste-error`, `ade-details-link-*` on
  their new elements; add `ade-details-paste-known`, `ade-details-link-kind`,
  `ade-details-link-kind-item` (with `data-kind`).
- Delete `panel/useLinkFields.ts` (sole caller is `AdeLinkGroups`). Delete `isJiraIssueUrl` from
  `jira.ts`; `parseJira` stays (`AdeAddPopover.vue` uses it).
- i18n (`packages/theme/src/i18n/en.json`): new keys under `space.ade.details.*` (add-line placeholder
  and aria, known note, kind menu label) and `backend.space.ade.link.*` (`notLink`, `oneLink`,
  `alreadyLinked`). Remove keys left unused (`space.ade.details.paste.notLink.error`,
  `space.ade.link.notJira.error` if no other user, `backend.space.ade.artifactTitle.body` if no other
  user); `check-catalog --unused` enforces it.

Out of scope: the new-task popover (`AdeAddPopover.vue`), the stage dialog's read-only
`AdeArtifactList.vue`, agent MCP reporting, Main tab layout (Part 2).

## Ownership (zero overlap with Part 2)

| Area | Part 1 (this plan) | Part 2 |
|---|---|---|
| `frontend/src/ade/v2/details/AdeLinkGroups.vue`, `AdeDetailsLink.vue`, `AdeDetailsLinkEdit.vue`, `linkGroups.ts` | owns | none |
| new `details/AdeLinkAddLine.vue`, `details/AdeLinkKindMenu.vue` | owns | none |
| `panel/useLinkFields.ts` (delete), `jira.ts`, `queries.ts`, `wire.ts`, `frontend/src/bridge/index.ts` | owns | none |
| `panel/AdeMainTab.vue`, `AdeMainBranches.vue`, `AdeStepBlock.vue`, branch/step row components, `AdeTaskIssueLine.vue`, `AdeTaskPanel.vue`, `details/AdeDetailsTab.vue` | none | owns |
| Go: `internal/ade/artifacts.go`, new `internal/ade/links.go`, new `internal/ade/linkclassify.go` (+ `_test.go`), `internal/bridge/adetask.go`, `internal/bridge/adewire/wire.go`, `internal/storage/repos/adeartifact.go`, `internal/agentmcp/artifacts.go` | owns | none |
| `internal/flows/adeflow/detailslinks_test.go` (new), `taskpanel_test.go`, `artifacts_test.go` | owns | none |
| `tests/contract/ade-details-links.json` (new), `ade-task-panel.json`, `ade-artifacts.json` | owns | none |
| `tests/ui/ade-v2-details-links.spec.ts` (new), `ade-v2-artifacts.spec.ts`, `tests/ui/support/{ipcChannels,mockRuntime,adeV2}.ts` | owns | none |
| `tests/ui/ade-v2-task-panel.spec.ts` | shared: Part 1 edits only the paste and inline-edit tests (move them to the new spec) and the header comment | Main-tab tests only |
| `packages/theme/src/i18n/en.json` | shared: additive keys under `space.ade.details.*`, `backend.space.ade.link.*`; removals named above | additive keys under its own prefix |
| `docs/ARCHITECTURE.md` | owns one short ADE fact: "Details links: backend classifies; rules in `ade/linkclassify.go`" | none |

Shared files merge by hunk; no shared hunk exists if each side stays in its prefix or tests. P508 may
change classes in the details components. On rebase keep P508's classes and this plan's structure;
new components take the classes P508 uses on sibling rows.

Landing: Part 1 and Part 2 run in separate worktrees off the same chapter commit. Each rebases onto
`chapter/v0.2.34` after P508 lands, in either order; a conflict outside the shared rows above means
an ownership miss, so stop and report it.

## IPC-split tests (one scenario, both halves)

Scenario `ade-details-links`.

Backend: `internal/flows/adeflow/detailslinks_test.go` `TestDetailsLinks`. Harness
`flowharness.New(t, WithGitHost(fakehost.New()), WithoutGh())`; GitHub stays registered, and
`Claims("github.com")` needs no `gh`. Create a task with no Jira and no host link. Then, in order:

1. `AddLink "PAY-9"` → `filed=jira`; task Jira key `PAY-9`.
2. `AddLink "https://forge.test/acme/api/-/merge_requests/8"` → `filed=host`, kind `changeRequest`.
3. `AddLink "https://forge.test/acme/api/-/merge_requests/9"` → `filed=stored`, kind `changeRequest`,
   ref `api!9`.
4. `AddLink "https://github.com/acme/api/tree/feat/x"` → stored `branch`, ref `feat/x`.
5. `AddLink "Design notes https://docs.google.com/document/d/abc/edit"` → stored `doc`, title
   `Design notes`.
6. `AddLink "[Runbook](https://wiki.example/runbook)"` → stored `link`, title `Runbook`.
7. `AddLink "https://jira.example.com/browse/PAY-12"` → stored `issue`, ref `PAY-12`.
8. `AddLink "PAY-9"` → `filed=known`; stored count unchanged.
9. Refusals, each `E_BAD_REQUEST`, nothing written: `"just some words"`,
   `"https://a.example https://b.example"`.
10. `EditLink stored(doc) "Design spec https://docs.google.com/document/d/abc/edit"` → title
    `Design spec`, URL unchanged.
11. `EditLink stored(Runbook) "https://wiki.example/runbook-v2"` → URL changed, title stays `Runbook`.
12. `EditLink stored(Runbook) kind "doc"` → kind `doc`.
13. `EditLink stored(Runbook) "https://docs.google.com/document/d/abc/edit"` → refuses
    `alreadyLinked`.
14. `EditLink jira "PAY-10"` → slot `PAY-10`.
15. `EditLink host "https://wiki.example/x"` → host slot cleared; stored `link` added.
16. `EditLink stored` with a detected id → refuses.

Record contract keys: `args:AdeTaskService.AddLink` (step 5's args), `AdeTaskService.AddLink`
(step 5's result), `args:AdeTaskService.EditLink` (step 11's args), `AdeTaskService.EditLink`
(step 11's result), `AdeTaskService.Artifacts` (final list, mask `id createdAt updatedAt taskId`).
Write with `KIRA_CONTRACT=write`, then rerun without it.

Frontend: `tests/ui/ade-v2-details-links.spec.ts`. Board fixture task with Jira and host link set;
`adeTaskArtifacts` answers from `ade-details-links.json` `AdeTaskService.Artifacts`; `adeTaskAddLink`
and `adeTaskEditLink` answer from the contract results.

- Rows show under their categories: chips read Pull request, Branch, Doc, Link, Issue for the
  contract rows (assert `data-kind` on `ade-details-link-kind`).
- Paste into the empty add line submits at once: one `adeTaskAddLink` call with
  `{taskId, text: <url>}`; input empties.
- Type `Design notes `, then paste the URL: no call yet; the input holds both. Enter sends
  `args:AdeTaskService.AddLink` exactly (`toEqual` on the contract args).
- Multi-line paste: one call per line, in order.
- Esc in the add line clears it, no call.
- Error: mock `adeTaskAddLink` to reject with a coded bad-request; the error shows and the text stays.
- `filed=known` response shows `ade-details-paste-known`.
- Edit in place: double-click the Runbook row → input holds `Runbook https://wiki.example/runbook`;
  replace with the step 11 text, Enter sends `args:AdeTaskService.EditLink` exactly. F2 also opens it.
  Esc cancels with no call. Empty + Enter sends `RemoveArtifact`.
- Kind menu: open on a stored row, pick Doc → `adeTaskEditLink` with `{slot:'stored', id, text:'',
  kind:'doc'}`. Detected rows have no menu and no edit.
- Jira slot X sends `adeTaskUpdateTask` with `clearJira: true`.

Migrate existing tests:

- `taskpanel_test.go`: the two `AddArtifact` calls become `AddLink` (`"Runbook https://wiki.example/runbook"`
  keeps the agent session; `"Other https://wiki.example/other"` has none). The MR and empty-title
  checks move to `TestDetailsLinks` (step 3 and `pastedArtifactTitle` in step 15's stored link). The
  rename becomes `EditLink stored "Other docs"`; the detected-id and empty checks become `EditLink`
  refusals. Re-record `ade-task-panel.json`; diff it and confirm the UI spec's names (`Runbook`,
  `Other docs`, `Fix login`) remain.
- `ade-v2-task-panel.spec.ts`: delete the paste and inline-edit tests (now in the new spec); fix the
  header comment.
- `artifacts_test.go`: the add and bad-add cases call `AddLink`; contract key
  `args:AdeTaskService.AddArtifact`/`AdeTaskService.AddArtifact` become `…AddLink`. Delete the stale
  keys from `ade-artifacts.json` by hand (write mode adds, never prunes). The archived-task refusal
  calls `AddLink`.
- `ade-v2-artifacts.spec.ts`: "pasting a link sends AddArtifact" becomes "sends AddLink" against the
  renamed key.
- `tests/ui/support`: `ipcChannels.ts` (`adeTaskAddLink: 'kira:adetask:addLink'`,
  `adeTaskEditLink: 'kira:adetask:editLink'`, drop the old two), `mockRuntime.ts` FQN map and the
  board-changed follow-up map, `adeV2.ts` default stubs.

Unit test: one table test `internal/ade/linkclassify_test.go` over `parseLinkText` and the doc table
only. It earns its place: markdown, separators, multi-URL refusal and ~10 doc rules interact. No
frontend unit tests.

## Work items (one sequential implementer, commit per group)

1. `agentmcp.CleanArtifactAs`; `storeArtifacts` `kindOK` param; `userArtifactKind`. Commit
   `refactor:`.
2. `linkclassify.go` (`parseLinkText`, `classifyLink`, doc table) + table test. Commit `feat:`.
3. `repos.UpdateArtifact` replaces `UpdateArtifactTitle`; engine `AddLink`, `EditLink` in `links.go`;
   delete engine `AddArtifact`, `UpdateArtifact`. Commit `feat:`.
4. Wire types, bridge methods, `validateLinkText`; delete old bridge methods; regenerate bindings.
   Commit `feat!:` with `BREAKING CHANGE:` footer naming the removed bound methods.
5. `TestDetailsLinks` + contract; migrate `taskpanel_test.go`, `artifacts_test.go`; re-record both
   contracts. Commit `test:`.
6. Frontend: wire.ts, bridge index, queries, `linkGroups.ts`, `AdeLinkAddLine.vue`,
   `AdeLinkKindMenu.vue`, `AdeDetailsLink.vue`, `AdeDetailsLinkEdit.vue`, `AdeLinkGroups.vue`; delete
   `useLinkFields.ts`, `isJiraIssueUrl`; i18n keys. Commit `feat:`.
7. UI: new spec; trim `ade-v2-task-panel.spec.ts`; migrate `ade-v2-artifacts.spec.ts`; support maps.
   Commit `test:`.
8. `docs/ARCHITECTURE.md` fact. Commit `docs:`.

## Verification (fast checks only)

- `bash scripts/setup.sh` first in the worktree (bindings are gitignored).
- `go build ./...`, `go vet ./apps/kira-space/...`, `bun run lint:go`.
- `go test ./internal/agentmcp/... ./apps/kira-space/internal/ade/... ./apps/kira-space/internal/bridge/...`.
- Own flow tests: `go test ./apps/kira-space/internal/flows/adeflow -run 'TestDetailsLinks|TestTaskPanelOrigins|TestTaskArtifacts'`.
- `bun run typecheck`, `bun run lint`, `bun run lint:dead`.
- Own UI specs: `bun run build:test:space`, then `playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-details-links ade-v2-task-panel ade-v2-artifacts`.
- Grep: no `AddArtifact`/`UpdateArtifact`/`useLinkFields`/`isJiraIssueUrl` left
  (`rg 'AddArtifact|UpdateArtifact\b|UpdateArtifactTitle|useLinkFields|isJiraIssueUrl' apps internal`).
  `RemoveArtifact` and `ReportArtifacts` stay.
- Full e2e/UI/visual suites are not run here: the chapter's single full run after P509 covers them,
  and visual baselines for the Details tab re-record there.

## Risks

- Removing `AddArtifact`/`UpdateArtifact` is a bound-surface break. Only this app's frontend and
  tests call them; the grep above proves it.
- `ParseBranchLink` on GitHub may run `gh` host discovery for a non-github.com host. Same cost
  `AddHostLink` already pays; acceptable on an explicit user add.
- A pasted branch link equal to a detected branch's URL merges into the detected row
  (`mergeArtifacts`: detected wins) and Related hides detected branches, so it looks like nothing was
  added. The Main tab already shows that branch. Acceptable; noted rather than special-cased.
- Doc rules are heuristic. The kind menu fixes a miss in one click; the table is one place to extend.
- Plain prose is refused, not stored. Free text belongs in Notes; the refusal says so. If the user
  wants text items later, that is a new kind and a new row in `SPEC.md`.
- Concurrency: P508 and Part 2 touch nearby files. Ownership table above; rebase rules stated.
