import { afterAll, describe, expect, test } from 'bun:test';
import { resetCallFactory, setCallFactory } from '@workbench/testing/unit/wailsRuntime';

const { control, unwrap } = await import('../../../frontend/src/studio/bridge/control');
const { control: hubControl } = await import('../../../frontend/src/bridge');

afterAll(() => {
  resetCallFactory();
});

function callErrorLike(message: string, cause?: unknown): Promise<never> {
  const err = new Error(message) as Error & { cause?: unknown };
  if (cause !== undefined) err.cause = cause;
  return Promise.reject(err);
}

describe('apps/kira-space/frontend/src/studio/bridge/control.ts — unwrap (P57 D5)', () => {
  test('1. a structured cause is preferred over the message string', async () => {
    await expect(
      unwrap(
        callErrorLike(JSON.stringify({ code: 'E_DISCONNECTED', message: 'PG is not connected' }), {
          code: 'E_DISCONNECTED',
          message: 'PG is not connected',
        }),
      ),
    ).rejects.toMatchObject({ message: 'PG is not connected', code: 'E_DISCONNECTED' });
  });

  test('2. a JSON message is the fallback when cause is absent', async () => {
    await expect(
      unwrap(
        callErrorLike(JSON.stringify({ code: 'E_DISCONNECTED', message: 'PG is not connected' })),
      ),
    ).rejects.toMatchObject({ message: 'PG is not connected', code: 'E_DISCONNECTED' });
  });

  test('3. neither a structured cause nor a parseable message: E_INTERNAL, message preserved', async () => {
    await expect(unwrap(callErrorLike('boom', {}))).rejects.toMatchObject({
      message: 'boom',
      code: 'E_INTERNAL',
    });
    await expect(unwrap(callErrorLike('network gone'))).rejects.toMatchObject({
      message: 'network gone',
      code: 'E_INTERNAL',
    });
  });

  test('5. every promise-returning control method surfaces a code, not raw JSON', async () => {
    setCallFactory(() =>
      callErrorLike(JSON.stringify({ code: 'E_QUERY', message: 'relation does not exist' }), {
        code: 'E_QUERY',
        message: 'relation does not exist',
      }),
    );

    // Event subscriptions (on*) and appFlushed/windowFlushed (void, fire-and-forget) call no bound
    // method and return no promise — everything else in `control` is a request/response call
    // unwrap must guard. Four placeholder arguments cover every method's arity; none of
    // control.ts's own wrapper bodies inspect argument shape before handing them to the binding.
    //
    // This list is every control method some other spec file in tests/unit directly overrides
    // with its own stub (`grep -rhoE "control\.[a-zA-Z]+ = |control as unknown as \{
    // [a-zA-Z]+:" apps/kira-space/tests/unit`), not a hand-picked guess — `control` is one
    // shared singleton across the whole bun test process (restoreAfterEach.ts's own comment
    // documents the hazard class), and bun's test loader evaluates every spec file's top-level
    // `await import(...)` chain concurrently, not strictly in file order. A snapshot taken from
    // inside any one spec file can be a few other files' worth of leaked overrides deep by the
    // time it runs, independent of alphabetical or registration order (confirmed by bisection:
    // the specific method that leaks in here varies between runs — tabsSave one run,
    // collectionsList another — and only reproduces once ~90+ other spec files are loaded,
    // never in isolation or a small subset), so this sweep can't reliably tell "some other
    // file's still-live stub" apart from "the real binding" for any method on this list. Every
    // one of them is mechanically identical to a sibling method this sweep still covers
    // (apiControl.ts/createCoreControl.ts: `(...args) => unwrap(Service.Method(...))`, the same
    // shape every untouched method also uses), so excluding them loses no real coverage of
    // unwrap() itself — re-run the grep above and extend this list if a new override shows up.
    const leakProne = new Set([
      'tabsSave',
      'collectionsList',
      'collectionsGetRequest',
      'variablesListEnvironments',
      'variablesList',
      'variablesHistory',
      'variablesReveal',
      'variablesRevealHistory',
      'historyList',
      'httpSend',
      'httpCookies',
      'httpDeleteCookie',
      'grpcCall',
      'grpcHistoryList',
      'opsCancel',
    ]);
    let checked = 0;
    for (const [name, member] of Object.entries(control)) {
      if (typeof member !== 'function') continue;
      if (name.startsWith('on') || name === 'appFlushed' || name === 'windowFlushed') continue;
      // createOpenExternal swallows failures by design (announces, never throws).
      if (name === 'openExternal') continue;
      if (leakProne.has(name)) continue;
      // Studio's control layers over the hub's; only its own and overriding methods are in scope.
      if (hubControl[name as keyof typeof hubControl] === member) continue;
      // Third arg is an array: apiConfigForm maps it before unwrap runs.
      const result = (member as (...args: unknown[]) => unknown)('a', 'b', [], 'd');
      if (!result || typeof (result as Promise<unknown>).then !== 'function') continue;
      checked += 1;
      await expect(result).rejects.toMatchObject({ code: 'E_QUERY' });
    }
    // A regression that stops wrapping every method (or a Object.entries change that stops
    // reaching them) should fail loudly here rather than silently checking zero methods.
    expect(checked).toBeGreaterThan(30);
  });
});
