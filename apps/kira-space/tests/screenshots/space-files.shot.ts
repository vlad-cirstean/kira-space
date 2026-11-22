import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { expect, taskFilesWindowTest as test } from '../ui/fixtures';
import { taskFilesControl } from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { STORY, storyBoard } from './support/buildStory';
import { shoot } from './support/shoot';

// P734: the task Files window with a file open and its row menu. Run by `bun run screenshots`;
// output goes to /screenshots/framed.

const t = (id: string) => `[data-testid="${id}"]`;
const FILE = 'src/orders/exportCsv.ts';
const BRANCH = STORY.apiBranchId;

const SOURCE = `import { db } from '../db';

/** Columns of the orders export, in file order. */
export const COLUMNS = [
  { key: 'id', title: 'Order' },
  { key: 'customer', title: 'Customer' },
  { key: 'total', title: 'Total' },
  { key: 'status', title: 'Status' },
];

const ORDERS_QUERY = \`SELECT id, customer, total, status FROM orders WHERE created_at >= $1\`;

/** Quotes a cell when it holds a comma, a quote or a line break. */
export function escapeCell(value: unknown): string {
  const text = String(value ?? '');
  return /[",\\r\\n]/.test(text) ? \`"\${text.replaceAll('"', '""')}"\` : text;
}

export async function exportOrdersCsv(month: Date): Promise<string> {
  const rows = await db.query(ORDERS_QUERY, [month]);
  const header = COLUMNS.map((c) => c.title).join(',');
  const lines = rows.map((row) => COLUMNS.map((c) => escapeCell(row[c.key])).join(','));
  return \`\${[header, ...lines].join('\\r\\n')}\\r\\n\`;
}
`;

const PATHS = [
  'README.md',
  'package.json',
  'src/db.ts',
  'src/orders/exportCsv.test.ts',
  FILE,
  'src/orders/index.ts',
  'src/server.ts',
  'tsconfig.json',
];

test('task files window', async ({ relaunch, consoleErrors }) => {
  const read = contract<Record<string, unknown>>('ade-task-files', 'read');
  const target = contract<{ taskId: string }>('ade-task-files', 'window');
  const control = taskFilesControl([
    {
      channel: IPC.adeTaskFiles,
      response: {
        paths: PATHS,
        status: { 'src/orders/exportCsv.ts': 'M', 'src/orders/exportCsv.test.ts': '?' },
        truncated: false,
      },
    },
    {
      channel: IPC.adeTaskFileRead,
      args: { branchId: BRANCH, path: FILE },
      response: {
        ...read,
        bytes: SOURCE.length,
        language: 'typescript',
        stat: { hash: 'a'.repeat(64), size: SOURCE.length },
        text: SOURCE,
      },
    },
  ]);
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: storyBoard([
      ...control,
      { channel: IPC.adeTaskFilesWindowTarget, response: { ...target, taskId: STORY.taskId } },
    ]),
  });
  await page.locator(t('ade-files-window')).waitFor();
  await expect(page.locator(t('ade-files-row')).first()).toBeVisible();
  const dirRow = (path: string) => page.locator(`${t('ade-files-row')}[data-path="${path}"]`);
  for (const dir of ['src', 'src/orders']) {
    const dirEl = dirRow(dir);
    if ((await dirEl.getAttribute('aria-expanded')) === 'false') await dirEl.click();
  }
  const row = dirRow(FILE);
  await row.click();
  await expect(page.locator(`${t('ade-files-viewer')}[data-path="${FILE}"]`)).toBeVisible();
  await row.click({ button: 'right' });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await page.clock.setFixedTime(CAPTURE_NOW + 1000);
  await shoot(page, 'space-files-window', consoleErrors);
});
