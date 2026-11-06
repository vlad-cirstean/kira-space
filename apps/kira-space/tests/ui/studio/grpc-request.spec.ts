import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { grpcTab as buildGrpcTab, modeTab } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { installFakeTimers } from './support/clock';
import { contract } from './support/contract';
import { acceptConfirm } from './support/dialogs';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';
import { CHANNEL_TO_FQN, emitWailsEvent } from './support/mockRuntime';

// Located by row id rather than by label — collections.spec.ts's own helper verbatim, over the
// collections tree's own row (`[data-testid="collection-row"]`), a different tree from
// tests/ui/support/tree.ts's `[data-testid="tree-row"]` (the Studio project tree).

// P11 §6.4: seven tests, driving both wire planes mocked (D13's own reasoning, ported verbatim
// from every other tests/ui spec) — the `Call` bound-call endpoint via installControlMocks'
// snapshot machinery, and D8's own pushed message channel via F20's new `emitWailsEvent` helper.

async function openHttpModeAndNewGrpcRequest(page: Page): Promise<void> {
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await page.click('[data-testid="new-grpc-request-start"]');
}

// A restored 'grpc-request' tab (grpcRequestTabStateSchema's own defaults, overridden per test) —
// the same "skip the UI's own build-up path and start from a known state" shortcut
// collections.spec.ts and http-request.spec.ts's third test both already use. This file's own
// tests run with the Messages pane showing by default — every other
// caller of the shared `grpcTab` defaults to History instead, so this stays a thin local wrapper
// supplying that one divergent field as `base` rather than changing the shared default.
function grpcTab(state: Record<string, unknown> = {}): Record<string, unknown> {
  return buildGrpcTab(state, { responsePane: 'messages' });
}

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

const TWO_SERVICE_SCHEMA = {
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
    {
      name: 'demo.Items',
      fullName: 'demo.Items',
      methods: [
        {
          name: 'ListItems',
          fullName: 'demo.Items/ListItems',
          clientStreaming: false,
          serverStreaming: true,
          inputType: 'demo.ListRequest',
          outputType: 'demo.Item',
          requestTemplate: '{\n  "pageSize": 0\n}',
        },
      ],
    },
  ],
  mode: 'reflection',
  warnings: [] as string[],
};

const STREAM_SCHEMA = {
  services: [
    {
      name: 'demo.Items',
      fullName: 'demo.Items',
      methods: [
        {
          name: 'ListItems',
          fullName: 'demo.Items/ListItems',
          clientStreaming: false,
          serverStreaming: true,
          inputType: 'demo.ListRequest',
          outputType: 'demo.Item',
          requestTemplate: '{\n  "pageSize": 0\n}',
        },
      ],
    },
  ],
  mode: 'reflection',
  warnings: [] as string[],
};

test('gRPC request — open a tab and browse a schema', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.grpcDescribe, response: TWO_SERVICE_SCHEMA }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewGrpcRequest(page);
  const view = page.locator('[data-testid="grpc-request-view"]');
  await expect(view).toBeVisible();

  // P22 D13 (F22): the request/response splitter draws a visible divider at rest — it used to be
  // 4px of nothing, with no line and no grab affordance until the pointer crossed it.
  await expect
    .poll(() => view.locator('.request-splitter').evaluate((el) => getComputedStyle(el).boxShadow))
    .not.toBe('none');

  // Right icon (D2: distinct from HTTP's 'globe') and, once a target exists, the right title
  // (grpcRequestTitle's own precedence falls to the target when no name/service/method is set).
  const tab = page.locator('[data-testid="tab"]');
  await expect(tab).toHaveCount(1);
  await expect(tab.locator('.codicon-symbol-interface')).toBeVisible();

  await page.fill('[data-testid="grpc-target"]', 'demo.example.com:443');
  await expect(tab).toContainText('demo.example.com:443');

  await page.click('[data-testid="grpc-request-pane-schema"]');
  const serviceList = page.locator('[data-testid="grpc-service-list"]');
  await expect(serviceList).toBeVisible();
  await expect(page.locator('[data-testid="grpc-service-name"]')).toHaveText([
    'demo.Echo',
    'demo.Items',
  ]);

  const methodRows = page.locator('[data-testid="grpc-method-row"]');
  await expect(methodRows).toHaveCount(2);
  await expect(methodRows.nth(0)).toContainText('SayHello');
  await expect(methodRows.nth(0).locator('[data-testid="grpc-method-streaming-badge"]')).toHaveText(
    'Unary',
  );
  await expect(methodRows.nth(1)).toContainText('ListItems');
  await expect(methodRows.nth(1).locator('[data-testid="grpc-method-streaming-badge"]')).toHaveText(
    'Stream',
  );
});

// P22b D10: the method select had no width rule of its own, so it shrank to its widest <option>
// label — a fraction of the address field beside it for a service with short method names. Both
// fields are flex: 1 in the same toolbar row now, so their rendered widths should track closely.
test('gRPC request — the method select is as wide as the address field beside it (D10)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.grpcDescribe, response: TWO_SERVICE_SCHEMA }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewGrpcRequest(page);
  await page.fill('[data-testid="grpc-target"]', 'demo.example.com:443');

  const target = page.locator('[data-testid="grpc-target"]');
  const methodSelect = page.locator('[data-testid="grpc-method-select"]');
  const targetBox = await target.boundingBox();
  const methodBox = await methodSelect.boundingBox();
  if (!targetBox || !methodBox) throw new Error('target/method select has no box');
  expect(Math.abs(methodBox.width - targetBox.width) / targetBox.width).toBeLessThan(0.1);
});

// GrpcRequestView.vue's own watcher used to call loadSchema immediately on every
// keystroke of the target field, with no debounce — a fast typist fired one Describe round trip
// per character. Typing character-by-character (page.keyboard.type, unlike page.fill's single
// input event) is what exercises that path.
test('gRPC request — typing the target debounces schema loads to one call', async ({
  relaunch,
}) => {
  // GrpcRequestView.vue's own constant — restated here rather than imported across the web/node
  // tsconfig boundary (this suite's existing convention, see `global.d.ts:1`-`5`).
  const SCHEMA_LOAD_DEBOUNCE_MS = 150;
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.grpcDescribe, response: UNARY_SCHEMA }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewGrpcRequest(page);
  await installFakeTimers(page); // support/clock.ts — after boot, never before
  await page.click('[data-testid="grpc-target"]');
  await page.keyboard.type('demo.example.com:443');

  const describeCalls = () => control.log().filter((e) => e.channel === IPC.grpcDescribe);
  // Time has not moved, so no wall-clock budget is being asserted here: the debounce *cannot*
  // have elapsed, however slowly the harness typed.
  expect(describeCalls()).toHaveLength(0);
  await page.clock.runFor(SCHEMA_LOAD_DEBOUNCE_MS - 1);
  expect(describeCalls()).toHaveLength(0);
  await page.clock.runFor(2);
  await expect.poll(() => describeCalls().length).toBe(1);
  expect(describeCalls()[0]?.args).toMatchObject({ target: 'demo.example.com:443' });
  // No trailing duplicate: one debounce, one call, not one-per-keystroke arriving late.
  await page.clock.runFor(2_000);
  expect(describeCalls()).toHaveLength(1);
});

test('gRPC request — choosing a method seeds the Message editor with its template', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.grpcDescribe, response: TWO_SERVICE_SCHEMA }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewGrpcRequest(page);
  await page.fill('[data-testid="grpc-target"]', 'demo.example.com:443');
  await page.click('[data-testid="grpc-request-pane-schema"]');
  await expect(page.locator('[data-testid="grpc-method-row"]')).toHaveCount(2);

  await page.locator('[data-testid="grpc-method-row"]').nth(1).click();

  // Selecting a method switches the request pane to Message (SchemaBrowser's own selectMethod)…
  await expect(page.locator('[data-testid="grpc-request-pane"]')).toContainText('pageSize');
  const editor = page.locator('[data-testid="grpc-message-editor"]');
  await expect(editor).toBeVisible();
  expect(await editorText(editor)).toBe('{\n  "pageSize": 0\n}');
  // …and the toolbar's method chip and select both reflect the chosen method.
  await expect(page.locator('[data-testid="grpc-method-chip"]')).toContainText(
    'demo.Items/ListItems',
  );
});

test('gRPC request — a unary call renders its status, message and metadata', async ({
  relaunch,
}) => {
  const RESULT = {
    code: 0,
    codeName: 'OK',
    statusMessage: '',
    elapsedMs: 12,
    header: [{ name: 'content-type', value: 'application/grpc' }],
    trailer: [{ name: 'grpc-status', value: '0' }],
    messageCount: 1,
    messageBytes: 26,
    messages: [{ seq: 0, json: '{"message":"Hello, Ada!"}', wireBytes: 26, offsetMs: 0 }],
  };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: 'demo.example.com:443',
          service: 'demo.Echo',
          method: 'SayHello',
          message: '{"name":"Ada"}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
    { channel: IPC.grpcCall, response: RESULT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();

  await page.click('[data-testid="grpc-call"]');

  const chip = page.locator('[data-testid="grpc-status-chip"]');
  await expect(chip).toContainText('OK (0)');
  await expect(chip).toHaveClass(/ok/);

  // A unary call's single message is auto-expanded (D14).
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(1);
  const messageBody = page.locator('[data-testid="grpc-message-entry"]');
  expect(await editorText(messageBody)).toContain('Hello, Ada!');

  // Both header and trailer groups render (F6).
  await page.click('[data-testid="grpc-response-pane-metadata"]');
  const metadata = page.locator('[data-testid="grpc-response-metadata"]');
  await expect(metadata).toContainText('content-type');
  await expect(metadata).toContainText('application/grpc');
  await expect(metadata).toContainText('grpc-status');

  // P22b D14: the response pane gets HTTP's own find-in-body (P16 D11) — until now the gRPC
  // sibling had none. Scoped to the selected message's own JSON, through the same
  // editor/findRanges.ts rangeHighlights seam.
  await page.click('[data-testid="grpc-response-pane-messages"]');
  await page.click('[data-testid="grpc-find-toggle"]');
  const findBar = page.locator('[data-testid="http-find-bar"]');
  await expect(findBar).toBeVisible();
  await page.fill('[data-testid="http-find-input"]', 'Ada');
  await expect(page.locator('[data-testid="http-find-count"]')).toContainText('1 of 1');
  await expect(messageBody.locator('.kira-ed-find-match-current')).toHaveCount(1);

  await page.keyboard.press('Escape');
  await expect(findBar).toHaveCount(0);
  await expect(page.locator('[data-testid="grpc-find-toggle"]')).not.toHaveClass(/is-active/);
});

test('gRPC request — a non-OK status is a result, not an error', async ({ relaunch }) => {
  const RESULT = {
    code: 7,
    codeName: 'PermissionDenied',
    statusMessage: 'missing bearer token',
    elapsedMs: 3,
    header: [],
    trailer: [],
    messageCount: 0,
    messageBytes: 0,
  };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: 'demo.example.com:443',
          service: 'demo.Echo',
          method: 'SayHello',
          message: '{}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
    { channel: IPC.grpcCall, response: RESULT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await page.click('[data-testid="grpc-call"]');

  const chip = page.locator('[data-testid="grpc-status-chip"]');
  await expect(chip).toContainText('PermissionDenied (7)');
  await expect(chip).toHaveClass(/err/);
  // D16's central claim: a non-OK status never renders as a MessageStrip error.
  await expect(page.locator('[data-testid="grpc-call-error"]')).toHaveCount(0);
});

/** Holds the `GrpcService.Call` bound call open forever, exactly the way the real backend's
 *  own `runServerStream` blocks for the life of the stream — every UI update for a streaming
 *  test comes from `emitWailsEvent`'s own push channel instead, never from this promise settling.
 *  Registered after `relaunch()` so it is the most-recently-added `page.route` handler for
 *  `/wails/runtime` and therefore wins (Playwright evaluates routes in reverse registration order);
 *  every other bound call falls back to `installControlMocks`'s own handler untouched. */
async function holdGrpcCallPending(page: Page): Promise<() => string | undefined> {
  let opId: string | undefined;
  await page.route('**/wails/runtime', async (route, request) => {
    if (request.method() !== 'POST') {
      await route.fallback();
      return;
    }
    const body = JSON.parse(request.postData() ?? '{}') as {
      args?: { methodName?: string; args?: [{ opId?: string }] };
    };
    if (body.args?.methodName === CHANNEL_TO_FQN[IPC.grpcCall]) {
      opId = body.args.args?.[0]?.opId;
      return;
    }
    await route.fallback();
  });
  return () => opId;
}

test('gRPC request — a server-streaming call appends messages as they arrive', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: 'demo.example.com:443',
          service: 'demo.Items',
          method: 'ListItems',
          message: '{"pageSize":10}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: STREAM_SCHEMA },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  const getOpId = await holdGrpcCallPending(page);

  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await page.click('[data-testid="grpc-call"]');
  await expect.poll(getOpId).toBeTruthy();
  const callId = getOpId() as string;

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: 0,
    messages: [
      { seq: 0, json: '{"item":"a"}', wireBytes: 14, offsetMs: 4 },
      { seq: 1, json: '{"item":"b"}', wireBytes: 14, offsetMs: 11 },
    ],
    done: false,
  });
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(2);
  await expect(page.locator('[data-testid="grpc-message-offset"]').first()).toHaveText('At 4ms');

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: 2,
    messages: [{ seq: 2, json: '{"item":"c"}', wireBytes: 14, offsetMs: 19 }],
    done: false,
  });
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(3);
  // Still running — Stop stays enabled until the terminal event.
  await expect(page.locator('[data-testid="grpc-request-stop"]')).toBeEnabled();

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: 3,
    messages: [],
    done: true,
    status: {
      code: 0,
      codeName: 'OK',
      statusMessage: '',
      elapsedMs: 25,
      header: [],
      trailer: [],
      messageCount: 3,
      messageBytes: 42,
    },
  });

  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(3);
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');
  await expect(page.locator('[data-testid="grpc-request-stop"]')).toBeDisabled();
});

// The live message list used to grow without bound and re-`reduce` its whole array
// on every single push. Sends one batch well past the 10,000-message live-view ceiling
// (state.ts's MAX_LIVE_MESSAGES) in a single event — real messages arrive one at a time, but a
// live server sending 10,000+ individual push events in a test would only slow the suite down,
// never exercise a code path this batch doesn't already cover identically.
test('gRPC request — the live message list caps at 10,000 and shows the true total', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: 'demo.example.com:443',
          service: 'demo.Items',
          method: 'ListItems',
          message: '{"pageSize":10}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: STREAM_SCHEMA },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  const getOpId = await holdGrpcCallPending(page);

  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await page.click('[data-testid="grpc-call"]');
  await expect.poll(getOpId).toBeTruthy();
  const callId = getOpId() as string;

  const total = 10_037;
  const batch = Array.from({ length: total }, (_, i) => ({
    seq: i,
    json: `{"i":${i}}`,
    wireBytes: 10,
    offsetMs: i,
  }));
  await emitWailsEvent(page, IPC.grpcCall, { callId, seq: 0, messages: batch, done: false });

  await expect(page.locator('[data-testid="grpc-live-messages-elided"]')).toHaveText(
    `Showing the most recent 10,000 of ${total.toLocaleString('en-US')} messages.`,
  );
  // The oldest 37 messages were dropped, not the newest — the ones a live user is watching arrive.
  await expect(page.locator('[data-testid="grpc-message-offset"]').first()).toHaveText('At 37ms');

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: total,
    messages: [],
    done: true,
    status: {
      code: 0,
      codeName: 'OK',
      statusMessage: '',
      elapsedMs: total,
      header: [],
      trailer: [],
      messageCount: total,
      messageBytes: total * 10,
    },
  });

  // The true total (10,037 × 10 bytes = 100,370) is what the byte summary shows too — it is kept
  // as a running total (state.ts's rt.messageBytes), not re-derived from the now-capped array.
  // The message *count* alongside it must be the same true total, not messages.length (capped at
  // 10,000) — the old capped reading contradicted the elided-messages strip's own "10000 of 10037"
  // right below it.
  await expect(page.locator('[data-testid="grpc-message-summary"]')).toContainText(
    `${total.toLocaleString('en-US')} messages`,
  );
  await expect(page.locator('[data-testid="grpc-message-summary"]')).toContainText('98.0 KB');
  await expect(page.locator('[data-testid="grpc-live-messages-elided"]')).toHaveText(
    `Showing the most recent 10,000 of ${total.toLocaleString('en-US')} messages.`,
  );
});

test('gRPC request — Stop cancels an in-flight stream and keeps what arrived', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: 'demo.example.com:443',
          service: 'demo.Items',
          method: 'ListItems',
          message: '{"pageSize":10}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: STREAM_SCHEMA },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  const getOpId = await holdGrpcCallPending(page);

  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await page.click('[data-testid="grpc-call"]');
  await expect.poll(getOpId).toBeTruthy();
  const callId = getOpId() as string;

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: 0,
    messages: [
      { seq: 0, json: '{"item":"a"}', wireBytes: 14, offsetMs: 4 },
      { seq: 1, json: '{"item":"b"}', wireBytes: 14, offsetMs: 11 },
    ],
    done: false,
  });
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(2);

  await page.click('[data-testid="grpc-request-stop"]');
  // opsCancel is a void wildcard default (mockRuntime.ts's WILDCARD_DEFAULTS) — consumed, not
  // seeded, exactly as §6.4's own wording says.
  await expect.poll(() => control.log().some((e) => e.channel === IPC.opsCancel)).toBe(true);

  await emitWailsEvent(page, IPC.grpcCall, {
    callId,
    seq: 2,
    messages: [],
    done: true,
    error: { code: 'E_CANCELLED', message: 'the call was stopped' },
    status: {
      code: 1,
      codeName: 'Canceled',
      statusMessage: 'context canceled',
      elapsedMs: 9,
      header: [],
      trailer: [],
      messageCount: 2,
      messageBytes: 28,
    },
  });

  // D17: the messages already delivered are kept, and the sentence names the true partial count.
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(2);
  await expect(page.locator('[data-testid="grpc-stopped-strip"]')).toHaveText(
    'Stopped after 2 messages.',
  );
  await expect(page.locator('[data-testid="grpc-request-stop"]')).toBeDisabled();
});

test('gRPC request — a request in a collection opens the grpc-request tab kind, not http-request', async ({
  relaunch,
}) => {
  const NOW = '2026-01-01T00:00:00.000Z';
  const TREE_EMPTY = {
    collections: [{ id: 'col-1', name: 'Demo API', sortOrder: 0, createdAt: NOW, updatedAt: NOW }],
    items: [] as unknown[],
  };
  const CREATED_ITEM = {
    id: 'item-grpc-1',
    collectionId: 'col-1',
    parentId: null,
    kind: 'request',
    protocol: 'grpc',
    name: 'List items',
    sortOrder: 0,
    method: '',
    url: '',
    createdAt: NOW,
    updatedAt: NOW,
  };
  const TREE_WITH_ITEM = { collections: TREE_EMPTY.collections, items: [CREATED_ITEM] };
  const SAVED_REQUEST = {
    target: 'demo.example.com:443',
    tlsMode: 'tls',
    caFile: '',
    serverName: '',
    descriptorMode: 'reflection',
    protoPath: '',
    importPaths: [],
    service: 'demo.Items',
    method: 'ListItems',
    message: '{"pageSize":10}',
    metadata: [],
  };

  const SAVED_ITEM = { ...CREATED_ITEM, name: 'List items' };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE_EMPTY },
    { channel: IPC.collectionsCreateGrpcItem, response: CREATED_ITEM },
    { channel: IPC.collectionsList, response: TREE_WITH_ITEM },
    { channel: IPC.collectionsGetGrpcRequest, response: SAVED_REQUEST },
    // Opening the tab sets a non-empty target, which fires GrpcRequestView's own reflection
    // watch (D4) — seeded so the Schema pane behind the toolbar's method chip has something real
    // to have resolved, though this test asserts only the chip and the tab kind.
    { channel: IPC.grpcDescribe, response: STREAM_SCHEMA },
    { channel: IPC.collectionsSaveGrpcRequest, response: SAVED_ITEM },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await modeTab(page, 'api').click();

  // *New gRPC request* from the tree's own context menu (D12's sibling of *New request*) creates
  // the row directly; the item is a file entry and is named in the file.
  await collectionRow(page, 'col-1').click({ button: 'right' });
  await page.click('[data-testid="menu-item-new-grpc-request"]');

  const created = collectionRow(page, 'item-grpc-1');
  await expect(created).toBeVisible();
  // D12's own chip: `grpcMethodClass` gives every gRPC row the one neutral class (no per-row
  // streaming distinction — that lives in the method's own descriptor, unreachable from a
  // collection row without a live call), so this is the row's one identifying mark.
  await expect(created.locator('[data-testid="grpc-collection-chip"]')).toHaveText('gRPC');

  await expect(collectionRow(page, 'item-grpc-1')).toContainText('List items');

  await collectionRow(page, 'item-grpc-1').dblclick();
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-request-view"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="grpc-method-chip"]')).toContainText(
    'demo.Items/ListItems',
  );

  // P22b D3: the same #head-trailing move HTTP's own Save gets — to the right of the request's
  // own title, and its left edge does not move when the dirty mark appears beside the title.
  await expect(page.locator('[data-testid="grpc-dirty"]')).toHaveCount(0);
  const target = page.locator('[data-testid="grpc-request-target"]');
  const saveButton = page.locator('[data-testid="grpc-save"]');
  const targetBoxBefore = await target.boundingBox();
  const saveBoxBefore = await saveButton.boundingBox();
  if (!targetBoxBefore || !saveBoxBefore) throw new Error('target/save button has no box');
  expect(targetBoxBefore.x + targetBoxBefore.width).toBeLessThanOrEqual(saveBoxBefore.x);

  const messageEditor = page.locator('[data-testid="grpc-message-editor"] .view-lines');
  await messageEditor.click();
  await page.keyboard.press('End');
  await page.keyboard.type('0');
  await expect(page.locator('[data-testid="grpc-dirty"]')).toBeVisible();
  const saveBoxAfter = await saveButton.boundingBox();
  if (!saveBoxAfter) throw new Error('save button has no box');
  expect(saveBoxAfter.x).toBe(saveBoxBefore.x);

  await page.click('[data-testid="grpc-save"]');
  await expect(page.locator('[data-testid="grpc-dirty"]')).toHaveCount(0);
});

// P13 D15: the Beautify affordance the already-registered view.format command implied but had
// no button for — identical behaviour to RequestBodyPane.vue's own http-body-beautify.
test('gRPC request — Beautify formats the request message', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({ message: '{"a":1,"b":"two"}' })] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();

  await page.click('[data-testid="grpc-beautify"]');
  const BEAUTIFIED = '{\n  "a": 1,\n  "b": "two"\n}';
  const editor = page.locator('[data-testid="grpc-message-editor"]');
  expect(await editorText(editor)).toBe(BEAUTIFIED);
});

// The regression this migration's own undo boundary
// (`MonacoHost.vue`'s `doc` watcher — two `model.pushStackElement()` calls around
// `pushEditOperations()`, never `model.setValue()`) exists to keep fixed. A Beautify (an
// external write, same path as the `doc` prop round trip) must not merge into the undo stack
// entry a keystroke right before it left open — one Ctrl/Cmd+Z after Beautify undoes only the
// formatting, landing back on what was typed, not on the pre-typing seed.
test('gRPC request — one undo after Beautify undoes only the beautify, not the typing before it (v1.4 regression)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({ message: '{"a":1}' })] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();

  const editor = page.locator('[data-testid="grpc-message-editor"]');
  await editor.locator('.view-lines').click();
  await page.keyboard.press('End');
  await page.keyboard.press('Backspace'); // drop the seed's own closing '}' before extending it
  await page.keyboard.type(',"b":2}');
  const TYPED = '{"a":1,"b":2}';
  await expect.poll(() => editorText(editor)).toBe(TYPED);

  await page.click('[data-testid="grpc-beautify"]');
  const BEAUTIFIED = '{\n  "a": 1,\n  "b": 2\n}';
  await expect.poll(() => editorText(editor)).toBe(BEAUTIFIED);

  await editor.locator('.view-lines').click();
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+z' : 'Control+z');
  expect(await editorText(editor)).toBe(TYPED);
});

// clearGrpcHistory has been implemented and bound, reachable from nowhere until
// CallHistoryList.vue's own toolbar. The structural guard (the delete control is
// no longer a <button> nested inside a <button>) lives in api-ui-consistency.spec.ts; this test
// drives the Clear behaviour end to end.
test('gRPC request — the history pane gets a real Clear action', async ({ relaunch }) => {
  const ENTRY = {
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
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    // The response section is now a panel from tab-open, not gated on
    // hasResult || hasHistory — so ResponsePane.vue's own mount-time "does this tab have any
    // history" probe fires immediately, before either the call or the pane switch below (empty).
    // Switching to History afterwards then genuinely refetches (the call was made while
    // Messages was showing, so it went through the lazy/stale branch) — one more real list() call
    // than this fixture needed before that fix, which is why there are three answers, not two.
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({ target: 'demo.example.com:443', service: 'demo.Echo', method: 'SayHello' }),
      ],
    },
    // Pre-existing bug: a restored tab with no target never triggers
    // loadSchema's own watch (GrpcRequestView.vue's `mode === 'reflection' && !target` early
    // return), so `methodResolved` never resolves and Call stays disabled forever. Every sibling
    // test in this file that clicks Call already sets `target` + a grpcDescribe snapshot (e.g.
    // line 234's own pattern); this test never did.
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
    { channel: IPC.grpcHistoryList, args: scope, response: [ENTRY] },
    { channel: IPC.grpcHistoryClear, args: scope, response: undefined },
    { channel: IPC.grpcHistoryList, args: scope, response: [] },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-call"]');
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');

  await page.click('[data-testid="grpc-response-pane-history"]');
  const clearBtn = page.locator('[data-testid="grpc-history-clear"]');
  await expect(clearBtn).toBeEnabled();
  await expect(page.locator('[data-testid="grpc-history-row"]')).toHaveCount(1);

  await clearBtn.click();
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toHaveText(
    'Every past call of this request is removed. This can’t be undone.',
  );
  await acceptConfirm(page);

  await expect(page.locator('[data-testid="grpc-history-row"]')).toHaveCount(0);
  await expect(clearBtn).toBeDisabled();

  expect(control.log().filter((e) => e.channel === IPC.grpcHistoryClear)).toHaveLength(1);
});

// A server-streaming call's history entry used to always store zero messages (call.go's
// ServerStream never populated CallResult.Messages), so grpc-history.ts's own messagesElided field
// never carried real data. Viewing a stored streaming entry whose call produced more than the
// 100-message cap must now show the "first N of M" note the domain type's own comment always
// described but nothing rendered.
test('gRPC request — a stored streaming history entry with elided messages shows a note', async ({
  relaunch,
}) => {
  const ENTRY = {
    id: 'call-elided-1',
    itemId: null,
    tabId: 'tab-grpc-1',
    calledAt: '2026-01-01T00:00:00.000Z',
    target: 'demo.example.com:443',
    method: 'demo.Echo/ServerStream',
    streaming: 'server',
    environment: '',
    code: 0,
    codeName: 'OK',
    statusMessage: '',
    elapsedMs: 500,
    messageCount: 137,
    messageBytes: 13700,
    storedBytes: 13700,
  };
  const SNAPSHOT = {
    entry: ENTRY,
    target: 'demo.example.com:443',
    method: 'demo.Echo/ServerStream',
    streaming: 'server',
    message: '{}',
    metadata: [],
    messages: Array.from({ length: 100 }, (_, i) => ({
      seq: i,
      json: '{}',
      wireBytes: 100,
      offsetMs: i,
      truncated: false,
    })),
    messagesElided: true,
    metadataElided: true,
    header: [],
    trailer: [],
  };
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [grpcTab({ service: 'demo.Echo', method: 'ServerStream' })],
    },
    { channel: IPC.grpcHistoryList, args: scope, response: [ENTRY] },
    { channel: IPC.grpcHistoryGet, args: { id: 'call-elided-1' }, response: SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-response-pane-history"]');
  await page.click('[data-testid="grpc-history-row"]');

  await expect(page.locator('[data-testid="grpc-history-band"]')).toBeVisible();
  await expect(page.locator('[data-testid="grpc-history-messages-elided"]')).toHaveText(
    'Showing the first 100 of 137 messages.',
  );
  await expect(page.locator('[data-testid="grpc-history-metadata-elided"]')).toHaveText(
    'Metadata, headers and trailers weren’t stored because they were too large for history.',
  );
  await page.click('[data-testid="grpc-response-pane-messages"]');
  // The message list is virtualized — only the visible window renders, so
  // the data length is asserted through the status row's own summary rather than a DOM node
  // count. That summary must read the stored entry's true messageCount
  // (137) here, not the 100 messages the elided snapshot carries — otherwise it
  // contradicts the "first 100 of 137" strip right above it.
  await expect(page.locator('[data-testid="grpc-message-summary"]')).toContainText('137 messages');
});

// ---- P18: item 1 (D1/D3), item 3 (D13/D14) ----

test('gRPC request — the history list refreshes after a call made while another pane was showing (P18 D1)', async ({
  relaunch,
}) => {
  const NEWEST = {
    id: 'live-2',
    itemId: null,
    tabId: 'tab-grpc-1',
    calledAt: '2026-01-01T00:05:00.000Z',
    target: 'demo.example.com:443',
    method: 'demo.Echo/SayHello',
    streaming: 'unary',
    environment: '',
    code: 0,
    codeName: 'OK',
    statusMessage: '',
    elapsedMs: 3,
    messageCount: 1,
    messageBytes: 10,
    storedBytes: 60,
  };
  const OLDEST = {
    id: 'live-1',
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
    elapsedMs: 30,
    messageCount: 1,
    messageBytes: 10,
    storedBytes: 60,
  };
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({ target: 'demo.example.com:443', service: 'demo.Echo', method: 'SayHello' }),
      ],
    },
    // Pre-existing bug, unrelated to P112: see the Clear-action test's own comment above — a
    // restored tab with no target never triggers loadSchema, so Call stays disabled forever.
    { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
    // Same two-answer shape as http-history.spec.ts's own D1 case: the first is the response
    // pane's own mount-time "does this tab have any history" probe (empty), before either call;
    // the second is what reopening History actually fetches after both calls.
    { channel: IPC.grpcHistoryList, args: scope, response: [] },
    { channel: IPC.grpcHistoryList, args: scope, response: [NEWEST, OLDEST] },
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
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  // Both calls happen with the Messages pane showing (the default) — never opening History in
  // between, so both take the lazy branch.
  await expect(page.locator('[data-testid="grpc-response-pane-messages"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  await page.click('[data-testid="grpc-call"]');
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');
  await page.click('[data-testid="grpc-call"]');
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');

  await page.click('[data-testid="grpc-response-pane-history"]');

  const rows = page.locator('[data-testid="grpc-history-row"]');
  await expect(rows).toHaveCount(2);
});

test('gRPC request — calling while viewing a stored call shows the new one (P18 D3)', async ({
  relaunch,
}) => {
  const STORED = {
    id: 'stored-1',
    itemId: null,
    tabId: 'tab-grpc-1',
    calledAt: '2026-01-01T00:00:00.000Z',
    target: 'demo.example.com:443',
    method: 'demo.Echo/SayHello',
    streaming: 'unary',
    environment: '',
    code: 5,
    codeName: 'NotFound',
    statusMessage: 'no such user',
    elapsedMs: 8,
    messageCount: 0,
    messageBytes: 0,
    storedBytes: 60,
  };
  const STORED_SNAPSHOT = {
    entry: STORED,
    target: 'demo.example.com:443',
    method: 'demo.Echo/SayHello',
    streaming: 'unary',
    message: '{}',
    metadata: [],
    messages: [],
    messagesElided: false,
    header: [],
    trailer: [],
  };
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({ target: 'demo.example.com:443', service: 'demo.Echo', method: 'SayHello' }),
      ],
    },
    // Pre-existing bug, unrelated to P112: see the Clear-action test's own comment above — a
    // restored tab with no target never triggers loadSchema, so Call stays disabled forever.
    { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
    { channel: IPC.grpcHistoryList, args: scope, response: [STORED] },
    { channel: IPC.grpcHistoryGet, args: { id: 'stored-1' }, response: STORED_SNAPSHOT },
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
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-response-pane-history"]');
  await page.click('[data-testid="grpc-history-row"]');

  const band = page.locator('[data-testid="grpc-history-band"]');
  await expect(band).toBeVisible();
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('NotFound (5)');

  await page.click('[data-testid="grpc-call"]');

  await expect(band).toHaveCount(0);
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');
});

test('a freshly opened gRPC tab shows the response pane switcher before any call (P18 D14)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.tabsList, response: [grpcTab({})] }],
  });

  // Present from the moment the tab opens — the HTTP twin of this case (D1) at
  // api-ui-consistency.spec.ts:152, mirrored here now that F12 closed the gRPC gap.
  await expect(page.locator('[data-testid="grpc-response-pane-toggle"]')).toBeVisible();
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toHaveCount(0);

  // Each pane still owns its own empty state — switching to Metadata on a never-called tab
  // renders the group headers with no rows rather than throwing.
  await page.click('[data-testid="grpc-response-pane-metadata"]');
  await expect(page.locator('[data-testid="grpc-response-metadata"]')).toContainText(
    'No header metadata',
  );
});

test('a non-OK status shows what the code means and what the server said (P18 D13)', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({ target: 'demo.example.com:443', service: 'demo.Echo', method: 'SayHello' }),
      ],
    },
    // Pre-existing bug, unrelated to P112: see the Clear-action test's own comment above — a
    // restored tab with no target never triggers loadSchema, so Call stays disabled forever.
    { channel: IPC.grpcDescribe, response: UNARY_SCHEMA },
    {
      channel: IPC.grpcCall,
      response: {
        code: 5,
        codeName: 'NotFound',
        statusMessage: 'no user with that id',
        elapsedMs: 4,
        header: [],
        trailer: [],
        messageCount: 0,
        messageBytes: 0,
        messages: [],
      },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-call"]');

  // P28 D1: the code's meaning is the status chip's tooltip now; the server's own
  // statusMessage keeps its own line, since that is a message rather than a restatement.
  const chip = page.locator('[data-testid="grpc-status-chip"]');
  const message = page.locator('[data-testid="grpc-status-message"]');
  await expect(page.locator('[data-testid="grpc-status-hint"]')).toHaveCount(0);
  await chip.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'no such method or resource',
  );
  await expect(message).toBeVisible();
  await expect(message).toHaveText('no user with that id');
});

test('the gRPC target field fills the toolbar row (P18 D14)', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.tabsList, response: [grpcTab({})] }],
  });

  const target = page.locator('[data-testid="grpc-target"]');
  const widthAt = () => target.evaluate((el) => el.getBoundingClientRect().width);
  // The method select is now `flex: 1` too, the address field's own sibling in the same row — so
  // the two now share the window's added width roughly evenly, not "the target absorbs (almost)
  // all of it" this test originally pinned. TLS toggle/Call stay fixed either way — the exact
  // defect (a no-op `style="flex: 1"` on the target field itself, which never grew at all).
  const methodWidthAt = () =>
    page
      .locator('[data-testid="grpc-method-select"]')
      .evaluate((el) => el.getBoundingClientRect().width);
  const tlsWidthAt = () =>
    page
      .locator('[data-testid="grpc-tls-toggle"]')
      .evaluate((el) => el.getBoundingClientRect().width);

  const viewport = page.viewportSize();
  if (!viewport) throw new Error('no viewport');

  const widthBefore = await widthAt();
  const methodBefore = await methodWidthAt();
  const tlsBefore = await tlsWidthAt();

  const delta = 200;
  await page.setViewportSize({ width: viewport.width + delta, height: viewport.height });

  const widthAfter = await widthAt();
  const methodAfter = await methodWidthAt();
  const tlsAfter = await tlsWidthAt();

  // Both flex:1 fields grow; between them they account for roughly the window's own delta, and
  // neither is left flat (D10's own "they share the free space evenly" claim, not one field
  // absorbing it all and the other still stuck at its content width).
  const targetGrowth = widthAfter - widthBefore;
  const methodGrowth = methodAfter - methodBefore;
  expect(targetGrowth).toBeGreaterThan(delta * 0.2);
  expect(methodGrowth).toBeGreaterThan(delta * 0.2);
  expect(targetGrowth + methodGrowth).toBeGreaterThan(delta * 0.7);
  expect(Math.abs(tlsAfter - tlsBefore)).toBeLessThanOrEqual(1);
});

// P22b D14: HTTP's own ResponseHistoryList.vue idiom (P16 D15) — an always-visible filter above
// the list, matching method, status name and time. The gRPC sibling had none.
test('gRPC request — the call history has its own filter (D14)', async ({ relaunch }) => {
  const SAY_HELLO = {
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
    elapsedMs: 3,
    messageCount: 1,
    messageBytes: 10,
    storedBytes: 60,
  };
  const LIST_ITEMS = {
    ...SAY_HELLO,
    id: 'call-2',
    method: 'demo.Items/ListItems',
    code: 5,
    codeName: 'NOT_FOUND',
  };
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({})] },
    { channel: IPC.grpcHistoryList, args: scope, response: [SAY_HELLO, LIST_ITEMS] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-response-pane-history"]');
  const rows = page.locator('[data-testid="grpc-history-row"]');
  await expect(rows).toHaveCount(2);

  const filter = page.locator('[data-testid="grpc-history-filter"]');
  await expect(filter).toBeVisible();
  await filter.fill('ListItems');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('ListItems');

  await filter.fill('nothing matches this');
  await expect(page.locator('[data-testid="grpc-history-filter-empty"]')).toBeVisible();

  await filter.fill('');
  await expect(rows).toHaveCount(2);
});

// P22b D16: MetadataTable.vue's own copy of FieldRowsTable.vue's trailing-blank-row watcher (F18's
// own trade — see MetadataTable.vue's header comment).
test('gRPC request — a new metadata row scrolls into view when it appears (D16)', async ({
  relaunch,
}) => {
  const metadata = Array.from({ length: 30 }, (_, i) => ({
    name: `x-meta-${i}`,
    value: `v${i}`,
    enabled: true,
  }));
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({ requestPane: 'metadata', metadata })] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const table = page.locator('[data-testid="grpc-metadata-table"]');
  await expect(table).toBeVisible();
  const rows = table.locator('[data-testid="grpc-metadata-row"]');
  await expect(rows).toHaveCount(31);

  const trailing = rows.last();
  await trailing.locator('[data-testid="grpc-metadata-name"]').fill('x-new');
  await expect(rows).toHaveCount(32);

  const newTrailing = rows.last();
  const tableBox = await table.boundingBox();
  const rowBox = await newTrailing.boundingBox();
  if (!tableBox || !rowBox) throw new Error('table/row has no box');
  expect(rowBox.y).toBeGreaterThanOrEqual(tableBox.y);
  expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(tableBox.y + tableBox.height + 1);
});

// The growth guard (D16's own reasoning — see http-request.spec.ts's identical test for why this
// is a separate scenario from the one above, scrolled to the top rather than the bottom).
test('gRPC request — deleting a metadata row does not scroll (D16 guard)', async ({ relaunch }) => {
  const metadata = Array.from({ length: 30 }, (_, i) => ({
    name: `x-meta-${i}`,
    value: `v${i}`,
    enabled: true,
  }));
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [grpcTab({ requestPane: 'metadata', metadata })] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  const table = page.locator('[data-testid="grpc-metadata-table"]');
  await expect(table).toBeVisible();
  await expect(table.evaluate((el) => el.scrollTop)).resolves.toBe(0);

  const rows = table.locator('[data-testid="grpc-metadata-row"]');
  await rows
    .nth(20)
    .locator('[data-testid="grpc-metadata-remove"]')
    .evaluate((el: HTMLElement) => el.click());
  await expect(rows).toHaveCount(30);
  await expect(table.evaluate((el) => el.scrollTop)).resolves.toBe(0);
});

// ---- Contract: api-grpc (backend half: grpcflow TestDescribeReflection, TestUnaryMetadataAndStatus,
// TestServerStreamAndCancel store the same fixture) ----

interface ContractSchema {
  services: { name: string; fullName: string; methods: { name: string }[] }[];
}
interface ContractStatus {
  codeName: string;
  header: { name: string }[];
  trailer: { name: string }[];
  messages: { json: string }[];
}

const GRPC_ORIGINS = { grpc: 'flow.test:50051' };

test('gRPC contract — reflection lists every method of the service', async ({ relaunch }) => {
  const schema = contract<ContractSchema>('api-grpc', 'GrpcService.Describe');
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.grpcDescribe, response: schema }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewGrpcRequest(page);
  await page.fill('[data-testid="grpc-target"]', GRPC_ORIGINS.grpc);
  await page.click('[data-testid="grpc-request-pane-schema"]');
  await expect(page.locator('[data-testid="grpc-service-name"]')).toHaveCount(
    schema.services.length,
  );
  const methods = schema.services.flatMap((svc) => svc.methods.map((m) => m.name));
  await expect(page.locator('[data-testid="grpc-method-row"]')).toHaveCount(methods.length);
  await expect(
    page.locator('[data-testid="grpc-method-row"]').getByText('ServerStream', { exact: true }),
  ).toBeVisible();
});

test('gRPC contract — a unary call shows message, header and trailer', async ({ relaunch }) => {
  const schema = contract<ContractSchema>('api-grpc', 'GrpcService.Describe');
  const result = contract<ContractStatus>('api-grpc', 'GrpcService.Call', {
    origins: GRPC_ORIGINS,
  });
  const svc = schema.services[0];
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: GRPC_ORIGINS.grpc,
          service: svc.fullName,
          method: 'Unary',
          message: '{"text":"hello"}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: schema },
    { channel: IPC.grpcCall, response: result },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="grpc-call"]');
  await expect(page.locator('[data-testid="grpc-status-chip"]')).toContainText('OK (0)');
  const entry = page.locator('[data-testid="grpc-message-entry"]');
  await expect(entry).toHaveCount(1);
  expect(await editorText(entry)).toContain('hello');

  await page.click('[data-testid="grpc-response-pane-metadata"]');
  const metadata = page.locator('[data-testid="grpc-response-metadata"]');
  await expect(metadata).toContainText('Header');
  await expect(metadata).toContainText(result.header[0].name);
  await expect(metadata).toContainText('Trailer');
  await expect(metadata).toContainText(result.trailer[0].name);
});

test('gRPC contract — a server stream delivers messages and Stop ends it', async ({ relaunch }) => {
  const schema = contract<ContractSchema>('api-grpc', 'GrpcService.Describe');
  const message = contract<Record<string, unknown>>('api-grpc', 'event:kira:grpc:call#message');
  const cancelled = contract<Record<string, unknown>>('api-grpc', 'event:kira:grpc:call#cancelled');
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.tabsList,
      response: [
        grpcTab({
          target: GRPC_ORIGINS.grpc,
          service: schema.services[0].fullName,
          method: 'ServerStream',
          message: '{"text":"s","count":50,"intervalMs":200}',
        }),
      ],
    },
    { channel: IPC.grpcDescribe, response: schema },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  const getOpId = await holdGrpcCallPending(page);

  await page.click('[data-testid="grpc-call"]');
  await expect.poll(getOpId).toBeTruthy();
  const callId = getOpId() as string;

  await emitWailsEvent(page, IPC.grpcCall, { ...message, callId });
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="grpc-request-stop"]')).toBeEnabled();

  await page.click('[data-testid="grpc-request-stop"]');
  await expect.poll(() => control.log().some((e) => e.channel === IPC.opsCancel)).toBe(true);
  await emitWailsEvent(page, IPC.grpcCall, { ...cancelled, callId });

  await expect(page.locator('[data-testid="grpc-request-stop"]')).toBeDisabled();
  await expect(page.locator('[data-testid="grpc-stopped-strip"]')).toContainText('Stopped after');
  await expect(page.locator('[data-testid="grpc-message-entry"]')).toHaveCount(1);
});
