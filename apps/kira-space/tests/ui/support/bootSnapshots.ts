import { defaultLayout } from '@shared/domain/layout';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { defaultSettings as defaultStudioSettings } from '../../../frontend/src/studio/state/settingsDomain';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

const KEYCHAIN_AVAILABLE: SecretStorageStatus = {
  available: true,
  backend: 'keychain',
  insecureFallback: false,
  reason: null,
};

/**
 * The `Promise.all` `apps/kira-space/frontend/src/main.ts`'s `bootstrap()` fires before `mount()`
 * — `layoutGetAll`/`settingsGetAll`/`codeWorkspaceListRepos`/`tabsList` —
 * answered with an empty, healthy app: defaults, no repositories imported, no tabs. This is what
 * every `tests/ui/` spec's `relaunch()` starts from — Kira Studio's own
 * EMPTY_BOOT_SNAPSHOTS, trimmed to this app's own bootstrap() (no connections/masked
 * columns/ops — none of that exists here).
 *
 * `gitCredentialPending`/`terminalDefaultCwd`/`windowsEnsure` are deliberately
 * absent — every one of them already has a `WILDCARD_DEFAULTS` entry in mockRuntime.ts (nothing
 * here ever needs to override them per-spec the way `tabsList` regularly does), the same "boot
 * call with no committed fixture will ever snapshot it" reasoning as Kira Studio's own array.
 * A spec that DOES care which mode boot lands in (modules.spec.ts) still wins by
 * providing its own `windowsEnsure` snapshot straight to `relaunch()`, same as any other channel.
 */
export const EMPTY_BOOT_SNAPSHOTS: readonly ControlSnapshot[] = [
  { channel: IPC.layoutGetAll, response: defaultLayout },
  { channel: IPC.settingsGetAll, response: defaultSettings },
  { channel: IPC.codeWorkspaceListRepos, response: [] },
  { channel: IPC.tabsList, response: [] },
  {
    channel: IPC.customScriptsList,
    response: {
      dir: '/home/test/runs',
      collections: [],
      scripts: [],
      problems: [],
      secretStorage: {
        available: true,
        backend: 'keychain',
        insecureFallback: false,
        reason: null,
      },
    },
  },
  { channel: IPC.scriptRunsList, response: [] },
  { channel: IPC.promptsList, response: [] },
  // The databases module's bootstrap(): its own settings, connections, mask-rule counts and ops.
  { channel: IPC.studioSettingsGetAll, response: defaultStudioSettings },
  { channel: IPC.connectionsList, response: [] },
  { channel: IPC.connectionsStates, response: [] },
  { channel: IPC.connectionsSecretsStatus, response: KEYCHAIN_AVAILABLE },
  { channel: IPC.maskRulesCounts, response: {} },
  { channel: IPC.studioOpsRecent, response: [] },
];

/** A spec's own snapshot for a channel replaces the default outright, rather than being appended
 *  after it — mockRuntime.ts answers a channel's snapshots in fixture order, so appending would
 *  make the boot-time call consume the (still-empty) default and never reach the spec's data. */
export function mergeBootSnapshots(overrides: readonly ControlSnapshot[]): ControlSnapshot[] {
  const overridden = new Set(overrides.map((s) => s.channel));
  return [...EMPTY_BOOT_SNAPSHOTS.filter((s) => !overridden.has(s.channel)), ...overrides];
}
