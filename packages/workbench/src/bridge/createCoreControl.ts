import { type AgentInfo, agentInfoSchema } from '@shared/domain/agent';
import type { LayoutPatch } from '@shared/domain/layout';
import {
  type LlmPromptChange,
  type LlmPromptList,
  type LlmPromptPreview,
  type LlmPromptPreviewArgs,
  type LlmPromptsChangedEvent,
  llmPromptListSchema,
  llmPromptPreviewSchema,
} from '@shared/domain/llmPrompts';
import type { QuitStatus, RoutedPrompt } from '@shared/domain/prompts';
import type { TerminalLaunchKind } from '@shared/domain/tabs';
import { type AppMetricsSample, CHANNEL, type TerminalEvent } from '@shared/protocol/events';
import { on, trust, unwrap, windowKey } from './rpc';

// P103 Part 2 (§5.6): the 20 bound-call methods that were byte-for-byte identical between Kira
// Studio's own studioControl object (bridge/index.ts) and Kira Space's control object (§1.6/§1.7's
// own "confirmed shared" survey, re-verified here by manually counting both files rather than
// trusting either's own stale internal comment) — settings/layout get-all-and-set-plus-changed,
// the quit and close-window flush handshakes, the folder picker, tabsList/tabsSave, and the five
// terminal methods. Everything else (94 more methods in Kira Studio's own
// studioControl, 24 more in Kira Space's) stays app-side, unmoved, in each app's own bridge/index.ts.
//
// P116 H5 adds ten more, moved out of Kira Studio's own studioControl once Kira Space grew the
// same window-chrome gaps (G1-G5/G7): the five menu-pushed signal channels (open-settings,
// toggle-project-panel, tab-next/prev/close), the keep-awake toggle's read/write/broadcast, the
// app-metrics broadcast, and the title bar's "New window" button. P188: the
// agent-aware keep-awake setting lives in Kira Space's own Settings, not a bound method.
//
// P119 adds three more: updateStatus/updateInstall/updateCancelInstall — both apps' own
// UpdateService is now byte-identical (Status/InstallUpdate/CancelInstall over the shared
// internal/appupdate), so the update dialog's whole control surface moves here rather than staying
// duplicated per app the way studioControl's updateStatus/updateOpenReleasePage used to be.
//
// P128 §2.3 adds windowsEnsure/windowsSetMode — both apps now persist a per-window module mode
// (internal/windowsvc.Service, shared since that phase), so the fifth generic `M` is each app's own
// mode union (Studio's AppMode, Kira Space's SpaceMode).
//
// `CoreBindings` is a **structural** interface, not an adapter: each app's own generated
// `@bindings/*` service modules (SettingsService, LayoutService, TabsService, LifecycleService,
// TerminalService, FilesService) already satisfy it by shape — no per-app glue code,
// no change to Wails' own binding generation. Every method here is typed against `unknown`
// deliberately (the plan's own §5.6 sketch): a shared package can't import either app's own
// `@bindings/*` alias (it resolves to a different generated module per app, and CLAUDE.md's "a
// shared package never imports apps/*/internal/..." rules out depending on either app's generated
// output directly), and TypeScript's method-shorthand parameter checking is bivariant, so a real
// service's own narrower argument type (e.g. TerminalService.Open's `launchKind: TerminalLaunchKind`
// against this interface's plain `string`) still satisfies it in either direction. `tabs.Save`'s
// own `tabs` field stays scalar `unknown` rather than `unknown[]`: each app's real TabsService.Save
// takes `tabs: TabRecord[]` for that app's own (large, discriminated-union) TabRecord, and the
// bivariant check that lets every other field here stay narrow didn't clear an array of one —
// `unknown` (checked once as a whole, not per element) does.
export interface CoreBindings {
  settings: {
    GetAll(): Promise<unknown>;
    Set(a: { patch: unknown }): Promise<unknown>;
  };
  layout: {
    GetAll(): Promise<unknown>;
    Set(a: { patch: unknown }): Promise<unknown>;
  };
  tabs: {
    List(a: { windowKey: string }): Promise<unknown>;
    Save(a: { windowKey: string; tabs: unknown }): Promise<void>;
  };
  lifecycle: {
    Flushed(a: { windowKey: string }): void;
    WindowFlushed(a: { windowKey: string }): void;
    ConfirmQuit(): Promise<void>;
    CancelQuit(): Promise<void>;
    QuitStatus(): Promise<unknown>;
  };
  terminal: {
    DefaultCwd(): Promise<unknown>;
    Open(a: {
      terminalId: string;
      cwd: string;
      cols: number;
      rows: number;
      windowKey: string;
      command: string;
      launchKind: string;
      scriptId: string;
      scriptLaunchToken: string;
      agentLaunchToken: string;
    }): Promise<unknown>;
    Write(a: { terminalId: string; data: string }): Promise<void>;
    Resize(a: { terminalId: string; cols: number; rows: number }): Promise<void>;
    Close(a: { terminalId: string }): Promise<void>;
  };
  files: {
    ChooseFolder(a: { title: string }): Promise<unknown>;
  };
  agent: {
    Info(): Promise<unknown>;
    InfoFor(id: string): Promise<unknown>;
  };
  windows: {
    OpenNew(): Promise<void>;
    Ensure(a: { windowKey: string }): Promise<unknown>;
    SetMode(a: { windowKey: string; mode: string }): Promise<void>;
  };
  keepAwake: {
    Status(): Promise<unknown>;
    SetManual(a: { enabled: boolean }): Promise<unknown>;
  };
  update: {
    Status(): Promise<unknown>;
    Check(): Promise<unknown>;
    InstallUpdate(): Promise<void>;
    CancelInstall(): Promise<void>;
  };
  llmPrompts: {
    List(): Promise<unknown>;
    Save(a: { changes: LlmPromptChange[] }): Promise<unknown>;
    Preview(a: { id: string; text?: string; vars?: unknown }): Promise<unknown>;
  };
  prompts: {
    List(): Promise<unknown>;
    Claim(a: { id: string; windowKey: string }): Promise<void>;
    Raise(a: { kind: string; ref: string }): Promise<void>;
    Dismiss(a: { id: string }): Promise<void>;
    SendTest(): Promise<void>;
  };
}

// The folder-picker result — identical shape in both apps' own generated
// FilesChooseFolderResult (`{ canceled: boolean; path: string | null }`, confirmed by reading both
// apps' generated bridge/models.ts side by side), restated here rather than imported from either
// app's own `@bindings/*` output for the same reason `CoreBindings` above stays on `unknown`.
interface FolderChoice {
  canceled: boolean;
  path: string | null;
}

// KeepAwakeStatus is both apps' own bound KeepAwakeService wire type, restated here rather than
// imported from either app's own `@bindings/*` output — FolderChoice's own reasoning applies.
export interface KeepAwakeStatus {
  manual: boolean;
  supported: boolean;
  error: string;
}

// AppUpdateStatus is both apps' own bound UpdateService.Status wire type, restated here rather
// than imported from either app's own `@bindings/*` output — FolderChoice's own reasoning applies.
export interface AppUpdateStatus {
  updateAvailable: boolean;
  currentVersion: string;
  latestVersion: string;
  installLogPath: string;
}

// `S`/`L`/`T`/`P` are each app's own Settings, Layout, TabRecord and SettingsPatch shape. Layout is
// the same type in both apps today (both import `@shared/domain/layout`'s `Layout`
// verbatim, with no bridge-level narrowing — unlike SettingsShell.vue's own `Pick<Settings, …>`
// narrowing, which is a UI-layer concern, not a bridge one), but TabRecord genuinely
// differs (each app's own `state/tabDomain.ts` declares a different `z.discriminatedUnion`), and
// Settings/SettingsPatch now genuinely differ too (each app's own
// `state/settingsDomain.ts`, not one shared `@shared/domain/settings` shape) — so all four stay
// generic, matching the `createCoreControl<Settings, Layout, TabRecord, SettingsPatch>
// (...)` call-site shape, rather than only genericizing the ones that must be.
export interface CoreControl<S, L, T, P, M> {
  settingsGetAll: () => Promise<S>;
  settingsSet: (patch: P) => Promise<S>;
  onSettingsChanged: (cb: (settings: S) => void) => () => void;

  layoutGetAll: () => Promise<L>;
  layoutSet: (patch: LayoutPatch) => Promise<L>;
  onLayoutChanged: (cb: (layout: L) => void) => () => void;

  // Quit handshake (P8 C8) and its close-window analogue (P8 C6/F8) — identical in both apps.
  onFlushBeforeClose: (cb: () => void) => () => void;
  appFlushed: () => void;
  onWindowFlushBeforeClose: (cb: () => void) => () => void;
  windowFlushed: () => void;

  filesChooseFolder: (title?: string) => Promise<FolderChoice>;

  // Both scoped to this page's own workbench (windowKey, read once at module load — P8 D2/F6).
  tabsList: () => Promise<T[]>;
  tabsSave: (tabs: T[]) => Promise<void>;

  terminalDefaultCwd: () => Promise<{ path: string }>;
  terminalOpen: (
    terminalId: string,
    cwd: string,
    cols: number,
    rows: number,
    command?: string,
    launchKind?: TerminalLaunchKind,
    scriptId?: string,
    launchToken?: string,
  ) => Promise<{ shell: string }>;
  terminalWrite: (terminalId: string, data: string) => Promise<void>;
  terminalResize: (terminalId: string, cols: number, rows: number) => Promise<void>;
  terminalClose: (terminalId: string) => Promise<void>;
  onTerminal: (cb: (event: TerminalEvent) => void) => () => void;

  // P116 H5's own ten — see this file's header comment.
  onOpenSettings: (cb: () => void) => () => void;
  onCheckForUpdates: (cb: () => void) => () => void;
  onToggleProjectPanel: (cb: () => void) => () => void;
  onToggleOperationsPanel: (cb: () => void) => () => void;
  onTabNext: (cb: () => void) => () => void;
  onTabPrev: (cb: () => void) => () => void;
  onTabClose: (cb: () => void) => () => void;

  onAppMetrics: (cb: (sample: AppMetricsSample) => void) => () => void;

  windowsOpenNew: () => Promise<void>;

  // P22 D12/P128 §2.2: this window's own persisted module mode — the boot-time seam each app's own
  // state/mode.ts hydrateMode reads without a second round trip, and the debounced writer
  // state/mode.ts's setMode schedules.
  agentInfo: () => Promise<AgentInfo>;
  agentInfoFor: (id: string) => Promise<AgentInfo>;
  windowsEnsure: () => Promise<M>;
  windowsSetMode: (mode: M) => Promise<void>;

  keepAwakeStatus: () => Promise<KeepAwakeStatus>;
  keepAwakeSetManual: (enabled: boolean) => Promise<KeepAwakeStatus>;
  onKeepAwakeChanged: (cb: (status: KeepAwakeStatus) => void) => () => void;

  // P119's own three — see this file's header comment.
  updateStatus: () => Promise<AppUpdateStatus>;
  updateCheck: () => Promise<AppUpdateStatus>;
  updateInstall: () => Promise<void>;
  updateCancelInstall: () => Promise<void>;

  // P246: the prompt router's calls and pushes — see prompts/PromptHost.vue.
  promptsList: () => Promise<RoutedPrompt[]>;
  promptsClaim: (id: string) => Promise<void>;
  promptsRaiseUpdate: (ref: string) => Promise<void>;
  promptsDismiss: (id: string) => Promise<void>;
  promptsSendTest: () => Promise<void>;
  quitStatus: () => Promise<QuitStatus>;
  quitConfirm: () => Promise<void>;
  quitCancel: () => Promise<void>;
  onPromptsChanged: (cb: (prompts: RoutedPrompt[]) => void) => () => void;
  onPromptsReveal: (cb: (event: { id: string }) => void) => () => void;
  // The editable LLM prompt registry (internal/llmprompts) — see llmPrompts/queries.ts.
  llmPromptsList: () => Promise<LlmPromptList>;
  llmPromptsSave: (changes: LlmPromptChange[]) => Promise<LlmPromptList>;
  llmPromptsPreview: (args: LlmPromptPreviewArgs) => Promise<LlmPromptPreview>;
  onLlmPromptsChanged: (cb: (event: LlmPromptsChangedEvent) => void) => () => void;
}

export function createCoreControl<S, L, T, P, M>(b: CoreBindings): CoreControl<S, L, T, P, M> {
  return {
    settingsGetAll: (): Promise<S> => unwrap(b.settings.GetAll()).then((r) => trust<S>(r)),
    settingsSet: (patch: P): Promise<S> =>
      unwrap(b.settings.Set({ patch })).then((r) => trust<S>(r)),
    onSettingsChanged: (cb: (settings: S) => void): (() => void) => on(CHANNEL.settingsChanged, cb),

    layoutGetAll: (): Promise<L> => unwrap(b.layout.GetAll()).then((r) => trust<L>(r)),
    layoutSet: (patch: LayoutPatch): Promise<L> =>
      unwrap(b.layout.Set({ patch })).then((r) => trust<L>(r)),
    onLayoutChanged: (cb: (layout: L) => void): (() => void) => on(CHANNEL.layoutChanged, cb),

    onFlushBeforeClose: (cb: () => void): (() => void) => on(CHANNEL.appFlushBeforeClose, cb),
    appFlushed: (): void => {
      void b.lifecycle.Flushed({ windowKey });
    },
    onWindowFlushBeforeClose: (cb: () => void): (() => void) =>
      on(CHANNEL.windowFlushBeforeClose, cb),
    windowFlushed: (): void => {
      void b.lifecycle.WindowFlushed({ windowKey });
    },

    filesChooseFolder: (title?: string): Promise<FolderChoice> =>
      unwrap(b.files.ChooseFolder({ title: title ?? '' })).then((r) => trust<FolderChoice>(r)),

    tabsList: (): Promise<T[]> =>
      unwrap(b.tabs.List({ windowKey })).then((r) => trust<T[]>(r ?? [])),
    tabsSave: (tabs: T[]): Promise<void> => unwrap(b.tabs.Save({ windowKey, tabs })),

    terminalDefaultCwd: (): Promise<{ path: string }> =>
      unwrap(b.terminal.DefaultCwd()).then((r) => trust<{ path: string }>(r)),
    terminalOpen: (
      terminalId: string,
      cwd: string,
      cols: number,
      rows: number,
      command?: string,
      launchKind?: TerminalLaunchKind,
      scriptId?: string,
      launchToken?: string,
    ): Promise<{ shell: string }> =>
      unwrap(
        b.terminal.Open({
          terminalId,
          cwd,
          cols,
          rows,
          windowKey,
          command: command ?? '',
          launchKind: launchKind ?? 'shell',
          scriptId: scriptId ?? '',
          scriptLaunchToken: launchKind === 'agent' ? '' : (launchToken ?? ''),
          agentLaunchToken: launchKind === 'agent' ? (launchToken ?? '') : '',
        }),
      ).then((r) => trust<{ shell: string }>(r)),
    terminalWrite: (terminalId: string, data: string): Promise<void> =>
      unwrap(b.terminal.Write({ terminalId, data })),
    terminalResize: (terminalId: string, cols: number, rows: number): Promise<void> =>
      unwrap(b.terminal.Resize({ terminalId, cols, rows })),
    terminalClose: (terminalId: string): Promise<void> => unwrap(b.terminal.Close({ terminalId })),
    onTerminal: (cb: (event: TerminalEvent) => void): (() => void) => on(CHANNEL.terminal, cb),

    onOpenSettings: (cb: () => void): (() => void) => on(CHANNEL.openSettings, cb),
    onCheckForUpdates: (cb: () => void): (() => void) => on(CHANNEL.checkForUpdates, cb),
    onToggleProjectPanel: (cb: () => void): (() => void) => on(CHANNEL.toggleProjectPanel, cb),
    onToggleOperationsPanel: (cb: () => void): (() => void) =>
      on(CHANNEL.toggleOperationsPanel, cb),
    onTabNext: (cb: () => void): (() => void) => on(CHANNEL.tabNext, cb),
    onTabPrev: (cb: () => void): (() => void) => on(CHANNEL.tabPrev, cb),
    onTabClose: (cb: () => void): (() => void) => on(CHANNEL.tabClose, cb),

    onAppMetrics: (cb: (sample: AppMetricsSample) => void): (() => void) =>
      on(CHANNEL.appMetrics, cb),

    windowsOpenNew: (): Promise<void> => unwrap(b.windows.OpenNew()),

    agentInfo: (): Promise<AgentInfo> =>
      unwrap(b.agent.Info()).then((r) => agentInfoSchema.parse(r)),
    agentInfoFor: (id: string): Promise<AgentInfo> =>
      unwrap(b.agent.InfoFor(id)).then((r) => agentInfoSchema.parse(r)),
    windowsEnsure: (): Promise<M> =>
      unwrap(b.windows.Ensure({ windowKey })).then((r) =>
        trust<M>(trust<{ mode: string }>(r).mode),
      ),
    windowsSetMode: (mode: M): Promise<void> =>
      unwrap(b.windows.SetMode({ windowKey, mode: mode as string })),

    keepAwakeStatus: (): Promise<KeepAwakeStatus> =>
      unwrap(b.keepAwake.Status()).then((r) => trust<KeepAwakeStatus>(r)),
    keepAwakeSetManual: (enabled: boolean): Promise<KeepAwakeStatus> =>
      unwrap(b.keepAwake.SetManual({ enabled })).then((r) => trust<KeepAwakeStatus>(r)),
    onKeepAwakeChanged: (cb: (status: KeepAwakeStatus) => void): (() => void) =>
      on(CHANNEL.keepAwake, cb),

    updateStatus: (): Promise<AppUpdateStatus> =>
      unwrap(b.update.Status()).then((r) => trust<AppUpdateStatus>(r)),
    updateCheck: (): Promise<AppUpdateStatus> =>
      unwrap(b.update.Check()).then((r) => trust<AppUpdateStatus>(r)),
    updateInstall: (): Promise<void> => unwrap(b.update.InstallUpdate()),
    updateCancelInstall: (): Promise<void> => unwrap(b.update.CancelInstall()),

    promptsList: (): Promise<RoutedPrompt[]> =>
      unwrap(b.prompts.List()).then((r) => trust<RoutedPrompt[]>(r ?? [])),
    promptsClaim: (id: string): Promise<void> => unwrap(b.prompts.Claim({ id, windowKey })),
    promptsRaiseUpdate: (ref: string): Promise<void> =>
      unwrap(b.prompts.Raise({ kind: 'update', ref })),
    promptsDismiss: (id: string): Promise<void> => unwrap(b.prompts.Dismiss({ id })),
    promptsSendTest: (): Promise<void> => unwrap(b.prompts.SendTest()),
    quitStatus: (): Promise<QuitStatus> =>
      unwrap(b.lifecycle.QuitStatus()).then((r) => trust<QuitStatus>(r)),
    quitConfirm: (): Promise<void> => unwrap(b.lifecycle.ConfirmQuit()),
    quitCancel: (): Promise<void> => unwrap(b.lifecycle.CancelQuit()),
    onPromptsChanged: (cb: (prompts: RoutedPrompt[]) => void): (() => void) =>
      on(CHANNEL.promptsChanged, (p) => cb(trust<RoutedPrompt[]>(p ?? []))),
    onPromptsReveal: (cb: (event: { id: string }) => void): (() => void) =>
      on(CHANNEL.promptsReveal, (e) => cb(trust<{ id: string }>(e))),

    llmPromptsList: (): Promise<LlmPromptList> =>
      unwrap(b.llmPrompts.List()).then((r) => llmPromptListSchema.parse(r)),
    llmPromptsSave: (changes: LlmPromptChange[]): Promise<LlmPromptList> =>
      unwrap(b.llmPrompts.Save({ changes })).then((r) => llmPromptListSchema.parse(r)),
    llmPromptsPreview: ({ id, text, vars }: LlmPromptPreviewArgs): Promise<LlmPromptPreview> =>
      unwrap(b.llmPrompts.Preview({ id, text, vars })).then((r) => llmPromptPreviewSchema.parse(r)),
    onLlmPromptsChanged: (cb: (event: LlmPromptsChangedEvent) => void): (() => void) =>
      on(CHANNEL.llmPromptsChanged, (e) => cb(trust<LlmPromptsChangedEvent>(e))),
  };
}
