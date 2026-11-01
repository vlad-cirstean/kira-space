import type {
  ScriptRun,
  ScriptRunArgs,
  ScriptRunLogChunk,
  ScriptRunLogPage,
  ScriptRunPreview,
  ScriptRunRestoreChoice,
  ScriptRunRestoreResult,
  ScriptRunRestoreState,
  ScriptRunResumeLaunch,
  ScriptRunStarted,
} from '@shared/domain/scriptRuns';
import type {
  CustomScript,
  CustomScriptFields,
  ScriptCollection,
  ScriptDir,
  ScriptMcpServer,
  SecretValues,
} from '@shared/domain/scripts';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import type { ScriptRunTabState } from '@shared/domain/tabs';
import { type ComputedRef, computed, type InjectionKey, inject } from 'vue';
import type { TerminalLaunch } from '../state/createTerminalTabs';
import type { TerminalHostDeps } from '../terminal/terminalHost';

// P128 §2.4: the shared automations module's own injected context — AutomationsPanel.vue/
// AutomationsStart.vue/AutomationsNewTab.vue read this instead of an app-specific store, so the same
// three components mount in both apps. `openTerminalTab` binds the app's own module workspace
// ('terminal' in both) itself, so shared code never names a workspace key.

/** Each app's script store (createCustomScriptsStore.ts), as the panel and its dialog see it. */
export interface ScriptsSeam {
  records(): readonly CustomScript[];
  /** `secretValues` is write-only: a string saves, null clears, an absent name keeps. */
  create(fields: CustomScriptFields, secretValues?: SecretValues | null): Promise<unknown>;
  update(
    id: string,
    fields: CustomScriptFields,
    secretValues?: SecretValues | null,
  ): Promise<unknown>;
  secretStorage(): SecretStorageStatus;
  remove(id: string): Promise<void>;
  collections(): readonly ScriptCollection[];
  createCollection(name: string): Promise<ScriptCollection>;
  renameCollection(id: string, name: string): Promise<void>;
  removeCollection(id: string): Promise<void>;
  /** collectionId null ungroups. */
  /** `beforeId` places the script before that sibling of the target set; null appends. */
  move(id: string, collectionId: string | null, beforeId?: string | null): Promise<void>;
  /** Persists one set's order: the ungrouped scripts (null) or one collection's. */
  reorder(collectionId: string | null, ids: string[]): Promise<void>;
  reorderCollections(ids: string[]): Promise<void>;
}

/** The run store's bound calls and push channel (ScriptRunsService), the same in both apps. */
export interface ScriptRunsSeam {
  /** A task id narrows to that task's runs (Kira Space). */
  list(limit?: number, taskId?: string): Promise<ScriptRun[]>;
  stop(id: string): Promise<void>;
  /** Where a saved script runs; an empty id answers the app home as `base`, for an unsaved script. */
  resolveDir(scriptId: string): Promise<ScriptDir>;
  onChanged(cb: (run: ScriptRun) => void): () => void;
  /** What a run would do, exactly as `start` does it. */
  preview(args: ScriptRunArgs): Promise<ScriptRunPreview>;
  /** `hash` is the preview's; a script edited since answers E_CONFLICT. */
  start(args: ScriptRunArgs, hash: string): Promise<ScriptRunStarted>;
  /** Continues the Claude session of a smart run a restart ended, as a new run. */
  resume(id: string): Promise<ScriptRunStarted>;
  /** The terminal launch that continues a smart run's conversation. */
  resumeCommand(runId: string): Promise<ScriptRunResumeLaunch>;
  /** The runs the last quit or crash ended, each planned against its script as it is now. */
  restoreState(): Promise<ScriptRunRestoreState>;
  /** Starts the confirmed runs; one outcome each, a failed run stays on the list. */
  restore(items: ScriptRunRestoreChoice[]): Promise<ScriptRunRestoreResult>;
  /** Forgets the list. */
  restoreDismiss(): Promise<void>;
  readLog(id: string, afterSeq: number): Promise<ScriptRunLogPage>;
  onLog(cb: (push: { runId: string; chunks: ScriptRunLogChunk[] }) => void): () => void;
  /** Next fire instants (unix ms) of a cron in a timezone; rejects with the Go validation text. */
  nextFires(cron: string, timezone: string, count?: number): Promise<number[]>;
  /** The run a script's schedule would start; `secrets` fills secret params the schedule never stores. */
  schedulePreview(scriptId: string, secrets: Record<string, string[]>): Promise<ScriptRunPreview>;
  runScheduleNow(
    scriptId: string,
    hash: string,
    secrets: Record<string, string[]>,
  ): Promise<ScriptRunStarted>;
  /** Starts a waiting scheduled run; `hash` is the schedule preview's. */
  confirmAccept(
    runId: string,
    hash: string,
    secrets: Record<string, string[]>,
  ): Promise<ScriptRunStarted>;
  confirmDecline(runId: string): Promise<void>;
  /** MCP servers claude reports. */
  mcpServers(): Promise<ScriptMcpServer[]>;
}

export interface AutomationsModuleContext {
  defaultCwd(): string;
  /** `replace` names a tab the new one takes the strip slot of. */
  openTerminalTab(opts: { cwd: string; launch?: TerminalLaunch; replace?: string }): void;
  /** Opens or focuses the tab of one script run by run id; `replace` names a tab it takes over. */
  openRunTab(state: ScriptRunTabState, replace?: string): void;
  /** The automations workspace's tabs, in strip order. */
  tabs: {
    list(): readonly { id: string; kind: string; state: unknown }[];
    activate(id: string): void;
  };
  host: TerminalHostDeps;
  scripts: ScriptsSeam;
  runs: ScriptRunsSeam;
  /** Focuses the Automations module, e.g. from the status bar. */
  showAutomations(): void;
  /** Opens the Operations dock, where finished runs are listed. */
  showOperations(): void;
  /** Kira Space: scripts can run for an ADE task. */
  ade: boolean;
  /** Native folder dialog; null when cancelled. */
  chooseFolder(title: string): Promise<string | null>;
}

export const automationsModuleKey: InjectionKey<AutomationsModuleContext> =
  Symbol('automationsModule');

export function useAutomationsModule(): AutomationsModuleContext {
  const ctx = inject(automationsModuleKey);
  if (!ctx) throw new Error('useAutomationsModule: no AutomationsModuleContext provided');
  return ctx;
}

/** The one `cwd === ''` guard AutomationsStart.vue and the tab strip's own "+" menu both used —
 *  disabled while the resolved default cwd (§7.2) isn't known yet. */
export function useNewTerminal(): { canOpen: ComputedRef<boolean>; open(): void } {
  const ctx = useAutomationsModule();
  const canOpen = computed(() => ctx.defaultCwd() !== '');
  function open(): void {
    if (!canOpen.value) return;
    ctx.openTerminalTab({ cwd: ctx.defaultCwd() });
  }
  return { canOpen, open };
}
