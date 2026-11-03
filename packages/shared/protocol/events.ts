import type { MaskRule } from '../domain/mask';
import type { VariableScope } from '../domain/variables';

/** The Go→renderer push channels (`apps/kira-studio/internal/bridge/events.go`'s constants, verbatim).
 *  Formerly the push half of ipc.ts's IPC const; the request/response half retired with
 *  window.kira, and the wire types for bound calls now come from the generated bindings (P57 D7).
 *  `kira:engine:state` and `kira:port` are absent — both are dead channels retired by D6/D3. */
export const CHANNEL = {
  openSettings: 'kira:menu:openSettings',
  checkForUpdates: 'kira:menu:checkForUpdates',
  newConnection: 'kira:menu:newConnection',
  newRequest: 'kira:menu:newRequest',
  importPostman: 'kira:menu:importPostman',
  importDataGrip: 'kira:menu:importDataGrip',
  toggleProjectPanel: 'kira:menu:toggleProjectPanel',
  toggleOperationsPanel: 'kira:menu:toggleOperationsPanel',
  commandPalette: 'kira:menu:commandPalette',
  tabNext: 'kira:menu:tabNext',
  tabPrev: 'kira:menu:tabPrev',
  tabClose: 'kira:menu:tabClose',
  viewFind: 'kira:menu:viewFind',
  viewRefresh: 'kira:menu:viewRefresh',
  viewRun: 'kira:menu:viewRun',
  viewRunAll: 'kira:menu:viewRunAll',
  viewFormat: 'kira:menu:viewFormat',
  // Space only: the Review Code menu item (ade.reviewCode).
  reviewCode: 'kira:menu:reviewCode',
  appFlushBeforeClose: 'kira:app:flushBeforeClose',
  windowFlushBeforeClose: 'kira:window:flushBeforeClose',
  connectionState: 'kira:connection:state',
  connectionMetadataInvalidated: 'kira:connection:metadataInvalidated',
  connectionsChanged: 'kira:connections:changed',
  settingsChanged: 'kira:settings:changed',
  layoutChanged: 'kira:layout:changed',
  opUpdate: 'kira:op:update',
  opBatch: 'kira:op:batch',
  appMetrics: 'kira:app:metrics',
  schemaChanged: 'kira:schema:changed',
  // P11 D8: a server-streaming call's coalesced message batches, delivered via EmitTo (one window
  // only) — the one genuinely new push channel this phase adds.
  grpcCall: 'kira:grpc:call',
  // P178: every pending credential prompt raised by the git stream or the ADE board.
  gitCredential: 'kira:git:credential',
  // P212: the phone pairing prompt's queue snapshot, the paired phones list and the server state
  // (Emit to every window).
  mobilePairing: 'kira:mobile:pairing',
  mobileDevices: 'kira:mobile:devices',
  mobileStatus: 'kira:mobile:status',
  // P212 Part 2: a phone-started launch for one window to open (EmitTo), and the terminals a phone
  // currently holds (Emit).
  mobileOpenLaunch: 'kira:mobile:openLaunch',
  mobileTerminals: 'kira:mobile:terminals',
  // C7 D7: a repository-wide search's coalesced file groups, delivered via EmitTo (one window
  // only) — grpcCall's own shape, restated for a payload that shares no field with it.
  codeSearch: 'kira:code:search',
  // M2 §7.1: the prompt-mode approval queue's live snapshot.
  dbMcpApproval: 'kira:dbmcp:approval',
  // P83 §3.2: one terminal's coalesced output and its exit, delivered via EmitTo (one window
  // only) — codeSearch's own shape, restated for a byte payload.
  terminal: 'kira:terminal:data',
  // P85 §9.3: the custom-scripts list changed — connectionsChanged's own shape (Emit, not EmitTo,
  // so every window's tab-strip dropdown stays in sync).
  customScriptsChanged: 'kira:customScripts:changed',
  // P242: one Automations run was created or changed (Emit, app-wide) — carries the whole run.
  scriptRunsChanged: 'kira:scriptRuns:changed',
  // P242: log lines a smart run just stored, {runId, chunks} (Emit, app-wide).
  scriptRunLog: 'kira:scriptRunLog:appended',
  // P246: the routed popup list changed (Emit, app-wide) — carries every open prompt with its target.
  promptsChanged: 'kira:prompts:changed',
  // P246: a notification click asks one window to show a prompt (EmitTo), {id}.
  promptsReveal: 'kira:prompts:reveal',
  // The LLM prompt overrides file changed (Emit, app-wide), {revision}.
  llmPromptsChanged: 'kira:llmPrompts:changed',
  // P108 Part 12 F18: one connection's mask-rule set changed (Upsert/Remove/RegenerateKey) —
  // schemaChanged's own per-connection shape (Emit, not EmitTo), so a second window's Privacy tab,
  // grid header menu and grid preview all stay in sync rather than reading a stale rule list.
  maskRulesChanged: 'kira:maskRules:changed',
  // P433 F4: a saved query was written (Save/Update/Delete/Touch), Emit'd app-wide, {connectionId, kind}.
  savedQueriesChanged: 'kira:savedQueries:changed',
  // P433 F4: one filter-history row was recorded (Emit, app-wide), {connectionId, path}.
  filterHistoryChanged: 'kira:filterHistory:changed',
  // P86 §11: every live Claude Code session across every window, Emit'd (not EmitTo) whenever
  // terminal.Registry.OnChange fires — customScriptsChanged's own shape, app-wide by definition.
  agentSessions: 'kira:agent:sessions',
  // P86 §8.4: one Claude Code hook firing for one tab, Emit'd (not EmitTo) — a hook event has no
  // window to address, so every window's own store filters by terminalId against tabs it owns.
  agentEvent: 'kira:agent:event',
  // P87 §3.2: the titlebar keep-awake toggle's own state, Emit'd (not EmitTo) exactly like
  // agentSessions — one machine, one assertion, so every window's button must agree.
  keepAwake: 'kira:keepAwake:changed',
  // P201: memory.db changed (this app's writes or the kira-memory MCP server's), Emit'd app-wide.
  memoryChanged: 'kira:memory:changed',
  // P210: semantic-search status or indexing progress changed; empty payload, refetch the status.
  memorySemantic: 'kira:memory:semantic',
  // P211: an import job or file changed state or progress; empty payload, refetch.
  memoryImport: 'kira:memory:import',
  // P285 Part 4: a running distill's progress; payload {requestId, stage, done, total, costUsd}.
  memoryDistill: 'kira:memory:distill',
  // P112: every API-client mutation (collections, saved requests, variables, environments) Emits
  // this with the scopes it touched, so every window's TanStack Query cache invalidates exactly
  // those keys — customScriptsChanged's own shape, generalised to a batch of scopes per event.
  apiDataChanged: 'kira:api:dataChanged',
  // P143: ADE v2 channels (`apps/kira-space/internal/bridge/adewire/channels.go`, verbatim).
  // Payload types live in apps/kira-space/frontend/src/ade/v2/wire.ts.
  adeTaskBoard: 'kira:adetask:board',
  adeTaskWorkflows: 'kira:adetask:workflows',
  // The shared agent context file changed or was saved; empty payload, refetch (Kira Space only).
  agentContextChanged: 'kira:agentContext:changed',
  agentSettingsChanged: 'kira:agentSettings:changed',
  // A repo settings file changed or was saved, or the set of file errors changed; empty payload (Kira Space only).
  repoConfigChanged: 'kira:repoConfig:changed',
  adeTaskRepos: 'kira:adetask:repos',
  adeTaskRuns: 'kira:adetask:runs',
  adeTaskLog: 'kira:adetask:log',
  adeTaskSessions: 'kira:adetask:sessions',
  adeTaskOpenSession: 'kira:adetask:openSession',
  adeTaskReview: 'kira:adetask:review',
  // P420: validation-proof sessions changed (Emit, app-wide), {sessionId}; a request for the proof
  // window to show one session (EmitTo), {sessionId}.
  proofChanged: 'kira:proof:changed',
  proofSelect: 'kira:proof:select',
} as const;

/** Summed across every process metrics.Sample covers — a single app-wide readout
 *  for the status bar, not a per-process breakdown. No generated binding carries this shape: it
 *  is emitted (`kira:app:metrics`), never returned from a bound call, so it has no home in any
 *  service's models.ts and stays hand-written here instead. Mirrors
 *  `apps/kira-studio/internal/metrics.Sample`'s JSON tags.
 *
 *  cpuPercent is normalized to the machine's whole capacity (0-100), not the per-core-sum macOS's
 *  own Activity Monitor shows in its per-process "% CPU" column (which reads up to `logicalCPUs`
 *  times higher for the same load) — StatusBar.vue's tooltip states this explicitly since it's the
 *  exact cross-check a user is likely to make. memoryBytes is RSS everywhere except darwin, where
 *  it is phys_footprint — Activity Monitor's own "Memory" column, not its "Real Memory" one.
 *  logicalCPUs and processCount exist so the tooltip can say what cpuPercent is a
 *  percentage *of* and how many processes memoryBytes covers. */
export interface AppMetricsSample {
  cpuPercent: number;
  memoryBytes: number;
  logicalCPUs: number;
  processCount: number;
}

// P112: `kira:api:dataChanged`'s own payload (internal/bridge/apidata.go's ApiDataChange/
// ApiDataChanged, field for field). Never a bound-call arg or return type, so — AppMetricsSample's
// own comment applies verbatim — there is no generated binding to reuse.
export type ApiDataChange =
  | { kind: 'tree' }
  | { kind: 'savedRequest'; itemId: string }
  | { kind: 'variables'; scope: VariableScope; ownerId: string }
  | { kind: 'environments' }
  | { kind: 'config' };

export interface ApiDataChangedEvent {
  changes: ApiDataChange[];
}

/** `kira:terminal:data`'s own payload (internal/bridge/terminal.go's TerminalEvent, field for
 *  field) — one coalesced chunk of a session's output, or its exit. Never a bound-call
 *  arg or return type, so it has no generated binding — a plain interface here, mirroring
 *  AppMetricsSample just above: this event crosses no storage boundary and is never restored, so
 *  there is nothing for a zod schema to guard that this interface doesn't already state. */
export interface TerminalEvent {
  terminalId: string;
  data?: string; // base64, absent on the exit event
  exited: boolean;
  exitCode?: number;
  error?: string;
}

/** `kira:savedQueries:changed`'s payload (internal/bridge/queries.go's SavedQueriesChangedEvent). */
export interface SavedQueriesChangedEvent {
  connectionId: string;
  kind: string;
}

/** `kira:filterHistory:changed`'s payload (internal/bridge/queries.go's FilterHistoryChangedEvent). */
export interface FilterHistoryChangedEvent {
  connectionId: string;
  path: string;
}

/** `kira:maskRules:changed`'s own payload (internal/bridge/maskrules.go's MaskRulesChangedEvent,
 *  field for field) — one connection's own rule set, sent whole on every Upsert/Remove/
 *  RegenerateKey. `keyRegenerated` is set only by RegenerateKey — the one case
 *  state/maskRules.ts's own applyRemote must also drop that connection's cached correlation key. */
export interface MaskRulesChangedEvent {
  connectionId: string;
  rules: MaskRule[];
  keyRegenerated: boolean;
}
