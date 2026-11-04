// The one store the grid header menu (menu.ts)
// and the grid preview (maskPreview.ts, via SlickGridHost.vue) all share — mirrors
// state/schemas.ts's own shape. No second copy in the dialog's own draft: a rule takes effect
// immediately (it is not part of the connection's save/cancel draft), matching how the header
// menu writes one.
//
// The rule list and the masked-column counts both live in TanStack Query's cache
// (queryKeys below), not in a `reactive` — a connectionId absent from the `['maskRules', id]`
// cache means "never loaded" (the same "absent until loaded" convention schemas.ts
// uses); every real caller now goes through a `useQuery` (SlickGridHost.vue)
// whose own `enabled`/auto-fetch subsumes the old explicit "loaded?" check.

import type { MaskRule, MaskRuleFields } from '@shared/domain/mask';
import type { MaskRulesChangedEvent } from '@shared/protocol/events';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import { control } from '../bridge/control';

export function maskRulesQueryKey(connectionId: string): readonly ['maskRules', string] {
  return ['maskRules', connectionId] as const;
}

export const maskRuleCountsQueryKey = ['maskRuleCounts'] as const;

export function maskRulesFor(connectionId: string): MaskRule[] {
  return queryClient.getQueryData<MaskRule[]>(maskRulesQueryKey(connectionId)) ?? [];
}

export async function loadMaskRules(connectionId: string): Promise<MaskRule[]> {
  const rules = await control.maskRulesList(connectionId);
  queryClient.setQueryData(maskRulesQueryKey(connectionId), rules);
  // Kept in lockstep with the full list — in the same synchronous write, not re-fetched from
  // Counts() separately — the toolbar's own `hasMaskRules` (DataToolbar.vue) reads this map, and a
  // rule just written from the header menu (menu.ts's markColumnMaskKind) must make that toggle
  // appear in the same tick it also calls setMaskPreview(tabId, true), not on the next full
  // counts reload.
  queryClient.setQueryData<Record<string, number>>(maskRuleCountsQueryKey, (old) => ({
    ...old,
    [connectionId]: rules.length,
  }));
  return rules;
}

// upsert/remove re-call loadMaskRules synchronously (not a `useMutation` +
// `invalidateQueries`) — a caller reads maskRulesFor/hasMaskRules right after either resolves
// (menu.ts's own existingMaskRule scan), and
// `invalidateQueries` alone only refetches an *active* `useQuery` observer, which not every caller
// has one of. Matches state/schemas.ts's own saveDdl: a synchronous cache write, not a fire-and-
// hope invalidate.
export async function upsertMaskRule(
  connectionId: string,
  fields: MaskRuleFields,
): Promise<MaskRule> {
  const rule = await control.maskRulesUpsert(connectionId, fields);
  await loadMaskRules(connectionId);
  // A rule can start or stop correlating on this same edit — the cached key (if any) is still
  // valid either way (keys are never invalidated by a rule edit, only by an explicit
  // regenerate), so it is deliberately left alone here.
  return rule;
}

export async function removeMaskRule(connectionId: string, id: string): Promise<void> {
  await control.maskRulesDelete(id);
  await loadMaskRules(connectionId);
}

export async function regenerateMaskKey(connectionId: string): Promise<void> {
  await control.maskRulesRegenerateKey(connectionId);
  // Force the next correlationKeyFor to refetch: every existing tag is now stale.
  invalidateCorrelationKey(connectionId);
  // The grid's masking preview watches this query's data identity; a shallow copy re-runs it so a
  // tab keeps no tag computed under the old key.
  const rules = queryClient.getQueryData<MaskRule[]>(maskRulesQueryKey(connectionId));
  if (rules) queryClient.setQueryData(maskRulesQueryKey(connectionId), [...rules]);
}

// A plain memoised cache, not TanStack Query — never read reactively by any component (only
// correlationKeyFor below reads it), so it needs none of Query's cache/invalidation machinery.
const correlationKeys: Record<string, string> = {};
// Bumped on every key rotation: a correlationKeyFor fetch that started before one must not write
// its (now stale) key back.
const keyGeneration: Record<string, number> = {};

function invalidateCorrelationKey(connectionId: string): void {
  keyGeneration[connectionId] = (keyGeneration[connectionId] ?? 0) + 1;
  delete correlationKeys[connectionId];
}

function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

/** Resolves connectionId's own correlation key, fetching (and caching) it on first use.
 *  `null` means the connection currently needs no key (no correlating rule) — a real miss, not a
 *  loading state; the grid preview's own tag cache degrades to "no tag" for every value in that
 *  case, per maskPreview.ts's own fail-closed comment. */
export async function correlationKeyFor(connectionId: string): Promise<Uint8Array | null> {
  let hex = correlationKeys[connectionId];
  if (hex === undefined) {
    const generation = keyGeneration[connectionId] ?? 0;
    hex = await control.maskRulesCorrelationKey(connectionId);
    if ((keyGeneration[connectionId] ?? 0) === generation) correlationKeys[connectionId] = hex;
  }
  return hex ? hexToBytes(hex) : null;
}

/** The Settings glance's own backend (§7.5) — every connection id with at least one rule, mapped
 *  to its rule count, loaded in one batch call. */
export async function loadMaskRuleCounts(): Promise<Record<string, number>> {
  const counts = await control.maskRulesCounts();
  queryClient.setQueryData(maskRuleCountsQueryKey, counts);
  return counts;
}

// The push already carries the fresh rule list (bridge/maskrules.go's own
// broadcastRules re-lists before emitting), so this writes straight into the cache — loadMaskRules'
// own two-cache-write shape (rules + counts, kept in lockstep), from a pushed payload instead
// of an IPC round trip. No writeGeneration guard (state/schemas.ts's own fuller shape): this module
// has no local draft to race against a fetch already in flight — a rule takes effect immediately
// (this file's own header comment), so there is nothing an incoming push here could ever clobber.
function applyRemote(event: MaskRulesChangedEvent): void {
  queryClient.setQueryData(maskRulesQueryKey(event.connectionId), event.rules);
  queryClient.setQueryData<Record<string, number>>(maskRuleCountsQueryKey, (old) => ({
    ...old,
    [event.connectionId]: event.rules.length,
  }));
  // A key rotation elsewhere makes this window's
  // own cached correlation key wrong (still importable, no longer the key run_query's render
  // path or a fresh CorrelationKey call would return), so drop it — the next correlationKeyFor call
  // re-fetches the new one, same as a local regenerate already forces for the window that ran it.
  if (event.keyRegenerated) invalidateCorrelationKey(event.connectionId);
}

let unsubscribeChanged: (() => void) | null = null;
let unsubscribeConnectionsChanged: (() => void) | null = null;

/** main.ts's boot sequence, beside initSchemaSync — live before any Privacy tab, header menu or
 *  grid preview ever mounts, whether or not one does this session (initSchemaSync's own precedent).
 *  Without this, a rule added/removed/regenerated in one window never reached any
 *  other window's own `['maskRules', connectionId]` cache — the Settings glance's counts, the
 *  Privacy tab's list and the grid preview's tags all stayed stale until something unrelated
 *  happened to refetch them. */
export function initMaskRulesSync(): void {
  // One entry per connection plus one counts map; the connectionsChanged listener below removes a
  // deleted connection's entry, so keeping them while unobserved stays bounded.
  queryClient.setQueryDefaults(['maskRules'], { gcTime: KEEP_WHILE_OPEN });
  queryClient.setQueryDefaults(maskRuleCountsQueryKey, { gcTime: KEEP_WHILE_OPEN });
  unsubscribeChanged?.();
  unsubscribeChanged = control.onMaskRulesChanged(applyRemote);

  // A deleted connection keeps no rules, counts or key (state/schemas.ts's own cleanup shape).
  unsubscribeConnectionsChanged?.();
  unsubscribeConnectionsChanged = control.onConnectionsChanged((records) => {
    const liveIds = new Set(records.map((r) => r.id));
    for (const id of Object.keys(correlationKeys)) {
      if (!liveIds.has(id)) invalidateCorrelationKey(id);
    }
    for (const query of queryClient.getQueryCache().findAll({ queryKey: ['maskRules'] })) {
      if (!liveIds.has(query.queryKey[1] as string))
        queryClient.removeQueries({ queryKey: query.queryKey });
    }
    queryClient.setQueryData<Record<string, number>>(maskRuleCountsQueryKey, (old) => {
      if (!old) return old;
      return Object.fromEntries(Object.entries(old).filter(([id]) => liveIds.has(id)));
    });
  });
}
