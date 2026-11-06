import { defineStore } from 'pinia';
import { reactive } from 'vue';

// Postgres-only, renderer-only, session-only — never persisted, never sent to the engine.
// A console opened at a bare connection root (path === '') has no session-level way to change
// which database it targets from inside the script itself (unlike MariaDB's own `USE db;`), so
// "Set as default" on a database/schema row remembers the row's own encoded path here and
// openConsoleTab() substitutes it in when a console is opened at the root.
export const useConsoleDefaultsStore = defineStore('consoleDefaults', () => {
  const defaults = reactive({} as Record<string, string>);

  function setConsoleDefault(connectionId: string, path: string): void {
    defaults[connectionId] = path;
  }

  function consoleDefaultFor(connectionId: string): string | null {
    return defaults[connectionId] ?? null;
  }

  return { setConsoleDefault, consoleDefaultFor };
});
