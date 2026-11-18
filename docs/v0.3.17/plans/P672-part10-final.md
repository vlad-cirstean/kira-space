# P672 Part 10 final scan

Master: `plans/P672-part10.md` (final-scan section). Base: `chapter/v0.3.17` at `2f2d00668`, after
iter2 of Parts 2..9 (batches A-C, D-F). UI only, no Go.

## Method

- One scan of the whole tree across all subjects. Codegraph calls:
  - AdeRunSandboxBadge;
  - Button, InlineState, SectionHeading, statusTone;
  - ApiAgentAccess, AdeMainBranches;
  - PanelHeader, WorktreeList, ValidationSection;
  - AdeAllSessions, PlanScreen, DescribeView, EnvironmentsView;
  - TooltipIconButton, StatusBarItem;
  - AdeLinkCard, AgentUsageItem, AppMetricsItem;
  - QueryState, EmptyState, LoadFailure, ConfigFileProblem;
  - MemoryPanel, MemoryStart.
- Class-token sweeps, each tag matched quote-aware as the guard script does:
  - bound SectionHeading labels;
  - bound `:class` on toolbar buttons;
  - NativeSelect option sources and hosts;
  - outline buttons outside dialogs;
  - caption spans;
  - FieldDescription and FieldError outside a Field;
  - Badge `chip` without colour;
  - hand `*` markers;
  - NativeSelect and Input inside bars.
- A finding breaks a rule ARCHITECTURE already states, or a pattern one iteration adopted and a sibling area missed.
- Deliberate exceptions stay exempt:
  - git ViewToolbar detail head;
  - Studio tab view head;
  - AdeTaskCard and the plan board (D1);
  - bar toggles stay `segmented`;
  - engine cards without descriptions;
  - toasts.
- No a11y-only items.

## Carry-forward verdicts

### C1: decision-list NativeSelects (finding F1)

One rule, app-wide, which generalises the runs-params rule at ARCHITECTURE:773:

- Fixed option set:
  - Applies when there are at most 5 options (None or All counted) and the labels are short.
  - Renders SecondaryTabs.
  - In a form, it is `choice` with `size="kira-lg"`, beside the form controls.
  - In a row, table cell or labelled control strip, it is `choice` at default size.
  - In a bar (ViewToolbar, PanelHeader, search bar), it is `segmented` with no size.
- NativeSelect covers:
  - more than 5 options;
  - a data-driven list;
  - an action preset.
- NativeSelect size:
  - default size in a form;
  - `size="kira"` in a bar or row (see F7).

Counts were checked at source.

Convert to SecondaryTabs. Keep each existing testid on the SecondaryTabs root. Items are built from the existing constant and label function.

| Site | Options | Host | Becomes |
|---|---|---|---|
| `packages/workbench/src/settings/fields/LogLevelField.vue:47` | 5 | settings form | choice kira-lg |
| `apps/kira-space/frontend/src/studio/workbench/settings/ApiPane.vue:117` (HTTP_VERSIONS) | 2 | settings form | choice kira-lg |
| `apps/kira-space/frontend/src/studio/views/httprequest/RequestSettingsPane.vue:147` | 2 | settings form | choice kira-lg |
| `packages/docker-ui/src/components/EditInPlaceSection.vue:131` (RESTARTS) | 4 | edit form | choice kira-lg |
| `packages/git-ui/src/components/dialogs/RepoSettingsDialog.vue:254` graph scope | 2 | dialog form | choice kira-lg |
| `RepoSettingsDialog.vue:336` pull strategy | 4 | dialog form | choice kira-lg |
| `apps/kira-space/frontend/src/configsync/ConfigSyncUnitRow.vue:90` | 2 | row | choice default |
| `apps/kira-space/frontend/src/studio/views/httprequest/FormDataTable.vue:129` | 2 | table cell | choice default |
| `packages/docker-ui/src/components/EditRecreateSection.vue:277` (PROTOCOLS) | 3 | port row | choice default, drop `w-24` |
| `EditRecreateSection.vue:327` (MOUNT_TYPES) | 2 | mount row | choice default, drop `w-24` |
| `apps/kira-space/frontend/src/studio/views/console/kibana/SortControl.vue:77` (KIBANA_SAMPLE_SIZES) | 5 | labelled strip | choice default |
| `packages/git-ui/src/components/SearchBox.vue:258` scope | 3 | search bar | segmented |
| `apps/kira-space/frontend/src/studio/kubernetes/components/LogsView.vue:185` tail (TAILS) | 4 | ViewToolbar | segmented |
| `packages/docker-ui/src/components/LogsView.vue:218` tail (TAILS) | 4 | ViewToolbar | segmented |

Stay NativeSelect, because the list is long or data-driven:

- HitsHistogram interval: 7 options.
- MaskRuleRow and MaskRuleList kind (MASK_KIND_OPTIONS): 6 options.
- RepoSettingsDialog host provider: data-driven.
- Kube container: data-driven.
- RequestBodyPane code language (CODE_LANGUAGE_OPTIONS, 3) is a sub-language pick. It is the same
  control as the body-kind select beside it, which is data-driven. Rule: a fixed set sharing a bar
  slot with a dependent NativeSelect stays NativeSelect. Size per F7.
- The remaining data-driven lists in the Part 8 handoff stay as they are.

Number values (sample size, tail) go through the existing `formatCount` and `tailLabel` as item labels. `value` is the string form; parse it back in the update handler, as the select did.

A choice label follows ARCHITECTURE "Tabs by role":

- A `choice` with a FieldLabel takes the same text as its `aria-label`.
- In a row or cell, `aria-label` comes from the old select's label.
- A `segmented` in a bar takes `aria-label`.

Before you edit, read one existing host as the pattern: `DateFormatField.vue:53` (form) and `ReviewThread.vue:261` (row).

No guard: option counts come from constants, and a regex cannot count them. The rule lives in ARCHITECTURE.

### C2: AdeRunSandboxBadge popover (finding F2)

File: `apps/kira-space/frontend/src/ade/v2/panel/AdeRunSandboxBadge.vue`.

The trigger carries its tone as a class (`:class="tone"`: text-warn, muted, subtle). That evades U149.

The popover is hand-built:

- caption spans at :92 and :106;
- a hand network pair at :95-104;
- a hand "none" span at :109;
- hand denial rows at :112-122.

Every sibling popover uses the shared pieces instead (RunningOpsStatusItem: SectionHeading, PopoverList, rowVariants).

### C3: ApiAgentAccess `env.name`: no change

`data` means a font-data identifier. Its current users are all identifiers:

- AdeSettingsResult file name;
- SchemaBrowser service;
- StackList and DetailHeader branch.

An environment name is a user-chosen display name, and it renders in body font everywhere else (`EnvironmentsView.vue:210`). AdeAllSessions `g.title`, a task title, follows the same rule. Uppercase is the SectionHeading caption style for any label, which is correct.

The Edit button, `dialog` at `kira-lg`, matches its Part 4 list-action siblings (ApiConfigProblems, StudioMcpConnectionsPane, KubeMcpSection). Its FieldDescription none hint is covered by F4.

### C4: AdeMainBranches slot gap: no change

- The SectionHeading default slot is `gap-1`.
- PanelHeader `#actions` is `gap-1`.
- The ARCHITECTURE bar rule is `gap-1`.

ProjectPicker (`size="kira"`) and the toolbar Button sit in a bar slot, so this is correct.

## Guard and ARCHITECTURE verification

- `scripts/check-ui-primitives.sh` exits 0, so no exempt is stale. There are 48 PERMANENT lines, matching ARCHITECTURE:1439.
- The header comments (lines 13-31) are out of order and have stray blank comment lines. The Part 6 line sits before Part 5, and the Part 8 line before Part 7. Fix this in F9.
- ARCHITECTURE ranges end at U156 (lines 1437 and 1476). They move to the new maximum in the docs commit.

## Findings

### F1: fixed small option sets to SecondaryTabs

Per C1. Update each touched UI spec: a `selectOption` on the testid becomes a click on the item. Use the item testid pattern already used by SecondaryTabs hosts (read `SecondaryTabs.vue` for how items get testids; if items have none, click by role `tab`/`radio` and name).

### F2: AdeRunSandboxBadge on shared pieces

- Trigger:
  - Add `:data-tone="badge.denials > 0 || badge.access === 'off' ? 'warn' : undefined"`.
  - Delete the `tone` computed and the `:class`.
  - The not-enforced state shows in the label (`space.ade.sandbox.notEnforced.label`), so `text-subtle` goes.
- Popover:
  - Captions at :92 and :106 become `SectionHeading flush`.
  - The network pair at :95-104 becomes `KeyValueList density="compact"`.
  - The "none" span at :109 becomes `InlineState kind="empty"` with the same text.
  - The denial rows at :112-122 become `rowVariants({ layout: 'single' })` rows inside a PopoverList. Keep `font-data` on the path cell.
  - The `FieldDescription` status at :83 stays, because it is popover prose (TooltipDetails precedent).
- Keep every testid.

### F3: status tone through `data-tone` everywhere, including `info`

Toolbar buttons still tone through a class in shared code, which evades U149:

- `packages/workbench/src/components/StatusBarItem.vue`: `:class="toneClass"` from `TONE_TEXT`, in 3 Button branches.
- `packages/git-ui/src/components/CommitMeta.vue:326`: `TONE_TEXT[changeRequestTone(...)]`.
- AdeRunSandboxBadge (F2).

Changes:

- `packages/theme/src/components/ui/button/index.ts` toolbar variant: add `data-[tone=info]:text-info data-[tone=info]:hover:text-info`.
- `packages/theme/src/components/TooltipIconButton.vue`: add `'info'` to the `tone` type.
- `packages/theme/src/lib/statusTone.ts`: add `export type ToolbarTone = 'ok' | 'warn' | 'error' | 'info'` and `toolbarTone(t: StatusTone): ToolbarTone | undefined`, with neutral mapping to undefined.
- StatusBarItem: `:data-tone="tone"` on each Button branch. Drop `toneClass` and the `TONE_TEXT` import.
- CommitMeta: `:data-tone="toolbarTone(changeRequestTone(changeRequestIcon!.state))"`, with `class="ml-auto leading-none"`.

### F4: InlineState for list and section empty and failure states

#### FieldDescription used as an empty state

InlineState has 102 callers, but these sites hand-roll FieldDescription instead. Each becomes `<InlineState kind="empty">` with the same text and testid:

- HostViewSidebar reviewers :108, checks :131, labels :166, assignees :184;
- IssueSidebar labels :102, components :114;
- HostViewConversation :51; IssueComments :30; MarkdownBody :34; CommentComposer :106;
- StudioMcpConnectionsPane :124; KubeMcpSection :76; ScriptsMcpSection :62;
- ApiSecrets :92; ApiAgentAccess :31 (`api-agent-access-none`); ApiAgentAccessForm :117;
- AdeSettingsSecrets :86;
- MobileAccessPane :198, :211, :300;
- AdeReposPage :435;
- AdeLinkStrip (`ade-links-empty`);
- AdeLeasesPane (`space.ade.leases.empty.hint`);
- AdeSettingsResult empty hint;
- ApiAccessRulesSection (`apiRules.empty.body`).

Before converting, read each site. A FieldDescription that follows a control and explains it, rather than stating that a list is empty, stays as it is; list each kept site in the commit body.

#### Offline-plus-Retry rows

Both rows are `div flex items-center gap-2 text-kira-sm text-subtle` with a `dialog` Retry, inside a QueryState `#error` slot:

- `ConnectionMcpTables.vue:140-150`;
- `ConnectionMaskReview.vue:210-220`.

Each becomes `InlineState kind="error"` with the text, plus a toolbar `size="kira"` Retry button with `CodiconIcon name="refresh"`, the canonical Retry.

#### Section load errors as FieldError

These become `InlineState kind="error"` with the message:

- `AdeLinkStrip.vue:206` (`links.error`);
- `ContainerSizeSection.vue:42`.

### F5: FieldError used as pane or operation failure

The ARCHITECTURE state rule says FieldError means only an invalid field.

#### git-ui

Whole-pane replacement, with `v-else` replacing the content, becomes EmptyState `fill`:

- `icon="warning"`;
- `:title="t('common.state.loadFailed.title')"`;
- `:description` set to the existing error text;
- a toolbar Retry with `refresh` icon in the default slot where a retry exists.

Sites for EmptyState fill:

- `review/ReviewFilesPane.vue:182` loadError;
- `review/ReviewCommentsPane.vue:134`;
- `StashDetailPane.vue:69`;
- `WorkingDetailPane.vue:60`;
- `review/ReviewView.vue:513` compare error. Its `dialog` Retry becomes the toolbar Retry.
- `ReviewView.vue:460-470` bootError (hand `<p>` plus `dialog` Retry), same shape.

For these sites:

- The `p-3` class goes.
- Before editing, read each site's control flow to confirm the content is replaced. If the content is kept, apply the Alert rule below instead.
- `ReviewView.vue:509` resolving `<p class="p-3 text-muted-foreground">` becomes EmptyState `fill loading` with the same text as title.

Content kept becomes `Alert variant="error" strip` plus AlertDescription:

- `DetailPane.vue:72`;
- `ReviewFilesPane.vue:227` diffError;
- `ReviewFilesPane.vue:233` markError.

Row expansion becomes `InlineState kind="error"`:

- `review/ReviewCommitRow.vue:249` (drop `p-2`; InlineState owns its padding).

#### Settings and panels

A failed operation with the content kept becomes `Alert variant="error"`:

- `FileTypesPane.vue:106`, :109;
- `CliToolsPane.vue:67`;
- `ConfigSyncPane.vue:125`;
- `AdeWorkflowReviewPanel.vue:366`;
- `AdeLibraryStepEditor.vue:106`.

Not findings, because these are errors inside a Field or attached to the field's control:

- MobileAccessPane;
- AdeTaskStatuses;
- AdeStepInspector libError;
- form and picker validations.

### F6: Badge variants and the required marker

Required tag: one shape, the default Badge.

- `HostViewSidebar.vue:123`, :155 drop `variant="chip"`.
- `PromptDetail.vue:135` drops `variant="info"`.
- `ValidationSection.vue:48` is already default.

Untoned `chip` with no colour class or style, which renders as bare text, becomes default Badge:

- HostViewSidebar labels :172;
- IssueSidebar :105, :120;
- HostViewConversation :68 review state;
- AdeLinkTarget :76 fork;
- ValidationSection :30, :33. These keep `font-data`; the Badge has no `data` prop, so put `class="font-data"` on the Badge.

Lookalike: `KeyValueRowsTable.vue:239` `Badge variant="chip" class="bg-field text-subtle ml-1.5 shrink-0"` becomes `<Badge class="ml-1.5 shrink-0">`.

Required asterisk: add a `required?: boolean` prop to `packages/theme/src/components/ui/field/FieldLabel.vue`. When set, it renders after the slot:

```html
<span class="text-error">*<span class="sr-only"> {{ t('common.required.label') }}</span></span>
```

Hosts drop their hand span and pass `:required`:

- `ParamsForm.vue:59` (`p.required && !isSaved(p)`);
- `AdeContextFields.vue:53`, :94.

### F7: form controls in bars take `size="kira"`

The ARCHITECTURE form rule says `size="kira"` in bars and rows. These are default (`kira-lg`) beside `kira` siblings:

- `RequestBodyPane.vue:210` code language;
- `GrpcRequestView.vue:364` method select (beside AutocompleteField kira);
- `HitsHistogram.vue:171` interval (in its header strip);
- `kubernetes/components/LogsView.vue:173` container;
- `ResourceBrowser.vue:114` Input (`w-40`).

The kube and docker tail selects leave via F1.

### F8: small centralisation and spacing

- Memory origin tone:
  - The `origin === 'user' ? 'info' : origin === 'external' ? 'warn' : 'default'` ternary is duplicated at `packages/workbench/src/memory/MemoryPanel.vue:217` and `MemoryStart.vue:64`.
  - Add `ORIGIN_BADGE: Record<MemoryOrigin, 'info' | 'warn' | 'default'>` to `packages/workbench/src/memory/labels.ts`, beside `GRADE_TEXT`, and read it in both files.
- `apps/kira-space/frontend/src/ade/v2/needs/AdeAllSessions.vue`:
  - Outer `gap-4` becomes `gap-3` (pane block rule).
  - Drop the `px-1.5 pb-1` wrapper around SectionHeading, which pads twice.

### F9: guard script header

Reorder `scripts/check-ui-primitives.sh` header lines 13-31 by Part (4, 5, 6, 7, 8, 9 iter2). Remove the blank comment lines around the Part 9 line. Add:

`# Guards U157-U161, U149 extended: docs/v0.3.17/plans/P672-part10-final.md.`

## Guards

Write each after its fixes, with zero offenders. Before the fixes, confirm the regex hits the known sites: run it on the pre-fix tree via `git stash` or by reading the match list. Put every pattern in the same `perm` table form as U135-U156, using `(?&t)` and `${TOK}`.

- U149, extended in place: add a second alternative for a bound class holding a status tone. Literal form:
  `(?:<TooltipIconButton\b|<Button\b(?=(?&t)\svariant="toolbar"))(?&t)\s:class="[^"]*(?:TONE_TEXT\[|'text-(?:ok|warn|error|info)')`.
- U157, FieldError as a pane or section failure (use EmptyState, Alert or InlineState): `<FieldError\b(?&t)\s:?class="[^"]*${TOK}p[xy]?-`.
  - Also extend it to a FieldError directly inside a QueryState `#error` template if F5 sites show that shape. Otherwise padding only.
- U158, FieldDescription as an empty state (use InlineState kind empty):
  `<FieldDescription\b(?&t)>\s*\{\{\s*t\('[^']*\.(?:empty|none)\b` or a FieldDescription whose testid ends `-empty` or `-none`.
  - Check whether F4's kept sites trip it. A kept site with an `.empty` key either gets a reworded key or a PERMANENT line with its reason. Prefer the rename only if the text is not an empty state.
- U159, untoned chip Badge (use default Badge): `<Badge\b(?=(?&t)\svariant="chip")(?!(?&t)\s:?(?:class|style)=)(?&t)>`.
- U160, hand required asterisk (use FieldLabel `required`): `class="text-error"(?&t)?>\s*\*`. Exempt `FieldLabel.vue` in the built-in exempt column.
- U161, default-size form control in a bar (use `size="kira"`):
  `<(ViewToolbar|PanelHeader|PopoverBar)\b(?&t)>(?:(?!</\1\s*>)[\s\S])*?<(?:NativeSelect|Input)\b(?!(?&t)\ssize=)(?&t)`.
  - If a backreference conflicts with `(?&t)` group numbering, spell out three alternatives instead.

No guard for F1 (counts) or F8 (two sites).

## Execution

One sequential Sonnet implementer. The theme files, the guard script and the docs are shared by most findings. Streams would overlap semantically in `statusTone.ts`, `button/index.ts` and the guard script.

Commits, in order, Conventional Commits, each passing the pre-commit hook without `--no-verify`:

1. `feat(theme): toolbar info tone and toolbarTone map` (F3 theme part), then `refactor(ui): status bar, CommitMeta, sandbox badge tone via data-tone` (F3 hosts plus F2).
2. `refactor(ui): fixed small option sets render SecondaryTabs` (F1). Split per package if large: workbench and studio, docker-ui, git-ui.
3. `refactor(ui): list and section empty and failure states use InlineState` (F4).
4. `refactor(git-ui): pane failures use EmptyState, Alert and InlineState` (F5 git-ui), then `refactor(ui): settings and panel operation errors use Alert` (F5 rest).
5. `refactor(ui): one required Badge and FieldLabel required marker` (F6).
6. `refactor(ui): bar form controls take size kira` (F7) plus `refactor(ui): memory origin badge map, AdeAllSessions spacing` (F8).
7. `chore(lint): guards U157-U161, U149 extended` (guards plus F9 header).
8. `docs: P672 Part 10 final uniformity rules` (ARCHITECTURE).
9. `docs: P672 Part 10 final progress` (SPEC).

Fast checks per commit: typecheck, lint, `scripts/check-ui-primitives.sh`, `bun run lint:dead`.

## Targeted UI specs

Find specs by testid: for each touched file, grep its testids under `apps/kira-space/tests/ui`. Known hits:

- F1:
  - `studio/settings-apply-on-save.spec.ts` (log level);
  - `studio/docker-edit.spec.ts` (restart, port, mount);
  - `studio/http-request.spec.ts`, `studio/http-request-body.spec.ts`, `studio/collections.spec.ts` (form-data kind, HTTP version);
  - `git-host-provider.spec.ts`, `git-host-azure.spec.ts` (repo settings);
  - `studio/data-view.spec.ts`, `studio/tab-filter-state.spec.ts`, plus git search specs (`search-scope`);
  - the Kibana console and kube/docker logs specs;
  - config-sync specs.
- F2: ADE sandbox specs (grep `ade-run-sandbox`).
- F3: status bar and CommitMeta specs.
- F4 and F5: host-view, issue, review, stash, settings (file types, CLI tools, config sync), ADE links and leases specs, `ConnectionMcpTables` and mask-review specs.
- F8: `memory-module.spec.ts`, `memory-provenance.spec.ts`.

Spec rules:

- Run with `--workers=2`, once near the end.
- A timeout gets one lone rerun.
- Radio and tab keys use `tests/ui/support/radioKeys.ts` (`delay: 40`).
- Keep specs under the 600-line cap (N10) and use the N8 shared helpers.
- Run `bun run build:mobile` before the mobile spec (MobileAccessPane).

Assertions on `text-warn`, `text-subtle` or the `<select>` role update to `data-tone` or the tab item. Visual baselines refresh in PR CI.

## ARCHITECTURE edits

- Tabs by role (line 1471): add the C1 rule.
  - At most 5 short fixed options render SecondaryTabs: `choice` in a form (kira-lg) or a row, cell or labelled strip (default size); `segmented` in a bar.
  - More than 5, data-driven, action presets, or a set sharing a bar slot with a dependent select render NativeSelect.
  - Line 773 stays as the runs-params instance.
- Status tones (line 1465): toolbar tone becomes "ok, warn, error, info"; add "`toolbarTone` maps a StatusTone (neutral none); U149 also fails `TONE_TEXT[` or a quoted tone in a bound class".
- State primitive (line 1466): append these sentences:
  - "FieldError never carries padding or stands in for a pane (U157)."
  - "An empty list or section is InlineState, never FieldDescription (U158)."
  - "Retry is a toolbar Button with `refresh`."
- Field line (1459): add "a required field marks its FieldLabel `required` (U160); bar form controls `size="kira"` (U161)".
- Badge: add to the status-tones line "Badge `chip` only with its own colour class or style; untoned tags are default (U159); a required tag is default".
- Guard ranges: line 1437 `U83-U161`, line 1476 `U110-U161`. Update the PERMANENT count at line 1439 if any are added.

## SPEC progress line

Append under the Part 9 iter2 line in `## P672 Part 10`. Do not change the row status; the orchestrator closes the row.

`Part 10 final: fixed small option sets render SecondaryTabs (choice in forms and rows, segmented in bars), toolbar info tone and toolbarTone, AdeRunSandboxBadge on shared popover pieces, list empty states and section failures InlineState, git-ui pane failures EmptyState or Alert, required tag default Badge and FieldLabel required, bar controls size kira, guards U157-U161 plus U149 extended, plan plans/P672-part10-final.md.`

## No change

- C3 and C4: see the verdicts above.
- Title-bar meta spans: `host-view-ref` (font-data ref) and `proof-steps` are both `text-kira-sm text-muted-foreground`. The font differs because the content differs, which is correct.
- Config-file status notes (none, lastGood, readOnly) repeat across 8 hosts. The markup is uniform and the texts are host-specific.
- AppMetricsItem 4-column grid in TooltipDetails: tooltip density, a numeric table, not a KeyValueList.
- StreamFilterRow `text-state-on`, ReviewHunkMark `text-primary`: not status tones.
- Outline `dialog` buttons outside dialogs (about 150): consistent with Part 4's list-action classification.
- 67 caption spans: these are metadata text, not headings. Part 6 moved every heading-role caption.
- NativeSelect `variant="bordered"` written out explicitly is the default; cosmetic only.
