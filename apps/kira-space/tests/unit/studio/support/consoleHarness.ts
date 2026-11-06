import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../../frontend/src/state/pinia';

/** Shared pinia/bridge/store bootstrap for the console-*.spec.ts run()/stop() race tests (P12
 *  rounds 1-2) — each spec drives ConsoleViewStore end to end against real store wiring. Pass
 *  `control: true` for the specs that also stub the control bridge (opsCancel). */
export async function bootstrapConsole(opts: { control?: boolean } = {}) {
  setActivePinia(pinia);

  const { data } = await import('../../../../frontend/src/studio/bridge/data');
  restoreAfterEach(data);

  // createTabsStore's saveIfChanged now serialises every save through one
  // persistent chain (enqueueSave) instead of firing an independent, uninspected promise per call —
  // correct for real IPC, which always eventually settles, but every console-*.spec.ts here shares
  // ONE tabsStore singleton across the whole tests/unit run (Pinia + this module's own shared
  // `pinia`), and opening a console tab below fires a real, un-mocked control.tabsSave whose
  // generic Call() stub (wailsRuntime.ts) deliberately never settles for a call nobody here awaits.
  // Before this fix, that was harmless — an independent, forgotten promise. Now it would
  // permanently wedge the shared chain, starving every later spec's own tabsSave assertions for the
  // rest of the run.
  // The benign override is set *before* restoreAfterEach snapshots control, not in beforeEach: the
  // enqueued save this triggers can settle on a tick past this file's own afterEach (confirmed via
  // stream-count-honors-filter.spec.ts's identical hazard — only reproduces with ~90+ other spec
  // files loaded, never in isolation), so a beforeEach/afterEach-scoped override races the chain
  // back onto the hanging default and wedges it permanently for every later spec in the process —
  // worse than the leak restoreAfterEach's own comment warns about, since that leak is confined to
  // one spec file and this one is permanent for the rest of the run.
  const controlMod = await import('../../../../frontend/src/studio/bridge/control');
  (controlMod.control as unknown as { tabsSave: () => Promise<void> }).tabsSave = () =>
    Promise.resolve();
  restoreAfterEach(controlMod.control);

  let control: typeof import('../../../../frontend/src/studio/bridge/control').control | undefined;
  if (opts.control) {
    control = controlMod.control;
  }

  const { useConnectionsStore } = await import('../../../../frontend/src/studio/state/connections');
  const connectionsStore = useConnectionsStore();
  const { useTabsStore } = await import('../../../../frontend/src/state/tabs');
  const tabsStore = useTabsStore();
  const { useConsoleViewStore } = await import(
    '../../../../frontend/src/studio/views/console/state'
  );
  const consoleViewStore = useConsoleViewStore();

  return { data, control, connectionsStore, tabsStore, consoleViewStore };
}
