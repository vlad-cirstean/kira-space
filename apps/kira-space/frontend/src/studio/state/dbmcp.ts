import type { DbMcpInstallResult, DbMcpStatus } from '@shared/domain/dbmcp';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { useSettingsStore } from '../../state/settings';
import { control } from '../bridge/control';

// M1 §6.2: the Database MCP section's own store — a status-plus-last-action shape, hydrated at
// boot (main.ts).
const DEFAULT_STATUS: DbMcpStatus = {
  running: false,
  command: '',
  agentAvailable: false,
  probed: [],
  expiresAt: '',
  error: '',
};

export const useDbMcpStore = defineStore('dbMcp', () => {
  const state = reactive({
    status: DEFAULT_STATUS as DbMcpStatus,
    installResult: null as DbMcpInstallResult | null,
  });

  async function hydrateDbMcp(): Promise<void> {
    state.status = await control.dbMcpStatus();
  }

  // The toggle applies immediately, bypassing the dialog's draft/Save flow entirely,
  // since SetEnabled both persists the leaf and starts/stops the embedded instance in one call.
  // settingsStore.dbMcp.serverEnabled is set directly from the
  // confirmed `enabled` argument rather than waited-for through the separate kira:settings:changed
  // broadcast SetEnabled also emits server-side — this window's own toggle must never lag its own
  // click, and a later broadcast arrival (this window's own echo, or another window's) reapplies
  // the identical value, a harmless no-op.
  async function setDbMcpEnabled(enabled: boolean): Promise<void> {
    state.status = await control.dbMcpSetEnabled(enabled);
    useSettingsStore().dbMcp.serverEnabled = enabled;
    // A fresh enable/disable makes any previous install outcome stale.
    state.installResult = null;
  }

  // Covers the app-restart-with-no-plaintext-held case — the Database MCP section shows this action
  // in place of the command whenever status.running is true but status.command is empty.
  async function regenerateDbMcpToken(): Promise<void> {
    state.status = await control.dbMcpRegenerate();
  }

  async function installDbMcpAgent(): Promise<void> {
    state.installResult = await control.dbMcpInstallAgent();
  }

  return {
    ...toRefs(state),
    hydrateDbMcp,
    setDbMcpEnabled,
    regenerateDbMcpToken,
    installDbMcpAgent,
  };
});
