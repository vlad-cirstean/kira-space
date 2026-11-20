import './wailsRuntime';

// A shared globalThis.window stub for every tests/unit spec that needs one, imported for its
// side effect (`import '@workbench/testing/unit/window'`) rather than declared inline per spec.
//
// bridge/control.ts and bridge/port.ts no longer read `window.kira` at all — control.ts's
// module-scope dependency is now `Events`/`Stream` from '/wails/runtime.js', and state/tabs.ts's
// module-scope `control.onFlushBeforeClose(...)` call goes through that same rewritten control.ts.
// `window` itself is kept as a bare `addEventListener` stub only because some transitively-imported
// module may still probe for a DOM global at load time; no spec that imports this module asserts
// anything that reads it.
(globalThis as { window?: unknown }).window = {
  addEventListener: () => {},
};

// bridge/port.ts and bridge/control.ts each call into '/wails/runtime.js' at their own
// module scope (Stream('engine'), Events.On), so any spec whose chain reaches either now needs
// that specifier to resolve under Bun. The './wailsRuntime' import above registers the one shared
// mock.module for it; see its own comment for why it has to be the only registration in the whole
// run.

// Vite `define` replaces __KIRA_DEBUG_HOOKS__ in builds; under Bun the bare identifier must exist.
(globalThis as { __KIRA_DEBUG_HOOKS__?: boolean }).__KIRA_DEBUG_HOOKS__ = false;
