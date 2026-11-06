#!/bin/sh
# P262/P263/P264: one UI language. Guards the element rules of git show bd0c224688d9:docs/v2.2/plans/P262-ui-unification.md §2
# (U1-U21: dialog structure, secondary tabs, buttons, form controls, weights, icon sizes, bars,
# popovers) and git show bd0c224688d9:docs/v2.2/plans/P263-ui-unification-2.md §2 (U22-U38: titles, spinners, alerts, tones,
# dots, toggles, tooltips, menus, resize handles, scrollbars, kbd, colours, focus, cards, tables, scale).
# Scans app/package sources and packages/theme/src, never packages/theme/src/components/ui (the primitives themselves).
#
# Named exemptions (PERMANENT below) cover a real requirement a primitive cannot meet. One that no
# longer exempts anything fails the run, so the list only shrinks.
#
# U39-U63 never scan `/behaviour/` (the owning modules) or `/testing/`.
#
# Guards U72-U82: git show bd0c224688d9:docs/v2.2/plans/P266-consistency-audit.md section 4.4.
#
# Needs GNU grep -P like check-theme-classes.sh.
set -e

cd "$(dirname "$0")/.."

# Byte-wise matching: same result under any locale (the U59 pattern holds a multibyte dash).
export LC_ALL=C

GNU_GREP=""
for candidate in ggrep grep; do
  if command -v "$candidate" >/dev/null 2>&1 && echo x | "$candidate" -qP 'x' 2>/dev/null; then
    GNU_GREP=$candidate
    break
  fi
done
if [ -z "$GNU_GREP" ]; then
  echo "check-ui-primitives: GNU grep with -P is required (macOS: brew install grep, provides ggrep)." >&2
  exit 1
fi

DIRS="apps/kira-space/frontend/src apps/kira-space/frontend/mobile packages/workbench/src packages/git-ui/src packages/docker-ui/src packages/theme/src"
CORE_DIRS="packages/git-core/src packages/git-ipc/src packages/api-core/src"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# Guard kind `s` scans only these side-panel files (the P262 section 1.6 inventory).
cat >"$TMP/side_panels" <<'PANELS'
apps/kira-space/frontend/src/studio/workbench/panels/ProjectPanel.vue
apps/kira-space/frontend/src/studio/api/CollectionsPanel.vue
packages/docker-ui/src/components/DockerPanel.vue
packages/workbench/src/automations/AutomationsPanel.vue
packages/workbench/src/memory/MemoryPanel.vue
apps/kira-space/frontend/src/repo/GitPanel.vue
apps/kira-space/frontend/src/studio/project/ProjectTree.vue
apps/kira-space/frontend/src/studio/project/TreeRow.vue
apps/kira-space/frontend/src/studio/api/CollectionsTree.vue
apps/kira-space/frontend/src/studio/api/CollectionRow.vue
packages/docker-ui/src/components/ContainerList.vue
packages/docker-ui/src/components/ImageList.vue
packages/docker-ui/src/components/VolumeList.vue
packages/docker-ui/src/components/NetworkList.vue
packages/docker-ui/src/components/VirtualList.vue
apps/kira-space/frontend/src/repo/RepoFileTree.vue
apps/kira-space/frontend/src/repo/RepoTreeRow.vue
apps/kira-space/frontend/src/repo/RepoSearchView.vue
apps/kira-space/frontend/src/repo/RepoSearchRow.vue
apps/kira-space/frontend/src/repo/RepoReviewView.vue
packages/git-ui/src/components/review/ReviewView.vue
packages/git-ui/src/components/review/ReviewCommitRow.vue
packages/git-ui/src/components/review/ReviewFilesPane.vue
packages/git-ui/src/components/review/ReviewCommentsPane.vue
packages/git-ui/src/components/FileTree.vue
apps/kira-space/frontend/src/ade/v2/review/AdeReviewWindow.vue
apps/kira-space/frontend/src/ade/v2/review/AdeReviewFiles.vue
apps/kira-space/frontend/src/ade/v2/review/AdeReviewAgentPanel.vue
apps/kira-space/frontend/src/ade/v2/panel/AdePanel.vue
apps/kira-space/frontend/src/ade/v2/panel/AdePanelFrame.vue
apps/kira-space/frontend/src/ade/v2/panel/AdeTaskPanel.vue
apps/kira-space/frontend/src/ade/v2/panel/AdeBranchPanel.vue
apps/kira-space/frontend/src/ade/v2/panel/AdeArchivedPanel.vue
apps/kira-space/frontend/src/ade/v2/later/AdeLaterPage.vue
apps/kira-space/frontend/src/ade/v2/later/AdeShelfSection.vue
apps/kira-space/frontend/src/ade/v2/later/AdeShelfRow.vue
apps/kira-space/frontend/src/repo/ReposDialog.vue
packages/workbench/src/components/WorkbenchShell.vue
packages/workbench/src/components/PanelHeader.vue
packages/workbench/src/components/TreeTwisty.vue
packages/workbench/src/components/SettingsShell.vue
PANELS

# "U<n> path # reason": one guard exempt for one file.
cat >"$TMP/perm" <<'PERMANENT'
# foundation
U41 packages/theme/src/wrapSelection.ts # modifier-free typing guard on beforeinput, not a shortcut chord
U41 packages/workbench/src/terminal/terminalRenderer.ts # xterm engine host maps Shift+Enter to meta-Enter bytes, not an app shortcut
U11 packages/workbench/src/components/LogLines.vue # ANSI bold is log data
U62 packages/workbench/src/components/AutocompleteField.vue # accepting a suggestion restores focus and the caret on the real input
U26 packages/workbench/src/components/OpLogPanel.vue # error log row wash at the shared /15 tint, with the inset rail
U36 packages/theme/src/components/VarText.vue # inline variable mark, not a content box
# A (apps/kira-space)
U100 apps/kira-space/frontend/src/studio/api/CopyAsCurlDialog.vue # read-only output fields, nothing to discard
U34 apps/kira-space/frontend/src/studio/theme/EngineIcon.vue # engine brand marks are third-party logo colours
U36 apps/kira-space/frontend/src/studio/project/connection/ConnectionEnginePicker.vue # engine tile is a selectable radio card, not a content box
U46 apps/kira-space/frontend/src/studio/bridge/port.ts # bridge transport timeout
U41 apps/kira-space/frontend/src/studio/editor/MonacoHost.vue # Monaco engine host reads modifier keys
U62 apps/kira-space/frontend/src/studio/editor/MonacoHost.vue # Monaco engine host owns editor focus
U41 apps/kira-space/frontend/src/studio/views/grid/SlickGridHost.vue # column-header click reads the Ctrl/Cmd mouse modifier, not a key chord
U62 apps/kira-space/frontend/src/studio/views/grid/slick/editor.ts # SlickGrid cell editor owns its input focus
U52 apps/kira-space/frontend/src/studio/views/shared/celleditor/timestamp.ts # spelling-preserving timestamp re-encoding
U62 apps/kira-space/frontend/src/studio/views/console/ConsoleView.vue # Monaco host exposes focus(); not a DOM element focus
U47 apps/kira-space/frontend/src/studio/views/shared/fields/FieldRowsTable.vue # 2D arrow navigation between text inputs; useListNavigation ignores text fields
U62 apps/kira-space/frontend/src/studio/views/shared/fields/FieldRowsTable.vue # moves focus between sibling inputs and sets the caret
# B (apps/kira-space, W/memory)
U101 packages/workbench/src/approvals/ApprovalsDialog.vue # count marks approvals awaiting the person
U101 apps/kira-space/frontend/src/ade/v2/shell/AdeShell.vue # count badge marks sessions needing attention, not a tab count
U101 apps/kira-space/frontend/src/ade/v2/sessions/AdeSessionsMenu.vue # ADE tab count badge (P466)
U101 packages/theme/src/components/SecondaryTabs.vue # countBadge opt-in renders the ADE tab count badge (P466)
U101 apps/kira-space/frontend/mobile/TabBar.vue # count badge marks items needing attention, not a tab count
U91 apps/kira-space/frontend/src/workbench/OperationsPanel.vue # tooltip shows the repo root, not the truncated name
U91 packages/workbench/src/memory/import/ImportJobDetail.vue # tooltip lists the import roots, not the title text
U34 apps/kira-space/frontend/src/workbench/settings/MobileQr.vue # QR needs a white ground for scanner contrast
U37 apps/kira-space/frontend/src/ade/v2/plan/AdeTaskCard.vue # plan board task card selected fill, P129 board visual exempt per P263 §2.10
U37 apps/kira-space/frontend/src/ade/v2/plan/AdeBranchRow.vue # plan board branch row selected fill, board visual exempt per P263 §2.10
U37 apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue # plan board history and span row selected fill, board visual exempt per P263 §2.10
U37 apps/kira-space/frontend/src/studio/views/shared/DateTimePicker.vue # picker cell selection fill, not a list row (P525 rule 7)
U62 apps/kira-space/frontend/src/views/repo/useReadonlyFileViewer.ts # Monaco editor.focus(), not DOM focus
U62 apps/kira-space/frontend/src/views/repo/RepoDiffView.vue # Monaco editor.focus(), not DOM focus
U62 apps/kira-space/frontend/src/views/repo/useFileEdit.ts # Monaco editor.focus(), not DOM focus
U60 apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue # drags cards between day lists with sort off; useSortableReorder only reorders within one list
U62 apps/kira-space/frontend/src/ade/v2/notes/AdeNotesEditor.vue # TipTap chain().focus() commands, not DOM focus
# C (rest)
U91 packages/git-ui/src/components/UncommittedChangesStrip.vue # tooltip text differs from the visible label
U11 packages/docker-ui/src/components/LogsView.vue # ANSI bold is log data, not UI chrome
U41 packages/git-ui/src/App.vue # git engine host reads modifier keys
U41 apps/kira-space/frontend/src/capture/chord.ts # chord recorder reads modifier flags to build an accelerator string, not a shortcut
U62 packages/git-ui/src/components/CommitGrid.vue # SlickGrid engine host owns row focus
U47 packages/git-ui/src/components/SearchResults.vue # listbox of the SearchBox combobox; keys live in SearchBox
U46 packages/git-ui/src/graph/layoutClient.ts # main-thread worker fallback defers each request with its own zero-delay timer; debounce drops concurrent ones
U47 packages/git-ui/src/lib/gridKeyboard.ts # PageUp/PageDown over a streamed 100k-row store, not list semantics
PERMANENT

# `(?&t)` = the inside of one tag, quote-aware so `=>` or `a > b` inside an attribute value does not
# end the tag early.
PRE='(?(DEFINE)(?<t>(?:"[^"]*"|'"'"'[^'"'"']*'"'"'|[^>"'"'"'])*))'
TOK='(?<![\w-])'

# id@@files(v=.vue, vt=.vue+.ts, vc=.vue+.ts also in git-core/git-ipc/api-core, vm=.vue+.ts under mobile only, s=.vue+.ts in SIDE_PANELS only, vS=.vue under /settings/ only)@@built-in exempt paths (|; a name
# matches a file name or a repo-relative path, a /dir/ matches anywhere in the path)@@message@@regex
cat >"$TMP/guards" <<GUARDS
U1@@v@@@@<DialogContent without size=@@<DialogContent\b(?!(?&t)\bsize=)
U2@@v@@@@<DialogContent class sets width, height, padding or gap (size= owns them)@@<DialogContent\b(?&t)\bclass="[^"]*${TOK}(?:w|h|max-w|max-h|min-w|min-h|p|gap)-
U3@@v@@TextPromptDialog.vue@@role="dialog" outside TextPromptDialog.vue@@role="dialog"
U4@@v@@SecondaryTabs.vue@@<ToggleGroup/<ToggleGroupItem outside SecondaryTabs (use <SecondaryTabs>)@@<ToggleGroup(?:Item)?\b
U5@@v@@AdeShell.vue@@<TabsList/<TabsTrigger outside AdeShell.vue (use <SecondaryTabs>)@@<Tabs(?:List|Trigger)\b
U6@@vt@@TabStrip.vue|ModeSwitcher.vue|AdeShell.vue|ConsoleResultStrip.vue|ExecView.vue|AdeSessionStrip.vue@@tabChipVariants( outside primary tab bars@@tabChipVariants\(
U7@@v@@@@<Button without variant, or retired variant default/secondary/outline/destructive/toolbar-primary/danger@@<Button\b(?:(?!(?&t)\s:?variant=)|(?&t)\svariant="(?:default|secondary|outline|destructive|toolbar-primary|danger)")
U8@@v@@@@<Button without size, or retired size default/xs/sm/lg/icon/icon-xs/icon-lg/icon-sm@@<Button\b(?:(?!(?&t)\s:?size=)|(?&t)\ssize="(?:default|xs|sm|lg|icon|icon-xs|icon-lg|icon-sm)")
U9@@v@@NumberStepperInput.vue@@Input/NativeSelect/Textarea/InputGroup with size="default" or h-/rounded-/bg-/border-/px-/py- class@@<(?:Input|NativeSelect|Textarea|InputGroup)\b(?:(?&t)\ssize="default"|(?&t)\s:?class="[^"]*${TOK}(?:h|rounded|bg|border|px|py)-)
U10@@v@@@@raw <label>/<select> (use FieldLabel/Label, NativeSelect)@@<(?:label|select)\b
U11@@vt@@@@font-bold, font-semibold, font-extrabold or font-black (headings use SectionHeading, FieldLegend)@@${TOK}font-(?:bold|semibold|extrabold|black)(?![\w-])
U12@@v@@@@CodiconIcon :size outside 12/13/16/24@@<CodiconIcon\b(?&t)\s:size="(?!(?:12|13|16|24)")\d+"
U13@@v@@@@hand-rolled panel header/toolbar bar (use PanelHeader/ViewToolbar)@@\bclass="(?=[^"]*${TOK}h-bar(?![\w-]))(?=[^"]*${TOK}gap-1(?![\w.-]))(?=[^"]*${TOK}border-b(?![\w-]))(?:(?=[^"]*${TOK}px-1\.5(?![\w-]))(?=[^"]*${TOK}text-kira-sm(?![\w-]))|(?=[^"]*${TOK}px-2(?![\w-]))(?=[^"]*${TOK}shrink-0(?![\w-])))
U14@@v@@@@EmptyMedia variant="icon", or Alert faking an empty state@@<EmptyMedia\b(?&t)\svariant="icon"|<Alert\b(?&t)\sclass="[^"]*${TOK}bg-transparent
U15@@v@@@@PopoverContent width outside w-56/w-80/w-96/w-120/w-auto@@<PopoverContent\b(?&t)\s:?class="[^"]*${TOK}w-(?!(?:56|80|96|120|auto)(?![\w.\[(-]))
U16@@v@@@@DialogClose wrapping an icon-sm Button (use DialogHeader closable)@@<DialogClose\b(?&t)>\s*<Button\b(?&t)\ssize="icon-sm"
U17@@v@@SearchField.vue@@search/filter text box outside SearchField (use <SearchField>)@@<(?:Input|InputGroupInput|input)\\b(?&t)\\s:?placeholder="[^"]*(?:[Ss]earch|[Ff]ilter|[Ff]ind)|<InputGroupAddon\\b(?&t)>\\s*<CodiconIcon\\b(?&t)\\sname="search"
U18@@s@@@@hand-rolled row state (use rowVariants selected/muted)@@${TOK}(?:hover:)?bg-(?:select|hover)(?![\w-])
U19@@s@@@@hand-rolled indent or row height (use rowIndent, useRowHeight, rowVariants)@@depth\s*\*\s*\d+|rowDensity\s*===|(?:ROW_HEIGHT|rowHeight)\s*[:=]\s*\d+|${TOK}(?:h-5\.5|min-h-9)(?![\w-])
U20@@s@@@@hand-rolled panel chrome (use Empty class="h-full", PanelBar)@@side-empty|<Empty\b(?&t)\sclass="[^"]*${TOK}(?:p|px|py)-|shrink-0 border-b border-border px-1\.5 py-1
U21@@v@@PanelHeader.vue|SectionHeading.vue@@hand-rolled uppercase heading (use PanelHeader/SectionHeading)@@${TOK}uppercase(?![\w-])
U22@@v@@@@native title= on an HTML element (use Tooltip)@@<(?:span|div|button|i|li|dd|dt|a|label|Label|p|td|th|tr|img|code|small|section|header)\b(?&t)\s:?title=
U23@@vt@@Spinner.vue|StatusDot.vue@@animate-spin/animate-pulse/codicon-modifier-spin (use Spinner, Skeleton, StatusDot pulse)@@${TOK}animate-(?:spin|pulse)(?![\w-])|codicon-modifier-spin|~spin
U24@@v@@@@Alert class sets width/margin/fill/radius/border/padding, AlertDescription class, or variant="default"@@<Alert\b(?&t)\s:?class="[^"]*${TOK}(?:(?:w|m|mx|my|mt|mb|ml|mr|bg|p|px|py|pt|pb)-|(?:rounded|border)\b)|<AlertDescription\b(?&t)\s:?class="(?!whitespace-pre-wrap")|<Alert\b(?&t)\svariant="default"
U25@@v@@InlineState.vue@@p/div with text-error (use FieldError or InlineState)@@<(?:p|div)\b(?&t)\s:?class="[^"]*${TOK}text-error(?![\w-])
U26@@vt@@@@retired tone variant err/destructive/note, or a tone alpha (use Badge, Alert, StatusDot)@@<(?:Badge|Alert|DropdownMenuItem)\b(?&t)\svariant="(?:err|destructive|note)"|${TOK}(?:text|bg|border)-(?:ok|warn|info|error)/\d+
U27@@v@@StatusDot.vue@@hand-rolled status dot or pill (use StatusDot, Badge)@@\bclass="(?=[^"]*${TOK}rounded-full(?![\w-]))(?:(?=[^"]*${TOK}size-(?:1\.5|2|2\.5)(?![\w.-]))|(?=[^"]*${TOK}w-2(?![\w.-]))(?=[^"]*${TOK}h-2(?![\w.-]))|(?=[^"]*${TOK}w-2\.5(?![\w.-]))(?=[^"]*${TOK}h-2\.5(?![\w.-])))|<span\b(?&t)\s:?class="(?=[^"]*${TOK}rounded-full(?![\w-]))(?=[^"]*${TOK}px-)
U28@@v@@SwatchRadio.vue@@raw checkbox/radio input (use Checkbox, RadioGroup)@@type="(?:checkbox|radio)"
U28@@vS@@@@Checkbox in a settings pane (a live boolean is a Switch)@@<Checkbox\b
U28@@v@@/settings/|MemoryPanel.vue|AdeMainTab.vue|ReposDialog.vue@@Switch outside settings panes (a form option is a Checkbox)@@<Switch\b
U29@@v@@@@TooltipProvider with props (the theme owns the delays), or TooltipContent class other than whitespace-pre-wrap@@<TooltipProvider\b(?!\s*>)|<TooltipContent\b(?&t)\s:?class="(?!whitespace-pre-wrap")
U30@@v@@@@DropdownMenuContent width, or DropdownMenuItem height/padding/gap class@@<DropdownMenuContent\b(?&t)\s:?class="[^"]*${TOK}(?:w|min-w|max-w)-|<DropdownMenuItem\b(?&t)\s:?class="[^"]*${TOK}(?:h|py|px|gap)-
U31@@v@@@@hand-written resize cursor or handle look, or a separator without resizeHandleVariants@@${TOK}cursor-(?:col|row|ew|ns)-resize|<ResizeHandle\b(?&t)\s:?class="[^"]*${TOK}(?:w-|h-|bg-|shadow|cursor|outline)|(?s)\A(?!.*resizeHandleVariants).*role="separator"
U32@@v@@@@scrollbar styling in a component (use scrollbar-none, base.css)@@\[scrollbar-|${TOK}scrollbar-(?!none(?![\w-]))[\w-]+|::-webkit-scrollbar
U33@@v@@@@keyboard hint text (use Kbd)@@Cmd/Ctrl|\(\{\{\s*formatShortcut
U34@@vt@@monacoTheme.ts|/setiFileIcon|/ade/v2/palette.ts@@colour literal in app code (use tokens)@@(?:["'\`\[(]|:\s*)#(?:[0-9a-fA-F]{6}(?:[0-9a-fA-F]{2})?|(?=[0-9]*[a-fA-F])[0-9a-fA-F]{3,4}|(?<d>\d)\k<d>\k<d>\k<d>?)\b|${TOK}(?:bg|text|border)-(?:black|white)(?![\w-])
U35@@v@@AdeNotesEditor.vue|AutocompleteField.vue@@outline-none, border-focus, not-allowed cursor or odd disabled opacity (one focus and disabled recipe)@@${TOK}outline-(?:none|hidden|0)(?![\w-])|focus:outline|focus-visible:outline-|${TOK}focus(?:-visible|-within)?:border-focus|${TOK}cursor-not-allowed|${TOK}disabled:opacity-(?!50(?![\w-]))|data-\[off\]
U36@@v@@PickerTrigger.vue@@hand-rolled box (use Card)@@\bclass="(?=[^"]*${TOK}rounded-kira(?:-sm)?(?![\w-]))(?=[^"]*${TOK}border(?![\w-]))(?=[^"]*${TOK}(?:p|px|py)-)
U37@@vt@@rowVariants.ts@@raw table markup, density formula, or hand-rolled row state (use Table, useRowHeight, rowVariants)@@<(?:table|thead|tbody|tr|th|td)\b|rowDensity\s*===|${TOK}(?:hover:)?bg-select(?![\w-])
U38@@v@@@@scale leftovers: text-kira-xs, stock text size/radius, text-[Npx], raw h-5..h-12@@${TOK}text-kira-xs(?![\w-])|${TOK}text-(?:xs|sm|base|lg|xl|2xl)(?![\w-])|${TOK}text-\[\d|${TOK}rounded(?:-md|-lg|-xl)?(?![\w-])|${TOK}h-(?:5|6|7|8|9|10|12)(?![\w.-])
U39@@vt@@/behaviour/!|/testing/!@@instanceof Error ? message : String (use errorText)@@instanceof\s+Error\s*\?\s*[\w.]+\.message\s*:\s*String\(
U40@@vt@@/behaviour/!|/testing/!@@@keydown.<modifier>, @keydown.context-menu or onKeyStroke (use useShortcut)@@@keydown\.(?:meta|ctrl|alt|shift)|@keydown\.context-menu|\bonKeyStroke\(
U41@@vt@@/behaviour/!|/testing/!|keys.ts@@raw metaKey/ctrlKey read (use SHORTCUTS and useShortcut)@@\.(?:metaKey|ctrlKey)\b
U42@@vt@@/behaviour/!|/testing/!@@useClipboard or navigator.clipboard (use useCopy, copyText)@@useClipboard\(|navigator\.clipboard
U43@@vt@@/behaviour/!|/testing/!@@direct web storage (use usePersistedState)@@use(?:Local)?Storage(?:<[^>]*>)?\(|\blocalStorage\.|\bsessionStorage\b
U44@@vt@@/behaviour/!|/testing/!@@useVirtualizer (use useVirtualRows)@@useVirtualizer\(
U45@@vt@@/behaviour/!|/testing/!@@reka splitter, pointer capture or useDraggable (use ResizeHandle, usePaneResize)@@<(?:ResizablePanelGroup|ResizablePanel|ResizableHandle|KuiColumnResizeHandle|DockResizeHandle|AdePanelResizeHandle)\b|setPointerCapture|import\b[^;]*\buseDraggable\b[^;]*from '@vueuse/core'
U46@@vt@@/behaviour/!|/testing/!@@numeric timer delay or setTimeout/setInterval (use TIMING with VueUse)@@\b(?:useDebounceFn|useThrottleFn|useIntervalFn|useTimeoutFn)\((?:[^()]|\([^()]*\))*,\s*\d[\d_]*\s*[,)]|\b(?:refDebounced|watchDebounced)\((?:[^()]|\([^()]*\))*,\s*(?:\d[\d_]*\s*[,)]|\{[^}]*\bdebounce:\s*\d)|refetchInterval:\s*\d|const \w*(?:DEBOUNCE|DELAY|POLL|TICK|INTERVAL)\w*_MS\s*=\s*\d|(?:debounce|delay):\s*\d{2,}|copiedDuring|\bsetTimeout\(|\bsetInterval\(
U47@@vt@@/behaviour/!|/testing/!@@ArrowUp/ArrowDown, or role listbox/tree without useListNavigation/useActiveDescendant@@'Arrow(?:Down|Up)'|(?s)\A(?!.*(?:useListNavigation|useActiveDescendant)).*role="(?:listbox|tree)"
U48@@vt@@/behaviour/!|/testing/!@@RowContextMenu or useRowMenu (use the context menu store)@@<RowContextMenu\b|useRowMenu\(
U49@@v@@/behaviour/!|/testing/!@@AdeConfirmDialog, window.confirm or a local ConfirmDialog (use the confirm store or ConfirmDialogView)@@<AdeConfirmDialog\b|window\.confirm|import ConfirmDialog from '\.\./components/
U50@@vt@@/behaviour/!|/testing/!@@input committing on blur and Enter, or useCommitField (use InlineEditInput, useInlineEdit)@@<(?:input|Input|InputGroupInput)\b(?=(?&t)\s@blur=)(?=(?&t)\s@keydown\.enter)|useCommitField\(
U51@@vt@@/behaviour/!|/testing/!@@hand-rolled pending ref (use useAsyncAction or mutation.isPending)@@const \w*(?:[Bb]usy|[Pp]ending|[Ss]aving|[Ll]oading|[Ss]ubmitting|[Rr]unning|[Dd]eleting|[Rr]etrying|[Cc]opying)\s*=\s*ref\b
U52@@vt@@/behaviour/!|/testing/!|/theme/src/format.ts|/theme/src/i18n/number.ts@@locale or Intl formatting, useTimeAgo, local format helper (use behaviour/format)@@toLocaleString\(|toLocaleDateString|toLocaleTimeString|Intl\.(?:NumberFormat|DateTimeFormat|RelativeTimeFormat)|useTimeAgo\(|function (?:plural|formatDuration|formatElapsed|formatSize|formatTime|formatMs)\b
U53@@vt@@/behaviour/!|/testing/!|queryClient.ts@@own QueryClient or a query option restating the createQueryClient default@@new QueryClient\b|retry:\s*false|refetchOnWindowFocus:\s*false|staleTime:\s*(?:Infinity|Number\.POSITIVE_INFINITY)
U53@@v@@/behaviour/!|/testing/!@@explicit queryClient argument in a component (inject it with useQueryClient)@@,\s*queryClient\s*,?\s*\)
U54@@v@@/behaviour/!|/testing/!@@v-if on isLoading/isError (use QueryState)@@v-(?:else-)?if="[^"]*\.(?:isLoading|isError)\b
U55@@vt@@/behaviour/!|/testing/!@@onMutate (use optimisticUpdate)@@onMutate:
U56@@vt@@/behaviour/!|/testing/!@@localeCompare, Collator or lowercase includes (use matchesQuery, compareNatural)@@localeCompare\(|new Intl\.Collator|\.toLowerCase\(\)\.includes\(
U57@@vt@@/behaviour/!|/testing/!@@manual scroll-to-end or scrollIntoView (use useFollowTail, revealElement)@@scrollTop\s*=\s*\w+(?:\.value)?\.scrollHeight|scrollIntoView\(
U58@@vt@@/behaviour/!|/testing/!@@computeFloatPosition or pointReference (use PointPopover)@@computeFloatPosition|pointReference
U59@@vt@@/behaviour/!|/testing/!@@placeholder dash literal (use displayValue)@@(?:\?\?|\|\|)\s*'(?:—|-)'|return\s+'(?:—|-)'
U60@@vt@@/behaviour/!|/testing/!@@vue-draggable-plus (use useSortableReorder)@@from 'vue-draggable-plus'
U61@@vt@@/behaviour/!|/testing/!@@empty catch (give a reason comment or surface the error)@@catch\s*(?:\(\w*\))?\s*\{\s*\}|\.catch\(\(\)\s*=>\s*(?:\{\s*\}|undefined)\)
U62@@vt@@/behaviour/!|/testing/!|SearchField.vue@@manual focus() (use useFocusOnMount, useFocusReturn, useListNavigation)@@\.focus\(\)|nextTick\([^)]*\.focus
U63@@s@@/behaviour/!|/testing/!@@side panel with SearchField and no usePanelHeaderSearch@@(?s)\A(?!.*usePanelHeaderSearch).*<SearchField
U72@@vt@@@@@lucide/vue import outside the theme ui primitives (use CodiconIcon, ICON)@@['"]@lucide/vue
U73@@vt@@/theme/src/icons.ts@@retired icon name gear/settings/kebab-vertical, or a 'codicon-' string literal (use ICON)@@(?<!<slot )\b(?:name|icon)(?:=|:\s*)["']{1,2}(?:gear|settings|kebab-vertical)["']|['"\x60]codicon-
U74@@v@@EmptyState.vue@@<Empty outside EmptyState.vue (use EmptyState)@@<Empty\b
U75@@vt@@@@duration-, ease-, animate-in, animate-out outside theme (motion tokens own them)@@${TOK}(?:duration|ease)-[\w\[]|${TOK}animate-(?:in|out)(?![\w-])
U76@@vt@@sortableReorder.ts@@ghostClass/chosenClass outside sortableReorder.ts (use SORTABLE_GHOST_CLASS; no chosen class)@@\b(?:ghost|chosen)Class:
U77@@vt@@@@bare void on a bridge call (use fireAndForget)@@\bvoid\s+(?:(?:ctx|props)\.)?(?:control|bridge)\.\w+\(
U78@@s@@@@Button inside EmptyState in a side panel (the front door has no action button)@@(?s)<EmptyState\b(?&t)(?<!/)>(?:(?!</EmptyState>).)*?<Button\b
U79@@vt@@apps/kira-space/frontend/src/bridge/index.ts@@direct linkOpenExternal or LinkService.OpenExternal (use createOpenExternal)@@linkOpenExternal\(|LinkService\.OpenExternal\(
U80@@v@@@@ShowMoreButton without useCappedList@@(?s)\A(?!.*useCappedList\().*ShowMoreButton
U80@@vt@@/behaviour/!|/testing/!@@hand-rolled render cap (use useCappedList)@@capRows\(|_ROW_CAP\b
U81@@vt@@@@empty .catch(() => {}) (use fireAndForget or give the error a surface)@@\.catch\(\s*\(\s*\w*\s*\)\s*=>\s*\{\s*\}\s*\)
U82@@vc@@@@.slice(0, 7|8) on a sha (use shortSha)@@\.slice\(\s*0\s*,\s*[78]\s*\)
U82@@vm@@@@h-11, size-11 or min-h-11 in mobile (use h-touch, size-touch, min-h-touch)@@${TOK}(?:h|size|min-h)-11(?![\w.-])
U83@@v@@@@variant="ghost" outside DialogHeader (use toolbar, link or a tone)@@variant="ghost"|'ghost'
U84@@v@@@@<Button class sets a size (the size variants own h-touch, size-touch, h-control-sm, h-auto p-0)@@<Button\b(?&t)\s:?class="(?:[^"]*${TOK}(?:h-touch|size-touch|h-control-sm)(?![\w.-])|(?=[^"]*${TOK}h-auto(?![\w-]))(?=[^"]*${TOK}p-0(?![\w-])))
U85@@v@@@@<EmptyState class sets padding or fill (use the fill prop)@@<EmptyState\b(?&t)\s:?class="[^"]*${TOK}(?:(?:p|px|py)-|h-full(?![\w-])|flex-1(?![\w-]))
U86@@vt@@@@z-index off the token ladder (use z-(--kira-z-*) tiers; in-pane z-0..3)@@${TOK}z-(?:\d{2,}|\[)
U87@@v@@Disclosure.vue@@<details (use Disclosure)@@<details\b
U88@@vt@@@@off-scale density (scale is 0.5, 1, 1.5, 2, 3, 4)@@${TOK}(?:gap|gap-x|gap-y|p|px|py)-(?:0\.25|1\.25|1\.75|2\.5|3\.5)(?![\w.-])
U89@@vt@@linkify.ts|badgeClass.ts@@cursor-pointer outside link owners, or cursor-help/copy (controls and rows are cursor-default)@@${TOK}cursor-(?:pointer|help|copy)(?![\w-])
U90@@vt@@@@hover tone override, or opacity as muted (use tone, text-muted-foreground)@@hover:text-(?:ok|error|warn|primary)(?![\w-])|${TOK}opacity-(?:40|60|70|75|80)(?![\w.-])
U91@@vt@@@@hand-written truncate, or tooltip-wrapped truncate (use TruncatedText)@@overflow-hidden text-ellipsis whitespace-nowrap|<TooltipTrigger\b(?&t)>\s*<(?:span|div|button)\b(?&t)\s:?class="[^"]*${TOK}truncate(?![\w-])
U92@@v@@SplitButton.vue@@<Button :disabled carries a pending flag, or Spinner inside Button (use pending)@@<Button\b(?&t)\s:disabled="[^"]*(?i:pending|busy|saving|loading|running|installing|committing)|(?s)<Button\b(?&t)(?<!/)>(?:(?!</Button>).)*?<Spinner\b
U93@@vt@@/theme/src/format.ts|/testing/!@@toFixed or {{ }}% (use formatPercent, formatBytes, formatDuration)@@toFixed\(|\}\}%
U94@@v@@@@heading element without text-kira-lg font-medium@@<h[1-6]\b(?!(?=(?&t)\sclass="[^"]*${TOK}text-kira-lg(?![\w-]))(?=(?&t)\sclass="[^"]*${TOK}font-medium(?![\w-])))
U95@@v@@@@hand hint paragraph (use FieldDescription)@@<p\b(?&t)\s:?class="(?=[^"]*${TOK}text-kira-sm(?![\w-]))(?=[^"]*${TOK}text-(?:muted-foreground|subtle)(?![\w-]))
U96@@v@@@@useListNavigation list with @contextmenu but no nav.contextMenu( (select before the menu)@@(?s)\A(?=.*useListNavigation\()(?=.*@contextmenu=)(?!.*\.contextMenu\()
U97@@v@@@@kebab-case emit, 'update:modelValue', or *TestId prop (use camelCase emits, defineModel, *Testid)@@defineEmits<\{[^}]*'[a-z]+-[a-z]|'update:modelValue'|\b\w+TestId\b
U98@@vt@@@@fixed row height (use useRowHeight, ROW_HEIGHT_PX)@@rowHeight:\s*\(\)\s*=>\s*\d|const \w*ROW_HEIGHT\w*\s*=\s*\d
U99@@vt@@@@log scope or fireAndForget scope not a camelCase module id@@\blog(?:Warn|Error)\(\s*(?!'[a-z]+(?:[A-Z][a-z0-9]*)*(?:\.[a-z][A-Za-z0-9]*)?')(?:'|"|\x60)|\bfireAndForget\((?:[^()]|\((?:[^()]|\((?:[^()]|\([^()]*\))*\))*\))*?,\s*(?:'(?![a-z]+(?:[A-Z][a-z0-9]*)*(?:\.[a-z][A-Za-z0-9]*)?')[^']*'|"[^"]*"|\x60[^\x60]*\x60)\s*,?\s*\)
U100@@v@@@@form <DialogContent without dirty (the discard guard)@@(?s)\A(?=.*<DialogContent\b)(?=.*(?:<Input\b|<Textarea\b|<MonacoHost\b|<AutocompleteField\b|<NativeSelect\b))(?!.*<DialogContent\b(?&t)\s:?dirty(?![\w-]))
U101@@v@@@@Badge variant="count" outside attention owners (a plain count is rowMeta)@@<Badge\b(?&t)\svariant="count"
U102@@v@@@@shortSha(/shortId( in a component (use Identifier :short)@@\bshort(?:Sha|Id)\(
U103@@vt@@@@bare .sort() (end on a stable key)@@\.sort\(\)
U104@@v@@@@toolbar divider must use ToolbarSeparator (P351)@@class="[^"]*(?:\bw-px\b[^"]*\bh-3\.5\b|\bh-3\.5\b[^"]*\bw-px\b)
U104@@v@@ToolbarSeparator.vue@@toolbar divider must use ToolbarSeparator (P351)@@<Separator\b(?&t)\sorientation="vertical"
U105@@vt@@AdeDayBand.vue@@repeating gradient texture (surfaces are flat tokens)@@repeating-(?:linear|radial|conic)-gradient
U106@@v@@@@filled button (outline or toolbar; see P525 rules)@@<(?:Button|TooltipIconButton|SplitButton)\b(?&t)\s:?class="[^"]*${TOK}bg-(?:primary|agent|tone-[a-z]+-solid|ok|warn|error|info)(?![\w/-])
U107@@v@@@@icon in a dialog header (title, description and actions only)@@(?s)<DialogHeader\b(?&t)>(?:(?!</DialogHeader>|<Button\b|<TooltipIconButton\b).)*?<(?:CodiconIcon|EngineIcon|svg)\b
U108@@v@@@@toolbar pressed look by class (use aria-pressed / aria-expanded; the toolbar variant draws it)@@<(?:Button|TooltipIconButton)\b(?&t)\s:class="[^"]*'bg-field
GUARDS

# hits: "U<n> path"
: >"$TMP/hits"
: >"$TMP/used_exempt"
gn=0
while IFS= read -r line; do
  gn=$((gn + 1))
  id=${line%%@@*}; rest=${line#*@@}
  kinds=${rest%%@@*}; rest=${rest#*@@}
  exempt=${rest%%@@*}; rest=${rest#*@@}
  rest=${rest#*@@}
  regex=$rest
  case "$kinds" in
    vt | vc | vm) inc="--include=*.vue --include=*.ts" ;;
    *) inc="--include=*.vue" ;;
  esac
  scan=$DIRS; flag=-r
  if [ "$kinds" = vc ]; then scan="$DIRS $CORE_DIRS"; fi
  if [ "$kinds" = vm ]; then scan=apps/kira-space/frontend/mobile; fi
  if [ "$kinds" = s ]; then
    inc=""; flag=""
    scan=$(cat "$TMP/side_panels")
  elif [ "$kinds" = vS ]; then
    inc=""; flag=""
    # shellcheck disable=SC2086
    scan=$(find $DIRS -path '*/settings/*' -name '*.vue')
  fi
  set +e
  # shellcheck disable=SC2086
  "$GNU_GREP" $flag -Pzl $inc -e "$PRE$regex" $scan >"$TMP/out" 2>"$TMP/err"
  rc=$?
  set -e
  if [ "$rc" -gt 1 ]; then
    echo "check-ui-primitives: grep failed for $id:" >&2
    cat "$TMP/err" >&2
    exit 2
  fi
  tr '\0' '\n' <"$TMP/out" | while IFS= read -r f; do
    if [ -z "$f" ]; then continue; fi
    # The primitives themselves are never scanned (theme is, minus components/ui).
    case "$f" in packages/theme/src/components/ui/*) continue ;; esac
    skip=0
    if [ -n "$exempt" ]; then
      old_ifs=$IFS; IFS='|'
      for e in $exempt; do
        e=${e%!}
        case "$e" in
          /*) case "$f" in *"$e"*) skip=1; echo "$gn $e" >>"$TMP/used_exempt" ;; esac ;;
          *) case "$f" in "$e" | */"$e") skip=1; echo "$gn $e" >>"$TMP/used_exempt" ;; esac ;;
        esac
      done
      IFS=$old_ifs
    fi
    if [ "$skip" -eq 0 ]; then echo "$id $f"; fi
  done >>"$TMP/hits"
done <"$TMP/guards"
sort -u "$TMP/hits" -o "$TMP/hits"

sed -e '/^#/d' -e 's/[[:space:]]*#.*$//' "$TMP/perm" | sort -u >"$TMP/perm_ids"
mv "$TMP/perm_ids" "$TMP/perm"

sort -u "$TMP/perm" -o "$TMP/perm"

STATUS=0
: >"$TMP/used_perm"
while read -r id f; do
  if grep -qxF "$id $f" "$TMP/perm"; then
    echo "$id $f" >>"$TMP/used_perm"
  else
    msg=$(grep "^$id@@" "$TMP/guards" | head -1 | awk -F'@@' '{print $4}')
    echo "check-ui-primitives: $f: $id $msg" >&2
    STATUS=1
  fi
done <"$TMP/hits"

sort -u "$TMP/used_perm" -o "$TMP/used_perm"
while read -r id f; do
  if ! grep -qxF "$id $f" "$TMP/used_perm"; then
    echo "check-ui-primitives: stale exemption: $id $f" >&2
    STATUS=1
  fi
done <"$TMP/perm"

# A built-in exempt name that exempts nothing fails; a trailing `!` (blanket dirs) opts out.
gn=0
while IFS= read -r line; do
  gn=$((gn + 1))
  rest=${line#*@@}; rest=${rest#*@@}
  exempt=${rest%%@@*}
  if [ -z "$exempt" ]; then continue; fi
  old_ifs=$IFS; IFS='|'
  for e in $exempt; do
    case "$e" in *!) continue ;; esac
    if ! grep -qxF "$gn $e" "$TMP/used_exempt"; then
      echo "check-ui-primitives: stale built-in exemption: ${line%%@@*} $e" >&2
      STATUS=1
    fi
  done
  IFS=$old_ifs
done <"$TMP/guards"

if [ "$STATUS" -ne 0 ]; then
  echo "check-ui-primitives: see git show bd0c224688d9:docs/v2.2/plans/P262-ui-unification.md, P263-ui-unification-2.md §2 and P264-behaviour-unification.md §3.3 and P266-consistency-audit.md §4.4." >&2
else
  echo "check-ui-primitives: every guarded element follows the P262-P266 rules."
fi
exit $STATUS
