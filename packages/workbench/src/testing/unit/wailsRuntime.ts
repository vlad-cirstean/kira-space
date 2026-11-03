import { mock } from 'bun:test';
import { createFakeSocket, type FakeSocket } from './fakeSocket';

// The one and only mock.module registration for '/wails/runtime.js' across the whole tests/unit
// run — P57's counterpart to window.ts's window.kira stub, and for the same reason: Bun's module
// registry is shared across every spec file in a single run, so two different mock.module calls
// for the same specifier race for whichever wins (window.ts's own comment documents the
// window.kira version of this, found the hard way in P43 iteration 3). Every spec whose import
// chain reaches bridge/port.ts or bridge/control.ts — directly, or via data.ts /
// workbench/state/engine.ts — needs this to resolve, not only specs that test the transport
// itself: control.ts's own top-level imports pull in every generated service binding (each of
// which imports `Call` from this same specifier) the moment control.ts is loaded at all.
//
// This is a mock.module call, not a tsconfig "paths" redirect, deliberately: a "paths" entry for
// this exact specifier — tried first — makes Bun's own module resolver hijack it before
// mock.module ever gets a chance, in every file that transitively reaches it, not only the one
// declaring the mapping (confirmed by experiment: pointing "paths" at this very file, a real and
// otherwise-loadable module, still produced "Cannot find module" from bridge/port.ts). Since
// TypeScript also forbids an ambient `declare module` for a path-like specifier ("Ambient module
// declaration cannot specify relative module name"), so port.ts's and control.ts's own imports
// each carry a suppression directive instead — see their comments.
//
// Most specs never touch any of this: they override `data`'s or `control`'s own methods above
// the transport (view-state.spec.ts's `(data as any).read = ...`), so `Call`/`Events` only need
// to exist as real named exports, never actually get invoked, and don't need to do anything
// useful. bridge-port.spec.ts is the one spec that drives the Stream transport directly.
//
// port.ts's `const socket = Stream('engine')` runs once, ever, for the whole test process (Bun's
// module registry caches port.ts itself across every spec file that imports it, whichever comes
// first in bun test's own file-discovery order — not necessarily, and empirically not always,
// alphabetical or CLI-argument order). An earlier design here tried to *pre-register* the fake
// socket bridge-port.spec.ts wanted (`setSocketFactory` before its own dynamic import), betting
// that its dynamic `import('.../bridge/port')` would always be the first thing in the whole run to
// evaluate port.ts's module body. That bet is exactly the race: any other spec whose import chain
// reaches port.ts first (bridge/data.ts, workbench/state/engine.ts) locks in a *different* fake
// socket — created by whatever factory was installed at that earlier moment — and
// bridge-port.spec.ts's later `setSocketFactory` call is too late to matter, since port.ts's
// module-scope `Stream('engine')` never runs a second time. The test then drives a socket object
// port.ts never sees, so every response/open/close it injects is silently inert (a request that
// never gets `send()`'d "isn't sent" only because nothing reaches the real transport at all, and
// `ready` times out instead of resolving) — which reads as 5 s test timeouts and JSON-parsing
// garbage a request or two later, depending on exactly which tests happened to still have pending
// promises when the mismatch set in. `sigma-count-refresh.spec.ts`'s filename-note comment records
// one attempt to dodge this by picking a spec name that empirically sorted the "right" way — a
// naming hack, not a fix, and it stopped working the moment another spec's name (or bun's own
// discovery order) shifted again.
//
// The actual fix: stop betting on load order entirely. `Stream(name)` memoizes one socket per
// name the first time it's asked for that name, by *whichever* caller asks first — a later call
// for the same name is a no-op that just returns the same instance, by design, since the
// module-scope call it would need to affect has already happened. `getStream(name)` (below) is
// that same lookup, exposed so a spec can ask for the real transport's socket *after* importing
// port.ts — for whatever socket port.ts actually ended up holding, rather than trying to inject
// one in advance and hoping to win a race it has no way to guarantee winning.
const streams = new Map<string, FakeSocket>();

export function Stream(name: string): FakeSocket {
  let socket = streams.get(name);
  if (!socket) {
    socket = createFakeSocket();
    streams.set(name, socket);
  }
  return socket;
}

/**
 * Returns the fake socket a name's `Stream(name)` call resolved to — creating and
 * memoizing one if nothing has called `Stream(name)` yet. Since `Stream` memoizes per name
 * (above), this is always the exact same object port.ts's own module-scope
 * `const socket = Stream('engine')` is holding, regardless of which spec file's import chain
 * triggered that call first.
 */
export function getStream(name: string): FakeSocket {
  return Stream(name);
}

// Deliberately never settles, rather than rejecting: state/tabs.ts's own module-scope code fires
// an unawaited `control.tabsSave(...)` on tab mutation with no `.catch`, and other state modules
// have similar fire-and-forget calls no spec here is testing — a rejection would surface as an
// unhandled promise rejection attributed to whatever spec happened to be running, even though
// every spec that cares about a bound call's result overrides `control`'s or `data`'s
// own method first (view-state.spec.ts's `(control as any).treeChildren = ...`). A call the test
// genuinely awaits without overriding would hang, which is the correct, loud failure mode instead.
function notImplemented(_what: string): Promise<never> {
  return new Promise(() => {});
}

let callFactory: (...args: unknown[]) => Promise<unknown> = () => notImplemented('Call');

// bridge-unwrap.spec.ts is the one spec that drives a real bound call end to end (to prove every
// control.ts method does run its result through `unwrap`); it calls this before importing
// control.ts and `resetCallFactory()` afterwards, so a later spec's own fire-and-forget calls
// (state/tabs.ts's unawaited `control.tabsSave(...)`) go back to hanging harmlessly instead of
// rejecting into an unrelated spec.
export function setCallFactory(f: (...args: unknown[]) => Promise<unknown>): void {
  callFactory = f;
}
export function resetCallFactory(): void {
  callFactory = () => notImplemented('Call');
}

// The generated bindings do `import { Call as $Call } from '/wails/runtime.js'` then call
// `$Call.ByName(...)` — the real bundled runtime attaches ByName/ByID onto the Call function
// object itself, so the stub mirrors that shape rather than exporting them as siblings.
function Call(...args: unknown[]): Promise<unknown> {
  return callFactory(...args);
}
Call.ByName = (...args: unknown[]): Promise<unknown> => callFactory(...args);
Call.ByID = (...args: unknown[]): Promise<unknown> => callFactory(...args);

const eventHandlers = new Map<string, Set<(event: { data: unknown }) => void>>();

/** Delivers a Go-pushed event to every `Events.On` subscriber of `name`. */
export function emitWailsEvent(name: string, data: unknown): void {
  for (const handler of [...(eventHandlers.get(name) ?? [])]) handler({ data });
}

const Events = {
  On: (name: string, handler: (event: { data: unknown }) => void): (() => void) => {
    let handlers = eventHandlers.get(name);
    if (!handlers) {
      handlers = new Set();
      eventHandlers.set(name, handlers);
    }
    handlers.add(handler);
    return () => {
      handlers.delete(handler);
    };
  },
};

mock.module('/wails/runtime.js', () => ({ Stream, Call, Events }));
