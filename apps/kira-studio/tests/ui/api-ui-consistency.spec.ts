import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { grpcTab, modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { stableBoundingBox } from './support/editorText';
import { IPC } from './support/ipcChannels';

// A small, Api-only spec (the SPEC's module-boundary rule — "a single test file covering
// both is not") guarding three things a restyle can genuinely regress, not the restyles
// themselves: a de-duplicated affordance staying de-duplicated, an HTML-validity fix staying
// fixed, and one new control's enabled state tracking real data.

test('collections empty state has no duplicate action buttons (D6)', async ({ relaunch }) => {
  // No control mock needed — mockRuntime.ts's own WILDCARD_DEFAULTS answers collectionsList with
  // an empty tree, which is exactly the state under test.
  const { window: page } = await relaunch({});
  await modeTab(page, 'api').click();

  const empty = page.locator(
    '[data-testid="new-collection-empty"], [data-testid="import-collection-empty"], [data-testid="new-request-empty"]',
  );
  await expect(empty).toHaveCount(0);
  // The header's own actions are the one copy that remains reachable. P28 D18 moved the import
  // out of this header entirely — it is a menu-bar command (and a command-palette entry) now.
  await expect(page.locator('[data-testid="new-request"]')).toBeVisible();
  await expect(page.locator('[data-testid="new-collection"]')).toBeVisible();
  await expect(page.locator('[data-testid="import-collection"]')).toHaveCount(0);
  // D6/F3: no bordered dialog button anywhere in the panel's own empty-state slot. P110 B26:
  // the old .p-dlgbtn class is gone (folded into Button variant="dialog"/"dialog-primary") --
  // `data-variant` is the component's own stable attribute, set regardless of variant name.
  await expect(
    page.locator('.side-empty [data-slot="button"][data-variant^="dialog"]'),
  ).toHaveCount(0);
});

// Minimal schema for the D12 test below — needs only enough for findMethod to resolve
// demo.Echo/SayHello, mirroring grpc-request.spec.ts's own UNARY_SCHEMA.
const UNARY_SCHEMA = {
  services: [
    {
      name: 'demo.Echo',
      fullName: 'demo.Echo',
      methods: [
        {
          name: 'SayHello',
          fullName: 'demo.Echo/SayHello',
          clientStreaming: false,
          serverStreaming: false,
          inputType: 'demo.HelloRequest',
          outputType: 'demo.HelloReply',
          requestTemplate: '{\n  "name": ""\n}',
        },
      ],
    },
  ],
  mode: 'reflection',
  warnings: [] as string[],
};

const GRPC_HISTORY_ENTRY = {
  id: 'call-1',
  itemId: null,
  tabId: 'tab-grpc-1',
  calledAt: '2026-01-01T00:00:00.000Z',
  target: 'demo.example.com:443',
  method: 'demo.Echo/SayHello',
  streaming: 'unary',
  environment: '',
  code: 0,
  codeName: 'OK',
  statusMessage: '',
  elapsedMs: 5,
  messageCount: 1,
  messageBytes: 20,
  storedBytes: 20,
};

test('gRPC history row delete control is not nested inside a button (F20)', async ({
  relaunch,
}) => {
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({})] },
    { channel: IPC.grpcHistoryList, args: scope, response: [GRPC_HISTORY_ENTRY] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const row = page.locator('[data-testid="grpc-history-row"]');
  await expect(row).toHaveCount(1);
  const deleteBtn = row.locator('[data-testid="grpc-history-delete"]');
  await expect(deleteBtn).toBeVisible();
  // The row itself may legitimately be an interactive element, but the delete control must never
  // be a descendant of a <button> — a <button> nested in a <button> is invalid HTML the parser
  // silently hoists out of its parent at parse time (F20), which this asserts can't come back.
  await expect(deleteBtn.locator('xpath=ancestor::button')).toHaveCount(0);
});

test('gRPC history Clear tracks whether there is anything to clear (D12)', async ({ relaunch }) => {
  const scope = { itemId: '', tabId: 'tab-grpc-1' };

  // service/method set and a live call answered so the response section (and its History pane)
  // renders even though the history list itself comes back empty — the same `hasResult ||
  // hasHistory` gate response-pane.vue's HTTP twin has.
  const empty = await relaunch({
    control: [
      {
        channel: IPC.tabsList,
        response: [
          grpcTab({ target: 'demo.example.com:443', service: 'demo.Echo', method: 'SayHello' }),
        ],
      },
      // Pre-existing bug: a restored tab with no target never triggers
      // loadSchema's own watch (GrpcRequestView.vue's `mode === 'reflection' && !target` early
      // return), so `methodResolved` never resolves and Call stays disabled forever —
      // grpc-request.spec.ts's own comment on this exact fix has the full rationale.
      { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
      {
        channel: IPC.grpcCall,
        response: {
          code: 0,
          codeName: 'OK',
          statusMessage: '',
          elapsedMs: 3,
          header: [],
          trailer: [],
          messageCount: 1,
          messageBytes: 10,
          messages: [{ seq: 0, json: '{}', wireBytes: 10, offsetMs: 0 }],
        },
      },
      { channel: IPC.grpcHistoryList, args: scope, response: [] },
    ],
  });
  await empty.window.click('[data-testid="grpc-call"]');
  await empty.window.click('[data-testid="grpc-response-pane-history"]');
  await expect(empty.window.locator('[data-testid="grpc-history-clear"]')).toBeDisabled();

  // Non-empty history alone is enough to render the toolbar — no live call needed here.
  const withEntries = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({})] },
      { channel: IPC.grpcHistoryList, args: scope, response: [GRPC_HISTORY_ENTRY] },
    ],
  });
  await expect(withEntries.window.locator('[data-testid="grpc-history-clear"]')).toBeEnabled();
});

// A freshly opened gRPC tab has neither a service nor a method, so Call is disabled
// from the moment the tab opens — the one place the app-wide disabled-primary-button defect
// (every primary Button variant, not a gRPC-specific bug) was visible to a user.
// Asserted as "the disabled primary path uses opacity", not as a contrast number computed in the
// test.
test('a disabled Call button dims via opacity, not an unreadable label on a full-strength fill (D3)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.tabsList, response: [grpcTab({})] }],
  });

  const call = page.locator('[data-testid="grpc-call"]');
  await expect(call).toBeDisabled();
  const { color, opacity } = await call.evaluate((el) => {
    const style = getComputedStyle(el as HTMLElement);
    return { color: style.color, opacity: style.opacity };
  });
  // rgb(99, 99, 107) is --kira-fg-disabled — the pre-D3 defect kept this on the full accent
  // fill. Post-D3 the label stays --kira-accent-fg and the whole control dims via opacity.
  expect(color).not.toBe('rgb(99, 99, 107)');
  expect(Number.parseFloat(opacity)).toBeLessThan(1);
});

// P15 §4: four cases guarding this phase's own end-to-end behaviour, not the restyles themselves —
// the response pane's chrome present from tab-open (D1), JSON as a top-level body segment (D6),
// and the tab-strip badge (D8).

test('a freshly opened request tab shows the response pane switcher before any send (D1)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await openHttpModeAndNewRequest(page);

  // Present from the moment the tab opens — asserted as a presence so a future refactor cannot
  // quietly re-hide it behind a first send again.
  await expect(page.locator('[data-testid="http-response-pane-toggle"]')).toBeVisible();

  // Each pane owns its own empty state — switching to Headers on a never-sent tab renders one
  // rather than throwing.
  await page.click('[data-testid="http-response-pane-headers"]');
  const headers = page.locator('[data-testid="http-response-headers"]');
  await expect(headers.locator('[data-slot="empty-state"]')).toHaveText(
    'Send a request to see the response',
  );
});

test('the body-mode segmented control has a JSON segment (D6)', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-request-pane-body"]');

  await page.click('[data-testid="http-body-mode-json"]');
  await expect(page.locator('[data-testid="http-body-mode-json"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  // Same segmented control the request pane's own toggle uses for the Body segment's count/kind
  // badge — selecting JSON is reflected there end to end, without asserting storage directly.
  await expect(page.locator('[data-testid="http-request-pane-body"]')).toHaveText('Body (JSON)');
});

function httpRequestTab(
  id: string,
  order: number,
  active: boolean,
  state: Record<string, unknown>,
): Record<string, unknown> {
  return {
    id,
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order,
    active,
    state: {
      method: 'GET',
      url: '',
      headers: [],
      bodyMode: 'none',
      body: '',
      code: '',
      codeLanguage: 'json',
      urlEncoded: [],
      formData: [],
      binaryFile: null,
      itemId: null,
      name: '',
      requestPane: 'body',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
      ...state,
    },
  };
}

test('a request tab with a body shows a tab-strip badge; one with none does not (D8)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        httpRequestTab('tab-has-body', 0, true, { bodyMode: 'raw', body: 'hello' }),
        httpRequestTab('tab-no-body', 1, false, { bodyMode: 'none' }),
      ],
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  const withBody = page.locator('[data-testid="tab"][data-tab-id="tab-has-body"]');
  const withoutBody = page.locator('[data-testid="tab"][data-tab-id="tab-no-body"]');
  await expect(withBody.locator('[data-testid="tab-badge"]')).toHaveCount(1);
  await expect(withoutBody.locator('[data-testid="tab-badge"]')).toHaveCount(0);
});

// P15b §4: six cases guarding this phase's own end-to-end behaviour — header-name autocomplete,
// {{variable}} colouring/hover/completion, and arrow-key navigation across the request tables.

const P15B_ENV = { id: 'env-p15b', name: 'Prod', sortOrder: 0, isActive: true };
const P15B_BASE_URL_VAR = {
  id: 'var-base-url',
  scope: 'environment',
  ownerId: 'env-p15b',
  name: 'base_url',
  value: 'api.example.com',
  isSecret: false,
  sortOrder: 0,
};
const P15B_SECRET_VAR = {
  id: 'var-api-key',
  scope: 'environment',
  ownerId: 'env-p15b',
  name: 'api_key',
  value: '',
  isSecret: true,
  sortOrder: 1,
};

function p15bVariableControl(): ControlSnapshot[] {
  return [
    { channel: IPC.variablesListEnvironments, response: [P15B_ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-p15b' },
      response: [P15B_BASE_URL_VAR, P15B_SECRET_VAR],
    },
  ];
}

test('the URL field: typing {{ auto-closes, and accepting a suggestion produces a clean reference', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const url = page.locator('[data-testid="http-url"]');
  await url.click();
  // D5(b) rule 1 composing twice: `{` gives `{|}`, a second `{` gives `{{|}}` — the reference the
  // user was going to type, caret where the name goes.
  await url.pressSequentially('{{');
  await expect(url).toHaveValue('{{}}');

  // D3(b)'s templateToken opens the popup once there is a non-empty word inside the reference.
  await url.pressSequentially('b');
  await expect(url).toHaveValue('{{b}}');
  const suggestions = page.locator('.autocomplete-suggestions li');
  await expect(suggestions.filter({ hasText: 'base_url' })).toBeVisible();

  // Accepting produces `{{base_url}}` — not `{{base_url}}}}` — the exact interaction between
  // auto-close and accept a reviewer cannot check by reading alone.
  await page.keyboard.press('Tab');
  await expect(url).toHaveValue('{{base_url}}');
  await expect(suggestions).toHaveCount(0);
});

test('the URL field paints a resolved reference and an unknown one differently', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.fill('[data-testid="http-url"]', 'https://{{base_url}}/v1?missing={{not_defined}}');

  const overlay = page.locator('.url-field .highlight-overlay');
  await expect(overlay.locator('.kira-ed-var')).toHaveCount(1);
  await expect(overlay.locator('.kira-ed-var-unknown')).toHaveCount(1);
});

test('hovering a resolved reference shows its value; hovering a secret never does', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.fill('[data-testid="http-url"]', 'https://{{base_url}}/{{api_key}}');

  const resolvedSpan = page.locator('.url-field .kira-ed-var');
  await expect(resolvedSpan).toHaveCount(1);
  const resolvedBox = await stableBoundingBox(resolvedSpan);
  await page.mouse.move(
    resolvedBox.x + resolvedBox.width / 2,
    resolvedBox.y + resolvedBox.height / 2,
  );
  const hover = page.locator('[data-testid="autocomplete-hover"]');
  await expect(hover).toBeVisible();
  await expect(hover).toContainText('api.example.com');
  await expect(hover).toContainText('Environment variable');

  // Move away, then hover the secret reference — a fresh token, so the panel re-arms.
  await page.mouse.move(0, 0);
  await expect(hover).toHaveCount(0);

  const secretSpan = page.locator('.url-field .kira-ed-var-secret');
  await expect(secretSpan).toHaveCount(1);
  const secretBox = await secretSpan.boundingBox();
  if (!secretBox) throw new Error('secret reference span has no box');
  await page.mouse.move(secretBox.x + secretBox.width / 2, secretBox.y + secretBox.height / 2);
  await expect(hover).toBeVisible();
  await expect(hover).toContainText('Resolved when the request is sent');
  // The security property, not a UX one: the secret's plaintext (empty in this fixture, but the
  // point stands architecturally — mergedValuesAndSecrets never hands a secret's value to
  // variableCompletion.ts at all, F5) never appears, and neither does the word "value".
  await expect(hover).not.toContainText('api_key');
});

// The request body editors are the single largest place a user writes {{variables}}
// — until now they only got rangeHighlights (colouring), and a resolved reference read as
// exactly the same colour as the JSON keys around it (`--kira-syntax-property`, byte-identical to
// the old `--kira-var-resolved` alias). This guards the fix end to end: a colour of its own, a
// hover, and completion, all inside the JSON body — not just the URL field the tests above cover.
test('the request body: a resolved {{variable}} has a colour of its own, hovers, and completes', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        httpRequestTab('tab-1', 0, true, {
          bodyMode: 'code',
          codeLanguage: 'json',
          code: '{\n  "base": "{{base_url}}",\n  "missing": "{{not_defined}}"\n}',
        }),
      ],
    },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const body = page.locator('[data-testid="http-request-pane"]');
  await expect(body.locator('.monaco-host')).toBeVisible();

  const resolvedSpan = body.locator('.kira-ed-var');
  const unknownSpan = body.locator('.kira-ed-var-unknown');
  await expect(resolvedSpan).toHaveCount(1);
  await expect(unknownSpan).toHaveCount(1);

  // --kira-var-resolved (#64bead) vs --kira-warn (#d5b449) vs --kira-syntax-property (#93c6e8,
  // what the "base"/"missing" JSON keys beside them are painted in) — three distinct colours, not
  // the two the resolved reference used to share with its own JSON key. monacoTheme.ts's own token
  // rules generate opaque `mtk*` class names rather than semantic ones (the same reason
  // http-request-body.spec.ts's own hasTokenColor() walks every descendant by colour instead of a
  // class selector), so the key's colour is found the same way here.
  // `toHaveCSS`/`expect.poll` (auto-retrying), not a one-shot `evaluate()`: Monaco's own debounced
  // marker pass (`scheduleLint`, independent of these decorations) can re-render `.view-lines`'
  // span DOM shortly after mount, and a plain `evaluate()` can catch that mid-churn.
  await expect(resolvedSpan).toHaveCSS('color', 'rgb(100, 190, 173)');
  await expect(unknownSpan).toHaveCSS('color', 'rgb(213, 180, 73)');
  let keyColor = '';
  const readKeyColor = (): Promise<string> =>
    body.evaluate((el) => {
      for (const node of el.querySelectorAll('.view-lines span')) {
        if (
          node.classList.contains('kira-ed-var') ||
          node.classList.contains('kira-ed-var-unknown')
        ) {
          continue;
        }
        if (node.textContent?.includes('base') || node.textContent?.includes('missing')) {
          return getComputedStyle(node).color;
        }
      }
      return '';
    });
  await expect
    .poll(async () => {
      keyColor = await readKeyColor();
      return keyColor;
    })
    .not.toBe('');
  const resolvedColor = await resolvedSpan.evaluate((el) => getComputedStyle(el).color);
  expect(resolvedColor).not.toBe(keyColor);

  // Hovering the resolved reference shows its value — through Monaco's own native hover widget
  // (MonacoHost.vue's own hover provider, `.monaco-hover`), not AutocompleteField's own overlay
  // panel (`autocomplete-hover`): the body editor is a real editable host, wired directly via
  // MonacoHost's hoverSource prop, not the URL field's read-only-overlay-behind-an-input trick.
  const resolvedBox = await stableBoundingBox(resolvedSpan);
  await page.mouse.move(
    resolvedBox.x + resolvedBox.width / 2,
    resolvedBox.y + resolvedBox.height / 2,
  );
  const hover = page.locator('.monaco-hover:not(.hidden)');
  await expect(hover).toBeVisible();
  await expect(hover).toContainText('api.example.com');

  // Typing {{ inside the body offers names — Monaco's own completion popup, since the body editor
  // is a real editable host, not an AutocompleteField overlay.
  await body.locator('.view-lines').click();
  await page.keyboard.press('End');
  await page.keyboard.press('End');
  await page.keyboard.type('"x": "{{base');
  const tooltip = page.locator('.suggest-widget.visible');
  await expect(tooltip).toBeVisible({ timeout: 5_000 });
  await expect(tooltip).toContainText('base_url');
});

// The actual originally-reported bug — the request body's {{variable}} hover tooltip
// stayed nested inside two `overflow: hidden` ancestors (`MonacoHost.vue`'s own `.monaco-host`,
// HttpRequestView.vue's `.request-pane`) with no shared, body-level `overflowWidgetsDomNode` to
// escape them, and its z-index had no stated relationship to this app's real `--kira-z-*` ladder.
// `fixedOverflowWidgets: true` alone does NOT reparent a widget to
// `document.body` (verified against the pinned Monaco's own `view.js` — it appends the
// "overflowing" widgets container to the editor's own root DOM node when no explicit
// `overflowWidgetsDomNode` is supplied); `editor/monaco.ts`'s `overflowWidgetsContainer()` is the
// fix, a single shared container every MonacoHost instance points at. Both are checked as direct
// DOM/CSS facts rather than pixel geometry — a stronger, viewport-independent proof that the fix
// landed, not a coincidence of one particular window size happening to leave enough room.
test('the request body {{variable}} hover escapes the editor pane and adopts the coordinated z-index (G20 D7)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        httpRequestTab('tab-1', 0, true, {
          bodyMode: 'code',
          codeLanguage: 'json',
          code: '{\n  "base": "{{base_url}}"\n}',
        }),
      ],
    },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const body = page.locator('[data-testid="http-request-pane"]');
  await expect(body.locator('.monaco-host')).toBeVisible();

  const resolvedSpan = body.locator('.kira-ed-var');
  await expect(resolvedSpan).toHaveCount(1);
  const resolvedBox = await stableBoundingBox(resolvedSpan);
  // A genuine leave-then-enter, not a teleport from Playwright's own initial (0, 0) mouse position
  // straight onto the token — Monaco's hover controller only arms its delay timer on a fresh
  // "entered this token" transition, and starting from a stale internal target left by an earlier
  // step (`stableBoundingBox`'s own polling included) can leave that transition undetected. Under
  // CPU load the transition can still be missed, so redo the whole leave-then-enter until it lands.
  const hover = page.locator('.monaco-hover:not(.hidden)');
  await expect(async () => {
    await page.mouse.move(0, 0);
    await page.mouse.move(
      resolvedBox.x + resolvedBox.width / 2,
      resolvedBox.y + resolvedBox.height / 2,
    );
    await expect(hover).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 15_000 });

  // D7's first half: escapes both named clipping ancestors, and is mounted as a direct child of
  // <body> — exactly what `overflowWidgetsDomNode` produces, and what the library's own pre-fix
  // default (the editor's own DOM node) never did.
  const [escapesRequestPane, escapesMonacoHost, escapesAppRoot, isInBody] = await Promise.all([
    hover.evaluate((el) => el.closest('.request-pane') === null),
    hover.evaluate((el) => el.closest('.monaco-host') === null),
    // main.ts mounts the whole app onto '#app' — escaping it entirely (not merely its two named
    // ancestors above) is the strongest, wrapper-depth-independent proof that
    // `overflowWidgetsDomNode` re-parented this tooltip's own container, rather than
    // merely resolving true by accident of which two specific classes happened to be checked.
    hover.evaluate((el) => el.closest('#app') === null),
    hover.evaluate((el) => document.body.contains(el)),
  ]);
  expect(escapesRequestPane, 'must not be a descendant of .request-pane').toBe(true);
  expect(escapesMonacoHost, 'must not be a descendant of .monaco-host').toBe(true);
  expect(escapesAppRoot, 'must not be a descendant of #app at all').toBe(true);
  expect(isInBody, 'the tooltip must still be attached under <body>').toBe(true);

  // D7's second half: the coordinated z-index — MonacoHost.vue's own global `.monaco-hover`
  // override sets it directly on the widget itself (no un-themed library default in between), so
  // this must read the *token's* current value, not any library literal (asserting this rather
  // than merely ">= the toolbar's z-index" catches the override silently reverting to a library
  // default just as reliably, and ties the two numbers together for real instead of by
  // coincidence).
  const [tooltipZIndex, expectedZIndex] = await Promise.all([
    hover.evaluate((el) => getComputedStyle(el).zIndex),
    page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--kira-z-tooltip').trim(),
    ),
  ]);
  expect(tooltipZIndex).toBe(expectedZIndex);
});

test('a header-name cell suggests Content-Type, and typing Content-T then accepting does not duplicate it', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="http-request-pane-headers"]');
  const nameInput = page.locator('[data-testid="http-header-name"]').first();
  await nameInput.click();
  await nameInput.pressSequentially('cont');
  const suggestions = page.locator('.autocomplete-suggestions li');
  await expect(suggestions.filter({ hasText: 'Content-Type' })).toBeVisible();
  await page.keyboard.press('Escape');
  await nameInput.fill('');

  // F1's own concatenation bug, asserted so it cannot come back: the default word-run tokenizer
  // has no `-`, so accepting after `Content-T` would otherwise produce `Content-Content-Type`.
  await nameInput.click();
  await nameInput.pressSequentially('Content-T');
  await expect(suggestions.filter({ hasText: 'Content-Type' })).toBeVisible();
  await page.keyboard.press('Tab');
  await expect(nameInput).toHaveValue('Content-Type');
});

test('ArrowDown moves focus down a column, but not while the completion popup is navigating (D6)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        httpRequestTab('tab-1', 0, true, {
          headers: [
            { name: 'Authorization', value: 'Bearer x', enabled: true },
            { name: 'Accept', value: 'application/json', enabled: true },
          ],
        }),
      ],
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="http-request-pane-headers"]');
  const names = page.locator('[data-testid="http-header-name"]');
  await expect(names).toHaveCount(3); // two real rows plus the trailing blank one

  await names.nth(0).click();
  await page.keyboard.press('ArrowDown');
  await expect(names.nth(1)).toBeFocused();

  // While the popup is open and navigating, ArrowDown must move the *popup* selection, not focus.
  await names.nth(0).click();
  await names.nth(0).fill('');
  await names.nth(0).pressSequentially('cont');
  const suggestions = page.locator('.autocomplete-suggestions li');
  await expect(suggestions.filter({ hasText: 'Content-Type' })).toBeVisible();
  await page.keyboard.press('ArrowDown');
  await expect(names.nth(0)).toBeFocused();
  await expect(names.nth(1)).not.toBeFocused();
});

// P16 §4: the Api half of this phase's own new coverage — D6 (dropdown height parity), D8/D9
// (the two retinted tokens), D11 (the response find bar), D12/D13 (the headers pane filter and
// the request tables' index-carry-through under a filter).

test('http-method-select and http-url report the same height (D6)', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const methodHeight = await page
    .locator('[data-testid="http-method-select"]')
    .evaluate((el) => (el as HTMLElement).offsetHeight);
  const urlBoxHeight = await page
    .locator('.p-input:has([data-testid="http-url"])')
    .evaluate((el) => (el as HTMLElement).offsetHeight);
  expect(urlBoxHeight).toBe(methodHeight);
});

test('a placeholder is muted text, and the elapsed caption is secondary (muted) text, never disabled (D8/D9)', async ({
  relaunch,
}) => {
  const RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  // D8: pins the two tokens against a future well-meaning revert — computed colour, not the
  // token's own declared value, since ::placeholder pseudo-elements don't expose custom
  // properties directly to getComputedStyle on the real element.
  const [placeholderColor, mutedToken] = await page.evaluate(() => {
    const input = document.querySelector('[data-testid="http-url"]') as HTMLElement;
    const cs = getComputedStyle(input, '::placeholder');
    const root = getComputedStyle(document.documentElement);
    return [cs.color, root.getPropertyValue('--kira-fg-muted').trim()];
  });
  expect(placeholderColor).not.toBe('');
  // Convert the muted token (a hex string) and the computed placeholder colour to the same
  // rgb(...) form the browser reports, via a throwaway element.
  const mutedRgb = await page.evaluate((hex) => {
    const probe = document.createElement('span');
    probe.style.color = hex;
    document.body.appendChild(probe);
    const rgb = getComputedStyle(probe).color;
    probe.remove();
    return rgb;
  }, mutedToken);
  expect(placeholderColor).toBe(mutedRgb);

  // D9: the elapsed-time caption in the response status row (shown once a response has landed)
  // is secondary content (P267 §2.3): --kira-fg-muted, not --kira-fg-disabled.
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-elapsed"]')).toBeVisible();
  const [dimColor, mutedElapsedRgb] = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="http-elapsed"]') as HTMLElement;
    const probe = document.createElement('span');
    probe.style.color = getComputedStyle(document.documentElement)
      .getPropertyValue('--kira-fg-muted')
      .trim();
    document.body.appendChild(probe);
    const rgb = getComputedStyle(probe).color;
    probe.remove();
    return [getComputedStyle(el).color, rgb];
  });
  expect(dimColor).toBe(mutedElapsedRgb);
});

test('the response find bar counts and steps through matches, and Escape clears them (D11)', async ({
  relaunch,
}) => {
  const RESPONSE_BODY = '{"id":1,"name":"Ada","note":"Ada Lovelace"}'; // "Ada" appears twice
  const RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [{ name: 'Content-Type', value: 'application/json' }],
    body: RESPONSE_BODY,
    bodyEncoding: 'utf8',
    bodyBytes: RESPONSE_BODY.length,
    bodyTruncated: false,
    elapsedMs: 10,
    finalUrl: 'https://api.example.com/users',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [httpRequestTab('tab-1', 0, true, { url: 'https://api.example.com/users' })],
    },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-find-toggle"]');
  await expect(page.locator('[data-testid="http-find-bar"]')).toBeVisible();

  // Real-interaction fix (reported bug — the response search bar sat below/at the bottom of the
  // response content instead of above it): mirrors data-view.spec.ts's own relative-position
  // pattern for the SQL grid's SearchToolbar — the find bar sits above the content it searches,
  // below the toolbar above it, never below the content (ResponsePane.vue's own comment records
  // why: DataView.vue's SearchToolbar usage already found "docks at the bottom of the result"
  // "overlapped the last visible row, which read as a bug rather than a search bar").
  const findBarBox = await page.locator('[data-testid="http-find-bar"]').boundingBox();
  const toolbarBox = await page.locator('.response-status-row').boundingBox();
  const bodyBox = await page.locator('.response-body').boundingBox();
  if (!findBarBox || !toolbarBox || !bodyBox) {
    throw new Error('http-find-bar, response-status-row or response-body has no bounding box');
  }
  expect(findBarBox.y).toBeGreaterThanOrEqual(toolbarBox.y + toolbarBox.height);
  expect(findBarBox.y).toBeLessThan(bodyBox.y);

  await page.fill('[data-testid="http-find-input"]', 'Ada');
  await expect(page.locator('[data-testid="http-find-count"]')).toHaveText('Match 1 of 2');
  await page.click('[data-testid="http-find-next"]');
  await expect(page.locator('[data-testid="http-find-count"]')).toHaveText('Match 2 of 2');
  await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(1);

  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="http-find-bar"]')).toHaveCount(0);
  await expect(page.locator('.kira-ed-find-match')).toHaveCount(0);
  await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(0);
});

test('filtering the response headers pane, and filtering the request headers table without losing a row’s identity (D12/D13)', async ({
  relaunch,
}) => {
  const RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [
      { name: 'Content-Type', value: 'application/json' },
      { name: 'X-Request-Id', value: 'abc123' },
      { name: 'Cache-Control', value: 'no-store' },
    ],
    body: '{}',
    bodyEncoding: 'utf8',
    bodyBytes: 2,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/users',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        httpRequestTab('tab-1', 0, true, {
          url: 'https://api.example.com/users',
          headers: [
            { name: 'Authorization', value: 'Bearer x', enabled: true },
            { name: 'Accept', value: 'application/json', enabled: true },
            { name: 'X-Debug', value: 'on', enabled: true },
          ],
        }),
      ],
    },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  // D12: the response headers pane's own filter, by name or value.
  await page.click('[data-testid="http-send"]');
  await page.click('[data-testid="http-response-pane-headers"]');
  await expect(
    page.locator('[data-testid="http-response-headers"] [data-testid="response-header-row"]'),
  ).toHaveCount(3);
  await page.fill('[data-testid="http-response-headers-filter"]', 'no-store');
  await expect(
    page.locator('[data-testid="http-response-headers"] [data-testid="response-header-row"]'),
  ).toHaveCount(1);
  await expect(page.locator('[data-testid="http-response-headers-count"]')).toHaveText(
    '1 of 3 headers',
  );
  await page.fill('[data-testid="http-response-headers-filter"]', '');

  // D13/F11: the request headers table's own filter, and the index-carry-through hazard —
  // filtering down to one real row and editing it must write to *that* row, never to whichever
  // position it happens to land on in the filtered list.
  await page.click('[data-testid="http-request-pane-headers"]');
  await page.click('[data-testid="http-field-filter-toggle"]');
  await page.fill('[data-testid="http-field-filter"]', 'Debug');
  const filteredRows = page.locator('[data-testid="http-header-row"]');
  await expect(filteredRows).toHaveCount(2); // X-Debug + the trailing blank row, never hidden
  await filteredRows.first().locator('[data-testid="http-header-value"]').fill('off');

  // Closing the toggle clears the filter and restores every row (D13's own rule) — the edit above
  // must have landed on X-Debug (real position 2), not on Authorization (position 0, the filtered
  // list's own position 0) and not on the trailing row.
  await page.click('[data-testid="http-field-filter-toggle"]');
  await expect(page.locator('[data-testid="http-field-filter"]')).toHaveCount(0);
  const allRows = page.locator('[data-testid="http-header-row"]');
  await expect(allRows).toHaveCount(4); // three real rows + trailing
  await expect(allRows.nth(0).locator('[data-testid="http-header-name"]')).toHaveValue(
    'Authorization',
  );
  await expect(allRows.nth(0).locator('[data-testid="http-header-value"]')).toHaveValue('Bearer x');
  await expect(allRows.nth(2).locator('[data-testid="http-header-name"]')).toHaveValue('X-Debug');
  await expect(allRows.nth(2).locator('[data-testid="http-header-value"]')).toHaveValue('off');
});

// P17 §4.4: this phase's own eight new cases — the pipe/fake namespace, the method select, and
// the dialog->tab/duplicate/bulk-edit/overview surfaces item 3/4/5/8 built.

test('typing "{{base_url | " in the URL field lists the six transforms, not the variable names (D13(b))', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const url = page.locator('[data-testid="http-url"]');
  await url.click();
  await url.pressSequentially('https://{{');
  await expect(url).toHaveValue('https://{{}}');
  await url.pressSequentially('base_url | ');
  await expect(url).toHaveValue('https://{{base_url | }}');

  // Ctrl+Space forces the full list open on an empty word (completionKeymap's own "show me
  // everything" binding, shared with the console) — after the pipe, that list is exactly the six
  // transforms, never base_url/api_key.
  await page.keyboard.press('Control+Space');
  const suggestions = page.locator('.autocomplete-suggestions li');
  await expect(suggestions).toHaveCount(6);
  await expect(suggestions.filter({ hasText: 'base_url' })).toHaveCount(0);
  await expect(suggestions.filter({ hasText: 'base64 encode' })).toBeVisible();

  await suggestions.filter({ hasText: 'base64 encode' }).click();
  await expect(url).toHaveValue('https://{{base_url | base64}}');
});

test('the URL field paints a piped reference resolved and an unrecognised transform unknown (D3 rule 4)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.fill(
    '[data-testid="http-url"]',
    'https://{{base_url | base64}}/v1?x={{base_url | nope}}',
  );

  const overlay = page.locator('.url-field .highlight-overlay');
  await expect(overlay.locator('.kira-ed-var')).toHaveCount(1);
  await expect(overlay.locator('.kira-ed-var-unknown')).toHaveCount(1);
});

test("hovering a piped secret names the transform and shows no value (§5's UI-half security assertion)", async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    ...p15bVariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.fill('[data-testid="http-url"]', 'https://{{base_url}}/{{api_key | base64}}');

  const secretSpan = page.locator('.url-field .kira-ed-var-secret');
  await expect(secretSpan).toHaveCount(1);
  const secretBox = await secretSpan.boundingBox();
  if (!secretBox) throw new Error('secret reference span has no box');
  await page.mouse.move(secretBox.x + secretBox.width / 2, secretBox.y + secretBox.height / 2);
  const hover = page.locator('[data-testid="autocomplete-hover"]');
  await expect(hover).toBeVisible();
  await expect(hover).toContainText(
    'Secret. Resolved when the request is sent, then transformed: base64.',
  );
  // The security assertion: no value, transformed or not, and the name isn't repeated either.
  await expect(hover).not.toContainText('api_key');
});

test('the method select opens an app-drawn menu, and PATCH gets its own colour, distinct from PUT (D18/D19)', async ({
  relaunch,
}) => {
  const NOW = '2026-01-01T00:00:00.000Z';
  const TREE = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
    ],
    items: [
      {
        id: 'item-1',
        collectionId: 'col-1',
        parentId: null,
        kind: 'request',
        name: 'Health check',
        sortOrder: 0,
        method: 'GET',
        url: 'https://api.example.com/healthz',
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
  };
  const HEALTH_REQUEST = {
    method: 'GET',
    url: 'https://api.example.com/healthz',
    headers: [],
    bodyMode: 'none',
    body: '',
    code: '',
    codeLanguage: 'json',
    urlEncoded: [],
    formData: [],
    binaryFile: null,
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="collection-row"][data-id="col-1"]')).toBeVisible();
  await page
    .locator('[data-testid="collection-row"][data-id="col-1"]')
    .locator('[data-testid="tree-twisty"]')
    .click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  const trigger = page.locator('[data-testid="http-method-select"]');
  await trigger.click();
  await expect(page.locator('[data-testid="method-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="method-menu-item-GET"] .codicon-check')).toHaveCount(1);

  await page.click('[data-testid="method-menu-item-PATCH"]');
  await expect(page.locator('[data-testid="method-menu"]')).toHaveCount(0);
  await expect(trigger).toHaveAttribute('data-value', 'PATCH');

  // "each a distinct colour" — the trigger's own resolved text colour actually differs between
  // the two methods that used to share one family (PUT/PATCH both 'warn' under httpMethodClass).
  const patchColor = await trigger.evaluate((el) => getComputedStyle(el).color);
  await trigger.click();
  await page.click('[data-testid="method-menu-item-PUT"]');
  const putColor = await trigger.evaluate((el) => getComputedStyle(el).color);
  expect(putColor).not.toBe(patchColor);
});

// A method is coloured text on the control's own neutral ground, not a tinted
// pill — the fill on `.p-method` (to match `.p-chip`'s status variants) reads as a
// second, competing control on the already-bordered method select. Colour survives; the fill
// does not, on every `.p-method` site (the trigger and the head chip both).
test('a method is coloured text on a neutral ground, not a tinted fill (D7)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, { method: 'POST' })] },
    ],
  });

  for (const testid of ['http-method-select', 'http-method-chip']) {
    const el = page.locator(`[data-testid="${testid}"]`);
    const { color, backgroundColor } = await el.evaluate((node) => {
      const style = getComputedStyle(node as HTMLElement);
      return { color: style.color, backgroundColor: style.backgroundColor };
    });
    // A neutral background: fully transparent, or the select's own --kira-bg-input ground
    // (#27272c) — never a color-mix tint of the method colour.
    expect(['rgba(0, 0, 0, 0)', 'rgb(39, 39, 44)']).toContain(backgroundColor);
    // The method's own colour still reaches the label — not the muted/neutral foreground a plain
    // select or chip would otherwise use.
    expect(color).not.toBe('rgba(0, 0, 0, 0)');
  }
});

const P17_TREE = {
  collections: [
    {
      id: 'col-1',
      name: 'Orders API',
      sortOrder: 0,
      createdAt: '2026-01-01T00:00:00.000Z',
      updatedAt: '2026-01-01T00:00:00.000Z',
    },
  ],
  items: [],
};

test('Variables… on a collection row opens a tab, and a second invocation activates it rather than opening a second (D16)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: P17_TREE },
    { channel: IPC.variablesListEnvironments, response: [] },
    { channel: IPC.variablesList, args: { scope: 'collection', ownerId: 'col-1' }, response: [] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await modeTab(page, 'api').click();

  const row = page.locator('[data-testid="collection-row"][data-id="col-1"]');
  await expect(row).toBeVisible();
  await row.click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  await expect(page.locator('[data-testid="variables-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);

  // D16's reuse: true — a second invocation activates the same tab, never a second one.
  await row.click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
});

test('the overview panel opens from a request tab, lists both scopes with their chips, masks a secret, and Edit… opens the D16 tab (item 8)', async ({
  relaunch,
}) => {
  const NOW = '2026-01-01T00:00:00.000Z';
  const TREE = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
    ],
    items: [
      {
        id: 'item-1',
        collectionId: 'col-1',
        parentId: null,
        kind: 'request',
        name: 'Health check',
        sortOrder: 0,
        method: 'GET',
        url: 'https://api.example.com/healthz',
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
  };
  const HEALTH_REQUEST = {
    method: 'GET',
    url: 'https://api.example.com/healthz',
    headers: [],
    bodyMode: 'none',
    body: '',
    code: '',
    codeLanguage: 'json',
    urlEncoded: [],
    formData: [],
    binaryFile: null,
  };
  const ENV = { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true, description: '' };
  const ENV_VAR = {
    id: 'var-env',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'region',
    value: 'us-east',
    isSecret: false,
    sortOrder: 0,
    description: '',
  };
  const COL_SECRET = {
    id: 'var-col-secret',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'apiKey',
    value: '',
    isSecret: true,
    sortOrder: 0,
    description: '',
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [ENV_VAR],
    },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [COL_SECRET],
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="collection-row"][data-id="col-1"]')).toBeVisible();
  await page
    .locator('[data-testid="collection-row"][data-id="col-1"]')
    .locator('[data-testid="tree-twisty"]')
    .click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  await page.click('[data-testid="http-variables-overview-toggle"]');
  await expect(page.locator('[data-testid="variables-overview"]')).toBeVisible();

  const rows = page.locator('[data-testid="variables-overview-row"]');
  await expect(rows).toHaveCount(2);
  await expect(
    page.locator('[data-testid="variables-overview-row"][data-scope="environment"]'),
  ).toHaveCount(1);
  await expect(
    page.locator('[data-testid="variables-overview-row"][data-scope="collection"]'),
  ).toHaveCount(1);
  await expect(page.locator('[data-testid="variables-overview-secret"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="variables-overview-value"]')).toContainText('us-east');

  await page.click('[data-testid="variables-overview-edit-environment"]');
  await expect(
    page.locator('[data-testid="variables-dialog"][data-scope="environment"]'),
  ).toBeVisible();
});

// P18 D10/D12: gRPC parity with the URL-field (:300) and header-name (:357) cases above — closing
// F15 (no {{variable}} colouring anywhere in gRPC) and P15b #7 (no metadata-name completion).

const GRPC_P18_ENV = { id: 'env-p18', name: 'Prod', sortOrder: 0, isActive: true };
const GRPC_P18_BASE_URL_VAR = {
  id: 'var-target',
  scope: 'environment',
  ownerId: 'env-p18',
  name: 'target',
  value: 'demo.example.com:443',
  isSecret: false,
  sortOrder: 0,
};

function grpcP18VariableControl(): ControlSnapshot[] {
  return [
    { channel: IPC.variablesListEnvironments, response: [GRPC_P18_ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-p18' },
      response: [GRPC_P18_BASE_URL_VAR],
    },
  ];
}

test('the gRPC target paints a resolved reference and an unknown one differently (P18 D10)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({})] },
    ...grpcP18VariableControl(),
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.fill('[data-testid="grpc-target"]', '{{target}}/{{not_defined}}');

  const overlay = page.locator('.grpc-target-field .highlight-overlay');
  await expect(overlay.locator('.kira-ed-var')).toHaveCount(1);
  await expect(overlay.locator('.kira-ed-var-unknown')).toHaveCount(1);
});

test('a metadata name cell suggests grpc-timeout (P18 D12)', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [grpcTab({})] }];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-request-pane-metadata"]');
  const nameInput = page.locator('[data-testid="grpc-metadata-name"]').first();
  await nameInput.click();
  await nameInput.pressSequentially('grpc-time');
  const suggestions = page.locator('.autocomplete-suggestions li');
  await expect(suggestions.filter({ hasText: 'grpc-timeout' })).toBeVisible();
  await page.keyboard.press('Tab');
  await expect(nameInput).toHaveValue('grpc-timeout');
});

// P18 D19: the environment select is app-drawn now (P17 D18's precedent) — a colour dot on the
// closed state and per row, and the closed height/border/padding unchanged (P16 D6, extending the
// http-method-select/http-url case at :424).

const GRPC_ENV_COLOR_A = {
  id: 'env-a',
  name: 'Prod',
  sortOrder: 0,
  isActive: true,
  description: '',
  color: 'blue',
};
const GRPC_ENV_COLOR_B = {
  id: 'env-b',
  name: 'Staging',
  sortOrder: 1,
  isActive: false,
  description: '',
  color: 'none',
};

test('the environment select opens an app-drawn menu carrying each environment’s colour, and its closed height matches the controls beside it (P18 D19)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    { channel: IPC.variablesListEnvironments, response: [GRPC_ENV_COLOR_A, GRPC_ENV_COLOR_B] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const select = page.locator('[data-testid="api-environment-select"]');
  await expect(select).toBeVisible();
  await expect(select.locator('[data-testid="conn-dot"]')).toBeVisible();

  // P16 D6's own rule, extended: the closed control's height matches its neighbours in the same
  // toolbar row, unaffected by the native-<select> -> app-drawn-<button> swap.
  const selectHeight = await select.evaluate((el) => (el as HTMLElement).offsetHeight);
  const urlBoxHeight = await page
    .locator('.p-input:has([data-testid="http-url"])')
    .evaluate((el) => (el as HTMLElement).offsetHeight);
  expect(selectHeight).toBe(urlBoxHeight);

  await select.click();
  const menu = page.locator('[data-testid="api-environment-menu"]');
  await expect(menu).toBeVisible();

  // The trigger sits flush against its own toolbar's right edge (`.p-push`), so the popover's own
  // right edge is pinned to *this control's* right edge (anchor="right") — not, as it used to be,
  // clamped against the viewport's right edge by shift() overriding a requested-but-unreachable
  // `anchor="left"`. Those two coincide at this test's own 1440px viewport width (the trigger
  // sits only ~15px from the edge), so this asserts the actual mechanism — flush with the
  // trigger — rather than a coincidence that would silently stop holding at a different width.
  const selectBox = await select.boundingBox();
  const menuBox = await menu.boundingBox();
  if (!selectBox || !menuBox) throw new Error('api-environment-select or its menu has no box');
  expect(Math.round(menuBox.x + menuBox.width)).toBe(Math.round(selectBox.x + selectBox.width));

  const noneRow = page.locator('[data-testid="api-environment-option-none"]');
  await expect(noneRow.locator('[data-testid="conn-dot"]')).toHaveClass(/bg-none/);

  const rows = page.locator('[data-testid="api-environment-option"]');
  await expect(rows).toHaveCount(2);
  const prodRow = rows.filter({ hasText: 'Prod' });
  const stagingRow = rows.filter({ hasText: 'Staging' });
  await expect(prodRow.locator('[data-testid="conn-dot"]')).not.toHaveClass(/none/);
  await expect(stagingRow.locator('[data-testid="conn-dot"]')).toHaveClass(/none/);

  await prodRow.click();
  await expect(menu).toHaveCount(0);
  await expect(select).toHaveAttribute('data-value', 'env-a');
});

// P18 D17: an environment's colour reaches the request view's toolbar cap and head dot, and
// changes live when the active environment changes — LAW 07's rail/dot, the exact analogue of a
// query console's own connection indicator, filling the slot F20 #6 found reserved-but-empty.

test('an environment’s colour reaches the request view’s toolbar cap and head dot, and changes when the active environment changes (P18 D17)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [httpRequestTab('tab-1', 0, true, {})] },
    // Two answers, consumed in order: env-a active (initial mount), then env-b active (the
    // reload setActiveEnvironment triggers after the switch below) — the same "list re-fetched
    // after every mutation" shape variablesSetActiveEnvironment/loadEnvironments already has.
    { channel: IPC.variablesListEnvironments, response: [GRPC_ENV_COLOR_A, GRPC_ENV_COLOR_B] },
    {
      channel: IPC.variablesListEnvironments,
      response: [
        { ...GRPC_ENV_COLOR_A, isActive: false },
        { ...GRPC_ENV_COLOR_B, isActive: true },
      ],
    },
    { channel: IPC.variablesSetActiveEnvironment, args: { id: 'env-b' }, response: null },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const rail = page.locator('[data-testid="http-request-view"] [data-testid="toolbar-rail"]');
  await expect(rail).toHaveClass(/\bbg-conn-blue\b/);

  const headDot = page.locator('[data-testid="view-head"] [data-testid="conn-dot"]');
  await expect(headDot).not.toHaveClass(/none/);
  await expect(headDot).toHaveClass(/\bbg-conn-blue\b/);

  await page.click('[data-testid="api-environment-select"]');
  await page.click('[data-testid="api-environment-option"][data-value="env-b"]');

  await expect(headDot).toHaveClass(/none/);
});

// One row grid, three consumers — VariableSetView's rows used to be independent
// flex items with their own per-field `flex` value, so a secret row's extra reveal affordance
// shifted its neighbours out of alignment. This guards the fix directly: three consecutive rows,
// one of them a secret, share left edges column by column.
test('a variable-set view: name/value/description cells of three consecutive rows share left edges (D9)', async ({
  relaunch,
}) => {
  const VARS = [
    {
      id: 'var-1',
      scope: 'collection',
      ownerId: 'col-1',
      name: 'a',
      value: '1',
      isSecret: false,
      sortOrder: 0,
    },
    {
      id: 'var-2',
      scope: 'collection',
      ownerId: 'col-1',
      name: 'b',
      value: '',
      isSecret: true,
      sortOrder: 1,
    },
    {
      id: 'var-3',
      scope: 'collection',
      ownerId: 'col-1',
      name: 'c',
      value: '3',
      isSecret: false,
      sortOrder: 2,
    },
  ];
  const VARIABLE_SET_TAB = {
    id: 'tab-vs-1',
    connectionId: null,
    path: 'variables:collection:col-1',
    kind: 'variable-set',
    order: 0,
    active: true,
    state: { scope: 'collection', ownerId: 'col-1', name: 'Orders API' },
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [VARIABLE_SET_TAB] },
    {
      channel: IPC.collectionsList,
      response: {
        collections: [
          {
            id: 'col-1',
            name: 'Orders API',
            sortOrder: 0,
            createdAt: '2026-01-01T00:00:00.000Z',
            updatedAt: '2026-01-01T00:00:00.000Z',
          },
        ],
        items: [],
      },
    },
    { channel: IPC.variablesListEnvironments, response: [] },
    { channel: IPC.variablesList, args: { scope: 'collection', ownerId: 'col-1' }, response: VARS },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const view = page.locator('[data-testid="variables-dialog"][data-scope="collection"]');
  await expect(view).toBeVisible();
  const rows = view.locator('[data-testid="variable-row"]');
  await expect(rows).toHaveCount(3);

  async function cellX(rowIndex: number, cellClass: string): Promise<number> {
    const box = await rows.nth(rowIndex).locator(cellClass).boundingBox();
    if (!box) throw new Error(`row ${rowIndex} ${cellClass} has no box`);
    return box.x;
  }

  for (const cellClass of ['.name-cell', '.value-cell', '.description-cell']) {
    const [x0, x1, x2] = await Promise.all([
      cellX(0, cellClass),
      cellX(1, cellClass),
      cellX(2, cellClass),
    ]);
    expect(x1).toBe(x0);
    expect(x2).toBe(x0);
  }
});
