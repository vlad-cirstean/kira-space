// control.ts used to both define Studio's own bound-call surface and compose it with apiControl.ts
// (the Api module's own surface) into the exported `control` object — which meant apiControl.ts,
// importing on/trust/unwrap/windowKey from this same file, imported back from the file that
// imports it (round-1 review finding 19). Studio's surface and the composition now live in
// bridge/index.ts (the composition root: it imports both halves, neither half imports the other),
// and this file is a thin re-export so every existing `import { control } from '.../bridge/control'`
// call site — and anything reaching for the shared rpc.ts primitives through this path — is
// unchanged.

export { unwrap } from '@workbench/bridge/rpc';
export { control } from './index';
