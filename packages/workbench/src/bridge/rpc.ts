// bridge/rpc.ts — the four bound-call primitives every service surface (Studio's own, in
// bridge/index.ts, and the Api module's, in apiControl.ts) shares. Split out of what used to be
// control.ts: apiControl.ts imported these from control.ts, which
// itself imported and composed apiControl.ts back in — a cycle that worked today only because
// every use sat inside a method body rather than at module-body top level, and that structurally
// inverted the intended dependency direction (the Api half should be removable without the
// Studio half caring, not the other way around). Neither half imports the other any more: both
// depend only on this file, which depends on neither.

// See bridge/port.ts's identical import for why this needs the directive below rather than the
// require-an-error kind (P57 M1/M2 finding: a tsconfig "paths" entry for this exact specifier
// breaks Bun's mock.module interception).
// biome-ignore lint/suspicious/noTsIgnore: an "unused directive" kind fails where this resolves fine (see comment above)
// @ts-ignore
import { Events } from '/wails/runtime.js';
import { windowKey as windowKeyValue } from '../lib/window';
import { toCodedError } from './codedError';

// Wails delivers a bound method's error as a RuntimeError whose .message is
// ipcerr.Error's own JSON encoding and whose .cause is that same {code, message} as an object
// (apps/kira-space/internal/bridge's ipcerr package + Wails' bindings.go/transport_http.go).
// Unwrapped once, here, so every consumer keeps reading `err.message` for display and `err.code`
// for branching.
// `details` — ipcerr.Error's own optional `json.RawMessage` field — is carried through
// the same way, parsed once here rather than pushed onto every consumer; `undefined` for every
// existing producer, which leaves the field unset (`omitempty`).
export function unwrap<T>(p: Promise<T>): Promise<T> {
  return p.catch((err: unknown) => {
    throw toCodedError(err);
  });
}

// P12 D11: exported so apiControl.ts (the module's own 39-method binding surface) can share it
// rather than a second copy — every bound call, both protocols, goes through the same
// on/trust/unwrap/windowKey.
export function on<T>(name: string, cb: (payload: T) => void): () => void {
  return Events.On(name, (ev: { data: T }) => cb(ev.data));
}

// The generated bindings type array-returning methods as `T[] | null` (a Go nil slice marshals to
// `null`), even though every backing repo/service in this codebase builds an explicit `[]T{}` or
// `make([]T, 0, ...)` and never returns nil for these. Every `r ?? []` at each call site
// keeps its own return type (plain arrays, never null) rather than
// pushing that generator conservatism onto every caller.
//
// The generated bindings also type every Go enum-like field (ConnectionSummary.kind,
// ConnectionState.status, SecretStorageStatus.backend, TreeVisibility's hiddenKinds, SavedQuery's
// kind/body, OpRecord.kind, TabRecord.kind, TreeNode.kind, ObjectMeta/ObjectDefinition.kind…) as
// plain `string`, since Go's own enum-like types don't carry a literal-union guarantee across the
// wire the way a Zod schema does. The Go value is always one of the valid members — the same
// trust boundary window.kira's Electron IPC handlers implicitly had — so `trust` is a deliberate,
// documented widen-then-narrow, not a silent bypass of a real check.
export function trust<T>(v: unknown): T {
  return v as T;
}

export const windowKey = windowKeyValue;
