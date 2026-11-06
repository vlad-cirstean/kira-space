import type { Page } from '@playwright/test';
import type { ProofViewStateEntry } from '@shared/domain/proof';
import type { ControlSnapshot } from '../../../ipc/support/types';
import { contract } from './contract';
import { IPC } from './ipcChannels';

// Snapshots for the proof window (tests/contract/proof.json and proof-pending.json, written by the
// Go flow tests) with the ids filled in. The window boots from `?window=proof-1`.

export const SESSION_ID = 'session-1';
export const PENDING_ID = 'session-2';

// biome-ignore lint/suspicious/noExplicitAny: Go-written fixtures have no static shape
type Json = Record<string, any>;

const fixture = (name: 'proof' | 'proof-pending' | 'proof-chart', key: string): Json =>
  contract(name, key) as Json;

/** A fresh folder from ChooseShotFolder (tests/contract/proof.json) with its masked fields filled. */
export const shotFolder = (): Json => ({
  ...fixture('proof', 'ProofService.ChooseShotFolder'),
  id: 'shot_test',
  path: '/tmp/test/Checkout fix',
});

/** Height in pixels from a PNG's IHDR chunk (bytes 20-24), given its base64. */
export function pngHeight(pngBase64: string): number {
  const bytes = Buffer.from(pngBase64.slice(0, 44), 'base64');
  return bytes.readUInt32BE(20);
}

export const CLOSED_AT = Date.UTC(2026, 0, 2, 3, 4, 5);

export const session = (): Json => ({
  ...fixture('proof', 'ProofService.Session'),
  id: SESSION_ID,
  closedAt: CLOSED_AT,
});
export const pendingSession = (): Json => ({
  ...fixture('proof-pending', 'ProofService.Session'),
  id: PENDING_ID,
  closedAt: CLOSED_AT,
});
export const sessions = (): Json[] =>
  (fixture('proof', 'ProofService.Sessions') as unknown as Json[]).map((s) => ({
    ...s,
    id: SESSION_ID,
    closedAt: CLOSED_AT,
  }));

/** A wide, tall table so both scroll axes have somewhere to go. */
export function bigTable(rows = 160, cols = 12): Json {
  return {
    columns: Array.from({ length: cols }, (_, c) => ({
      name: c === 0 ? 'id' : `column_${c}`,
      type: c === 0 ? 'number' : 'text',
    })),
    rows: Array.from({ length: rows }, (_, r) =>
      Array.from({ length: cols }, (_, c) => (c === 0 ? r + 1 : `value ${r + 1}.${c}`)),
    ),
  };
}

/** A query tall enough that the query box scrolls. */
export const LONG_QUERY = Array.from(
  { length: 40 },
  (_, i) => `select ${i + 1} as line_${i + 1}${i < 39 ? ',' : ''}`,
).join('\n');

export function stepSnapshot(
  sessionId: string,
  rev: number,
  ordinal: number,
  base: 'table' | 'document' | 'keyvalue',
  patch: Json = {},
): ControlSnapshot {
  return {
    channel: IPC.proofStep,
    args: { sessionId, rev, ordinal },
    response: { ...fixture('proof', `ProofService.Step#${base}`), ordinal, ...patch },
  };
}

export const chartSession = (): Json => ({
  ...fixture('proof-chart', 'ProofService.Session'),
  id: SESSION_ID,
  closedAt: CLOSED_AT,
});
export const chartPendingSession = (): Json => ({
  ...fixture('proof-chart', 'ProofService.Session#pending'),
  id: PENDING_ID,
  closedAt: CLOSED_AT,
});
export const chartViewStates = (): ProofViewStateEntry[] =>
  fixture('proof-chart', 'ProofService.ViewState#chart') as unknown as ProofViewStateEntry[];

export type ChartKey = 'line' | 'bar' | 'histogram' | 'area' | 'scatter' | 'pie' | 'bar-new';

/** A chart step from the Go-written contract; `patch` overrides any field of the step. */
export function chartStep(
  sessionId: string,
  rev: number,
  ordinal: number,
  key: ChartKey,
  patch: Json = {},
): ControlSnapshot {
  return {
    channel: IPC.proofStep,
    args: { sessionId, rev, ordinal },
    response: { ...fixture('proof-chart', `ProofService.Step#${key}`), ordinal, ...patch },
  };
}

export const CHART_KEYS: ChartKey[] = ['line', 'bar', 'histogram', 'area', 'scatter', 'pie'];

/** Boot snapshots for the chart session; steps 1-6 in the contract's order. */
export function chartControl(
  extra: ControlSnapshot[] = [],
  patches: Record<number, Json> = {},
): ControlSnapshot[] {
  return [
    { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
    {
      channel: IPC.proofSessions,
      response: (fixture('proof-chart', 'ProofService.Sessions') as unknown as Json[]).map((x) => ({
        ...x,
        id: SESSION_ID,
        closedAt: CLOSED_AT,
      })),
    },
    { channel: IPC.proofSession, response: chartSession() },
    ...CHART_KEYS.map((key, i) => chartStep(SESSION_ID, 1, i + 1, key, patches[i + 1])),
    ...extra,
  ];
}

export function proofControl(extra: ControlSnapshot[] = []): ControlSnapshot[] {
  return [
    { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
    { channel: IPC.proofSessions, response: sessions() },
    { channel: IPC.proofSession, response: session() },
    ...extra,
  ];
}

export function viewState(
  ordinal: number,
  state: Partial<ProofViewStateEntry['state']> = {},
): ProofViewStateEntry {
  return {
    ordinal,
    state: {
      queryScrollTop: 0,
      resultScrollTop: 0,
      resultScrollLeft: 0,
      selection: null,
      selectedRow: null,
      expanded: [],
      columnWidths: {},
      ...state,
    },
  };
}

export async function openProofWindow(page: Page, baseUrl: string, key = 'proof-1'): Promise<void> {
  await page.goto(`${baseUrl}?window=${key}`);
  await page.waitForSelector('[data-testid="proof-window"]');
}

/** The grid's scroll container inside a proof step. */
export const RESULT_VIEWPORT =
  '[data-testid="proof-step-result"] .slick-viewport-top.slick-viewport-right';

/** Mean and over-threshold share of per-channel differences between a copied PNG and a Playwright
 *  screenshot of the same element. */
export async function diffClipboardImage(
  page: Page,
  shot: Buffer,
  index = 0,
): Promise<{ width: number; height: number; mean: number; over: number }> {
  return page.evaluate(
    async ({ b64, index }) => {
      const clip = (window as unknown as { __clipboardImages: Blob[] }).__clipboardImages[index];
      if (!clip) throw new Error('no copied image');
      const decode = async (blob: Blob): Promise<ImageData> => {
        const bmp = await createImageBitmap(blob);
        const c = document.createElement('canvas');
        c.width = bmp.width;
        c.height = bmp.height;
        const ctx = c.getContext('2d');
        if (!ctx) throw new Error('no 2d context');
        ctx.drawImage(bmp, 0, 0);
        return ctx.getImageData(0, 0, bmp.width, bmp.height);
      };
      const a = await decode(clip);
      const raw = atob(b64);
      const bytes = Uint8Array.from(raw, (ch) => ch.charCodeAt(0));
      const b = await decode(new Blob([bytes], { type: 'image/png' }));
      const width = Math.min(a.width, b.width);
      const height = Math.min(a.height, b.height);
      let sum = 0;
      let over = 0;
      for (let y = 0; y < height; y++) {
        for (let x = 0; x < width; x++) {
          const i = (y * a.width + x) * 4;
          const j = (y * b.width + x) * 4;
          let d = 0;
          for (let k = 0; k < 3; k++)
            d = Math.max(d, Math.abs((a.data[i + k] ?? 0) - (b.data[j + k] ?? 0)));
          sum += d;
          if (d > 48) over++;
        }
      }
      return { width, height, mean: sum / (width * height), over: over / (width * height) };
    },
    { b64: shot.toString('base64'), index },
  );
}
