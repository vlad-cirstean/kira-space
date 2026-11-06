import { createPinia } from 'pinia';

// The app's one Pinia instance, exported rather than created inline in main.ts so a unit
// test can `setActivePinia(pinia)` before touching a store defined outside a component.
//
// Byte-identical with Kira Space's own copy, but declined as a
// packages/workbench move (unlike queryClient.ts, its harmless twin) — sharing one `createPinia()`
// instance would give both apps' same-named stores (`'tabs'`, `'settings'`, ... — CLAUDE.md's own
// store-id-stability rule) the same Pinia registry, and `bun run test:unit` runs both apps' specs
// in one process: a shared instance collides those store ids there (Kira Studio's `useTabsStore()`
// and Kira Space's return the *same* registered store), even though each app's own real runtime is
// a separate bundle with no such collision. Kept per-app; see the file's twin for the same note.
export const pinia = createPinia();
