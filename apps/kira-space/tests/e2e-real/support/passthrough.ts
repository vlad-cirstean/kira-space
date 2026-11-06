import type { Page, Route } from '@playwright/test';
import { CHANNEL_TO_FQN } from '../../ui/support/mockRuntime';

// The inverse of tests/ui/support/mockRuntime.ts: that module
// fakes every bound call by default and answers from a fixture; this one leaves every bound call
// alone by default (`route.continue()` — the real Go backend answers) and fakes only a named
// allowlist. Server mode has no dialogs (`FilesService.ChooseSave`/`ChooseOpen` answer a real HTTP
// 422, "file dialogs not available in server mode") — a spec that needs one stubs exactly that
// method and nothing else, losing nothing `tests/e2e/s3.spec.ts`'s own dialog stub hadn't already
// given up.
//
// CHANNEL_TO_FQN is reused rather than re-derived — its values are the literal
// `$Call.ByName("…")` strings the generated bindings send over the wire; the short "Service.Method"
// names an allowlist is keyed by are recovered from those same
// values, not retyped from memory.
const FQN_BY_SHORT_NAME: Readonly<Record<string, string>> = Object.freeze(
  Object.fromEntries(
    Object.values(CHANNEL_TO_FQN).map((fqn) => [fqn.split('.').slice(-2).join('.'), fqn]),
  ),
);

export interface FakeCall {
  /** Answered verbatim as the bound call's JSON response (HTTP 200) when `error` is unset. */
  response?: unknown;
  /** Answered as a real bound-call error (HTTP 422) — the shape transport_http.go writes. */
  error?: { code: string; message: string };
}

/** Keyed by the short "Service.Method" name (e.g. `"FilesService.ChooseSave"`), matching how
 *  P57-e2e-revisit.md §3.6/§8 name them — not the full FQN the wire actually carries. */
export type PassthroughFakes = Readonly<Record<string, FakeCall>>;

interface CallRequestBody {
  object: number;
  method: number;
  args?: {
    'call-id': string;
    methodName?: string;
    args?: unknown[];
  };
}

function runtimeErrorBody(code: string, message: string): string {
  // Mirrors tests/ui/support/mockRuntime.ts's own runtimeErrorBody byte-for-byte: the exact shape
  // apps/kira-space/internal/bridge/transport_http.go's httpError writes for a bound-call error, so
  // bridge/control.ts's `unwrap` reads it the same way it would a real one — which, from this
  // route's perspective, it is (§8: server mode really does answer FilesService.* this way).
  return JSON.stringify({
    kind: 'RuntimeError',
    message: JSON.stringify({ code, message }),
    cause: { code, message },
  });
}

/**
 * Installs the one route this tier needs: `/wails/runtime` defaults to `route.continue()` — the
 * real backend answers — except for methods named in `fakes` (short "Service.Method" names),
 * which are answered locally instead. Every other request (`/wails/runtime.js`,
 * `/wails/stream/ws`, `/wails/custom.js`, the built app's own static assets) is never routed at
 * all, since the pattern below matches only the one RPC path — it always reaches the real server.
 */
export async function installPassthrough(page: Page, fakes: PassthroughFakes = {}): Promise<void> {
  const fakesByFqn = new Map<string, FakeCall>();
  for (const [shortName, fake] of Object.entries(fakes)) {
    const fqn = FQN_BY_SHORT_NAME[shortName];
    if (!fqn) {
      throw new Error(`installPassthrough: unknown method "${shortName}" — not in CHANNEL_TO_FQN`);
    }
    fakesByFqn.set(fqn, fake);
  }

  await page.route('**/wails/runtime', async (route: Route) => {
    const request = route.request();
    if (request.method() !== 'POST') {
      await route.continue();
      return;
    }

    const body = JSON.parse(request.postData() ?? '{}') as CallRequestBody;
    const methodName = body.args?.methodName;
    const fake = methodName ? fakesByFqn.get(methodName) : undefined;
    if (!fake) {
      await route.continue();
      return;
    }

    if (fake.error) {
      await route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: runtimeErrorBody(fake.error.code, fake.error.message),
      });
      return;
    }
    const responseBody = fake.response === undefined ? 'null' : JSON.stringify(fake.response);
    await route.fulfill({ status: 200, contentType: 'application/json', body: responseBody });
  });
}
