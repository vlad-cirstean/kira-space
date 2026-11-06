import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { openHttpModeAndNewRequest } from './support/apiMode';
import { typeInto as typeIntoView } from './support/editor';
import { editorText } from './support/editorText';

// Four tests, one httpSend snapshot each (the same one-snapshot-per-test constraint
// http-request.spec.ts's own header comment states — a channel with more than one snapshot
// matches on args, and the send's renderer-minted opId makes two sends in one test unmatchable).

async function openBodyPane(page: Page): Promise<void> {
  await page.click('[data-testid="http-request-pane-body"]');
}

// insertText (a single CDP Input.insertText, not per-character key events) rather than
// keyboard.type — Monaco's own `locator.fill()` never works on it at all, and a
// one-shot insert also sidesteps any per-keystroke input handler (auto-closing brackets/quotes)
// that could otherwise fight an already-closed XML document (this file writes its own closing
// tags) the way CodeMirror's autoCloseTags once did.
async function typeInto(view: Locator, page: Page, text: string): Promise<void> {
  await typeIntoView(view, page, text, { paste: true });
}

/** True when some descendant of `view` (a MonacoHost) has a token painted in the given resolved
 *  color — the syntax-highlighted-token check, since `monacoTheme.ts`'s token rules generate
 *  opaque `mtk*` class names rather than semantic ones, the same technique row-coloring.spec.ts
 *  already uses against a known --kira-syntax-* value. */
async function hasTokenColor(view: Locator, rgb: string): Promise<boolean> {
  return view.locator('.view-lines').evaluate((el, wantRgb) => {
    for (const node of el.querySelectorAll('*')) {
      if (getComputedStyle(node).color === wantRgb) return true;
    }
    return false;
  }, rgb);
}

const HTTP_SEND_OK = {
  status: 200,
  statusText: 'OK',
  proto: 'HTTP/1.1',
  headers: [],
  body: '',
  bodyEncoding: 'utf8',
  bodyBytes: 0,
  bodyTruncated: false,
  elapsedMs: 3,
  finalUrl: 'https://api.example.com/echo',
  redirects: [],
};

test('Http request body — code · XML round-trips through the builder and the wire', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: HTTP_SEND_OK }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await openBodyPane(page);
  await page.click('[data-testid="http-body-mode-code"]');
  await page.selectOption('[data-testid="http-body-code-language"]', 'xml');

  // --kira-syntax-tag: #6e9fd8 = rgb(110, 159, 216) (theme/tokens.css) — a tag name colored this
  // way is the observable proof the xml() grammar (not plain text) is active.
  const editor = page.locator('[data-testid="http-request-pane"]');
  await typeInto(editor, page, '<root><a>1</a><b>two</b></root>');
  // expect.poll (auto-retrying), not a one-shot evaluate() — the same reason
  // api-ui-consistency.spec.ts's own token-colour check does (its comment at the same call).
  // Monaco's own debounced marker/tokenize pass can still be catching up on the language-mode
  // switch plus the insertText above when this first samples the DOM.
  await expect.poll(() => hasTokenColor(editor, 'rgb(110, 159, 216)')).toBe(true);

  await page.click('[data-testid="http-body-beautify"]');
  const BEAUTIFIED = '<root>\n  <a>\n    1\n  </a>\n  <b>\n    two\n  </b>\n</root>';
  expect(await editorText(editor)).toBe(BEAUTIFIED);

  await expect(page.locator('[data-testid="http-body-content-type-caption"]')).toHaveText(
    'Content-Type: application/xml (auto)',
  );

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/echo');
  await page.click('[data-testid="http-send"]');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    body: { mode: 'code', codeLanguage: 'xml', code: BEAUTIFIED },
  });
});

test('Http request body — form-data with a real file field sends a path, never bytes', async ({
  relaunch,
}) => {
  const PICKED_FILE = {
    canceled: false,
    file: { path: '/tmp/report.csv', name: 'report.csv', size: 4 * 1024 * 1024 },
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.filesChooseOpen, response: PICKED_FILE },
    { channel: IPC.httpSend, response: HTTP_SEND_OK },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await openBodyPane(page);
  await page.click('[data-testid="http-body-mode-formdata"]');

  const rows = page.locator('[data-testid="http-formdata-row"]');

  // Row 0: a text field.
  await rows.nth(0).locator('[data-testid="http-formdata-name"]').fill('title');
  await rows.nth(0).locator('[data-testid="http-formdata-value"]').fill('hello');

  // Row 1 (now the trailing blank row): switch to File, name it, and pick a file.
  await rows.nth(1).locator('[data-testid="http-formdata-kind"]').selectOption('file');
  await rows.nth(1).locator('[data-testid="http-formdata-name"]').fill('upload');
  await rows.nth(1).locator('[data-testid="http-formdata-choose-file"]').click();
  const fileCaption = rows.nth(1).locator('[data-testid="http-formdata-file-caption"]');
  await expect(fileCaption).toHaveText('report.csv (4.0 MB)');
  // Finding 7: the caption shows only the basename — the full path (a shared collection could
  // point this at an arbitrary local file) must still be auditable somewhere before Send.
  await fileCaption.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    '/tmp/report.csv',
  );

  // Row 2 (now the trailing blank row): a text field the user leaves *disabled* — D5: only
  // enabled, named rows may ever cross the wire.
  await rows.nth(2).locator('[data-testid="http-formdata-name"]').fill('skip');
  await rows.nth(2).locator('[data-testid="http-formdata-value"]').fill('skip-value');
  await rows.nth(2).locator('[data-testid="http-formdata-enabled"]').uncheck();

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/upload');
  await page.click('[data-testid="http-send"]');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  const body = (sendCalls[0].args as { body: { formData: unknown[] } }).body;
  expect(body.formData).toEqual([
    { name: 'title', kind: 'text', value: 'hello', path: '', contentType: '' },
    { name: 'upload', kind: 'file', value: '', path: '/tmp/report.csv', contentType: 'text/csv' },
  ]);

  // The load-bearing assertion: the picked file's bytes never crossed the bridge — every
  // call this test made (a no-args call logs `args: undefined`, which JSON.stringify reports as
  // `undefined`, not a string — treated as 0 bytes) is bounded against the picked file's own size,
  // so no logged argument can be carrying it. A regression that put the file on the wire
  // would carry the file's bytes (base64-inflated, so even larger) and fail this bound.
  for (const entry of control.log()) {
    const size = entry.args === undefined ? 0 : JSON.stringify(entry.args).length;
    expect(size).toBeLessThan(PICKED_FILE.file.size);
  }
});

test('Http request body — binary and code both persist and restore', async ({ relaunch }) => {
  const RESTORED_TAB = {
    id: 'tab-binary-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'POST',
      url: 'https://api.example.com/upload',
      headers: [],
      bodyMode: 'file',
      binaryFile: { path: '/tmp/image.png', name: 'image.png', size: 12345 },
      requestPane: 'body',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [RESTORED_TAB] },
    { channel: IPC.httpSend, response: HTTP_SEND_OK },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  const binaryCaption = page.locator('[data-testid="http-binary-file-caption"]');
  await expect(binaryCaption).toHaveText('image.png (12.1 KB)');
  // Finding 7: same as the form-data file row — the full path must be auditable, not just the
  // basename an imported (possibly malicious) collection chose to show.
  await binaryCaption.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    '/tmp/image.png',
  );

  await page.click('[data-testid="http-body-mode-code"]');
  await page.selectOption('[data-testid="http-body-code-language"]', 'javascript');
  const CODE = 'fetch("/widgets").then((r) => r.json());';
  await typeInto(page.locator('[data-testid="http-request-pane"]'), page, CODE);

  await expect
    .poll(
      () => {
        const saves = control.log().filter((e) => e.channel === IPC.tabsSave);
        if (saves.length === 0) return null;
        const args = saves[saves.length - 1].args as { tabs: { id: string; state: unknown }[] };
        const tab = args.tabs.find((t) => t.id === 'tab-binary-1');
        return tab?.state ?? null;
      },
      { timeout: 3000 },
    )
    .toMatchObject({ code: CODE, codeLanguage: 'javascript' });

  await page.click('[data-testid="http-send"]');
  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    body: { mode: 'code', codeLanguage: 'javascript', code: CODE },
  });
});

test('Http request body — a pre-P3 tab restores into code · JSON', async ({ relaunch }) => {
  const PRE_P3_TAB = {
    id: 'tab-pre-p3-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'POST',
      url: 'https://api.example.com/widgets',
      headers: [],
      bodyMode: 'json',
      body: '{"name":"gizmo"}',
      requestPane: 'body',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [PRE_P3_TAB] }];
  const { window: page } = await relaunch({ control: CONTROL });

  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  // P15 D6: code + codeLanguage: 'json' is presented as the top-level JSON segment now, with no
  // code-language <select> to assert on for it (JSON's own segment, item 6) — same stored state
  // (bodyMode: 'code', codeLanguage: 'json'), new presentation.
  await expect(page.locator('[data-testid="http-body-mode-json"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  await expect(page.locator('[data-testid="http-body-code-language"]')).toHaveCount(0);
  const editor = page.locator('[data-testid="http-request-pane"]');
  expect(await editorText(editor)).toBe('{"name":"gizmo"}');

  // The legacy alias's other half: switching to a mode absent from the pre-P3 record renders an
  // empty table rather than throwing.
  await page.click('[data-testid="http-body-mode-formdata"]');
  await expect(page.locator('[data-testid="http-formdata-table"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-formdata-row"]')).toHaveCount(1); // the trailing blank row only
});
