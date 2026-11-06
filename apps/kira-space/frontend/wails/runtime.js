// Dev-server stand-in for the runtime bundle Wails itself serves at the URL `/wails/runtime.js`.
//
// Every generated binding, plus src/bridge/{control,port}.ts, imports that literal absolute URL
// (the bindings are generated with `-b`, and the URL is what a packaged app answers:
// pkg/application/application.go serves `runtimeJSWithPrelude()` there). Vite's import analysis
// still has to resolve the specifier at transform time, and it resolves a leading-slash specifier
// against the Vite root - this file's own directory - so putting a real file at exactly that path
// is what makes `vite dev` load the app instead of erroring with "Failed to resolve import
// /wails/runtime.js". Because the resolved file sits under the root, the URL Vite rewrites the
// import to is unchanged: still `/wails/runtime.js`.
//
// That matters, because under `wails3 dev` the webview talks to the app's own asset server, which
// answers /wails/* itself and proxies only everything else to Vite. So the running app never
// fetches this file - it gets Wails' own bundle, the same one the UI tests mount (see
// tests/ui/support/mockRuntime.ts). This body is the fallback for the other case: the dev server
// opened in a plain browser, with no app behind it. The npm package is the same runtime,
// unbundled, at the version go.mod pins.
//
// The production build never reaches this file at all: vite.config.ts marks /wails/* external, so
// the built bundle keeps the literal import for Wails to satisfy at runtime.
export {
  Android,
  Application,
  Browser,
  Call,
  CancelError,
  CancellablePromise,
  CancelledRejectionError,
  Clipboard,
  Create,
  clientId,
  Dialogs,
  Events,
  Flags,
  getTransport,
  IOS,
  JSONStream,
  loadOptionalScript,
  objectNames,
  Screens,
  Stream,
  System,
  setTransport,
  Updater,
  WailsSocket,
  Window,
  WML,
} from '@wailsio/runtime';
