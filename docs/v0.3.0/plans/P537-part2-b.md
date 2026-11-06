# P537 Part 2, Stream B notes

Frontend merge. Base 677d6c60.

## Landed

- One entry. `src/main.ts` boots Studio data (`studio/boot.ts`), debug hooks (`studio/debugHooks.ts`),
  proof windows (`studio/proof/boot.ts`). `studio/{main.ts,App.vue,proof/ProofApp.vue}` gone.
- One `AppMode` in `packages/shared/domain/mode.ts`. `state/modeDomain.ts` gone.
- One `workbench/modes.ts`; `STUDIO_MODES` marks the modules on the Studio backend.
- One store each: mode, tabs, layout, keepAwake, appUpdate, appMetrics, terminals, terminalTabs,
  customScripts, pinia. Second instances only where the plan keeps two: `settings` and
  `studioSettings`, plus the op logs (`opLog`, `studioOpLog`).
- Tab kinds: `TabKind = SpaceTabKind | StudioTabKind`. Studio modes are workspaces keyed by mode id
  (`isRepoWorkspace` skips them when deriving open repos). `terminal` and `script-run` are Space
  kinds only on the TS side.
- Bridge hub: `bridge/index.ts` exports `control` and `coreBindings`. `studio/bridge/index.ts`
  layers Studio's `control` on top.
- One `SettingsDialog`. Draft spans both stores; `save` splits leaves by owner. Studio's
  `advanced` leaves live in the draft as `studioAdvanced`.
- One `App.vue` routes capture, host view, review, opened files, proof, workbench.

## Intentional same-name overrides in Studio's `control`

Studio files import `studio/bridge/control`; Space files import the hub. These names bind a Studio
service there: `settingsGetAll`, `settingsSet`, `onSettingsChanged`, `opsRecent`, `opsCancel`,
`onOpUpdate`, `configFilesOpen`, `configFilesReveal`.

## Moved shortcuts and palette commands

No frontend key binding clashed. Accelerators live in the Go menu (Stream A). Palette: Space's
`view.find`, tab-next, tab-prev, tab-close, toggle panels win. Studio's duplicates of those went.
Tab-next/prev/close call the Kubernetes tab store in Kubernetes mode, else the tabs store.
Studio palette strings `studio.shortcuts.palette.{placeholder,empty,nextTab,previousTab,closeTab,toggleProjectPanel,toggleOperationsPanel}`
dropped; the palette uses `space.shortcuts.palette.*`.

## Renamed testids

None. Dropped with Studio's duplicate panes: `settings-log-level` (Studio log level; Space keeps
`settings-git-log-level`). Studio Appearance pane testids live on in Space's pane
(`settings-row-coloring`, `settings-reset-appearance-rowColoring`).

## Settings schema

Studio's TS `Settings` no longer carries `appearance`, `agent`, `advanced.logLevel`,
`advanced.notifyPrompts`. Studio code reads appearance, `appearanceVersion` from the Space store.
Space's store applies the theme; `createSettingsStore` skips appearance for a store without that
section. Go may keep sending the keys; the TS store ignores them. Stream A decides the Go payload.

## Copy

`kiraStudio` product label removed; Studio start page and schedule fields say Kira Space. Studio
settings keys for deleted panes dropped (`check-catalog --unused` clean).

## Landing checks for the orchestrator

- `knip.json` is Stream C's. The temporary `src/studio/main.ts` entry no longer exists; remove it.
- `scripts/check-ui-primitives.sh` U79: dropped `studio/bridge/index.ts` from the exemption (it no
  longer calls `LinkService`; Studio inherits the hub's `openExternal`). One line, C-owned file.
- `tests/ui/**` is not Stream B's. One import fixed so `typecheck:space-tests` passes:
  `tests/ui/studio/support/mockRuntime.ts` imported `studio/state/tabDomain` (now `state/tabDomain`).
  Several `tests/ui/studio` files still name `studio/main.ts` in comments. Studio ui specs that open
  Settings, or assert Studio's Appearance/Advanced panes, or boot Studio's `index.html`, need the
  merged config from Stream C.
- `tests/unit/studio/go-ts-vocabulary-parity.spec.ts` reads `internal/studiostorage/model/tabs.go`.
  If Stream A merges that Go list into `internal/storage/model`, repoint it and compare against
  `SPACE_RENDERABLE_TAB_KINDS` plus `STUDIO_RENDERABLE_TAB_KINDS`.
- `tests/unit/studio/bridge-unwrap.spec.ts` skips methods Studio's control inherits from the hub.
- `tabs-save-*.spec.ts` patch the hub `control`, since the merged tabs store saves through it.

## Unused dependencies

None. `lint:dead` reports no unused dependency. Its only hint is the `src/studio/main.ts` knip entry (C).
