import type { Locator, Page } from '@playwright/test';
import type { ContractJson } from '@workbench/testing/ui/contract';
import { boxCenter } from '@workbench/testing/ui/drag';
import { calls } from '../../support/ipcCalls';
import { IPC } from '../../support/ipcChannels';

// Shared by the tab move specs (tab-window-move, tab-multi-select).

export function httpTab(id: string, order: number, active = false) {
  return {
    id,
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order,
    active,
    state: {
      method: 'GET',
      url: `https://api.example.com/${id}`,
      headers: [],
      bodyMode: 'none',
      body: '',
      requestPane: 'params',
      responsePane: 'history',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
}

// A restored data tab holds no pending edits, so it moves without the discard question.
export function dataTab(id: string, order: number, active = false) {
  return {
    id,
    connectionId: 'conn-x',
    path: `database:d/schema:s/table:${id}`,
    kind: 'data',
    order,
    active,
    state: {},
  };
}

type Control = Parameters<typeof calls>[0];

export const sentMoves = (control: Control) =>
  calls(control, IPC.tabsMove).map((e) => e.args as ContractJson);

export function lastSave(control: Control): string[] {
  const saves = calls(control, IPC.tabsSave);
  const last = saves[saves.length - 1]?.args as { tabs: { id: string }[] } | undefined;
  return (last?.tabs ?? []).map((t) => t.id);
}

/** Presses the chip and drags it to x = -60, outside the viewport, without releasing. */
export async function holdOutside(page: Page, chip: Locator): Promise<void> {
  const box = await chip.boundingBox();
  if (!box) throw new Error('chip has no box');
  const from = boxCenter(box);
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 10, from.y, { steps: 5 });
  await page.mouse.move(-60, 200, { steps: 15 });
}
