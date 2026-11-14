import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';

// state/tabs.ts saves through `bridge/control`, a different object from `studio/bridge/control`.
// Its un-mocked tabsSave never settles in this harness and would wedge the shared save chain for
// every later spec's own tabsSave assertions. Set before restoreAfterEach snapshots, so the
// benign default is what gets restored.
export async function benignTabsSave(): Promise<void> {
  const { control } = await import('../../../../frontend/src/bridge/control');
  (control as unknown as { tabsSave: () => Promise<void> }).tabsSave = () => Promise.resolve();
  restoreAfterEach(control);
}
