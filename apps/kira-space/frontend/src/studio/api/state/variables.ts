import type { PaletteColor } from '@shared/domain/color';
import type {
  ApiEnvironment,
  ApiVariable,
  ApiVariableHistoryEntry,
} from '@shared/domain/variables';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { queryClient } from '@workbench/state/queryClient';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { computed, reactive, ref, toRefs, watch } from 'vue';
import { control } from '../../bridge/control';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import { runReveal } from '../reveal';
import { openVariablesTab } from '../tabs';
import {
  apiEnvironmentsKey,
  apiEnvironmentsQueryOptions,
  loadCollectionsTree,
  loadEnvironments,
  loadVariableRows,
  reconcileEnvironments,
  refreshApiQuery,
} from './apiQueries';
import { useCollectionsStore } from './collections';
import { createRevealExpiry } from './revealExpiry';

// The environment list and the app-global active selection — read-only at this point (list and
// switch); editing, secrets, history and reordering land in later commits on this same store.
// Mirrors http/state/collections.ts's own shape: one reactive object, re-listed after every
// mutation rather than patched locally, since the list is small and this keeps the store
// impossible to get out of step with what Go stored.
//
// Split into two stores — this one (environments) and useVariableSetStore below
// (the variable-set tab's runtime, send-time cache, gated reveal and history popover). The two
// are genuinely independent: nothing here reaches into the variable-set machinery, and nothing
// there reaches back into environment CRUD.

export const useVariablesStore = defineStore('variables', () => {
  // An environment CRUD call's own failure message: a failed create/rename/delete/duplicate/
  // reorder/activate leaves the row as it was, so this tells the user why. Cleared on the next
  // successful mutation. queryClient is explicit: unit tests build this store without an app.
  const surface = useTaskAction({ queryClient });

  function dismissError(): void {
    surface.reset();
  }

  // The environments list is now TanStack Query's cache, not a store field — this observer
  // is app-lifetime (created once, with the setup store's own effect scope owning it), so
  // invalidateQueries always has an active observer to refetch. queryClient is passed explicitly
  // (not injected): main.ts creates stores before app.use(VueQueryPlugin), and unit tests mount no
  // app at all — useBaseQuery.js falls back to inject() only when no explicit client is given.
  const envQuery = useQuery(apiEnvironmentsQueryOptions(), queryClient);

  const environments = computed<ApiEnvironment[]>(() => envQuery.data.value ?? []);
  const loaded = computed(() => envQuery.data.value !== undefined);

  /** D3: the app-global selection, or null when none is active ("No environment"). */
  const activeEnvironment = computed<ApiEnvironment | null>(
    () => environments.value.find((e) => e.isActive) ?? null,
  );

  /** '' when no environment is active — the same convention SetActiveEnvironment's own id arg
   *  uses, and what control.httpSend's environmentId will carry at send time. */
  const activeEnvironmentId = computed(() => activeEnvironment.value?.id ?? '');

  /** The active environment's colour, for the request views' toolbar rail and head
   *  dot — 'none' with no environment active, the same "the rail slot stays reserved
   *  either way" treatment a colour of 'none' already gets. Always defined (never undefined), so
   *  ViewChrome's `envColor` prop always drives the dot rather than falling through to a
   *  connection's own colour, which an Api tab never has. */
  const activeEnvironmentColor = computed<PaletteColor>(
    () => activeEnvironment.value?.color ?? 'none',
  );

  /** Every list mutation below refreshes this one key then reconciles — reconcileEnvironments
   *  (F9's successor) evicts any cached environment-scope variables query whose owner no longer
   *  exists in the fresh list, local mutation or remote push alike. */
  async function afterEnvironmentsListChange(): Promise<void> {
    await refreshApiQuery(apiEnvironmentsKey);
    reconcileEnvironments();
  }

  const setActiveEnvironmentMutation = useMutation(
    {
      mutationKey: ['apiEnvironments', 'setActive'],
      mutationFn: (id: string) => control.variablesSetActiveEnvironment(id),
      onSuccess: afterEnvironmentsListChange,
    },
    queryClient,
  );

  /** id: '' selects "No environment" (D3). */
  async function setActiveEnvironment(id: string): Promise<boolean> {
    return surface.run(() => setActiveEnvironmentMutation.mutateAsync(id));
  }

  // ---- The per-tab environment override ----
  //
  // An incognito tab's own environment pick lives here, in memory, never touching
  // api_environments.is_active — switching environment is the main reason to open an incognito tab
  // at all, so the selector itself stays enabled and useful rather than disabled.
  // Absent (no entry) until the user picks one: an incognito tab inherits
  // the app-wide selection at open time, so the common case is byte-identical to today.
  const incognitoEnvByTab = new Map<string, string>(); // tabId → environment id ('' = none)

  function environmentIdForTab(tabId: string): string {
    if (useTabIncognitoStore().isIncognito(tabId) && incognitoEnvByTab.has(tabId)) {
      return incognitoEnvByTab.get(tabId) as string;
    }
    return activeEnvironmentId.value;
  }

  function environmentColorForTab(tabId: string): PaletteColor {
    if (!useTabIncognitoStore().isIncognito(tabId) || !incognitoEnvByTab.has(tabId))
      return activeEnvironmentColor.value;
    const id = incognitoEnvByTab.get(tabId);
    return environments.value.find((e) => e.id === id)?.color ?? 'none';
  }

  /** In-memory when the tab is incognito, else the ordinary app-wide write (setActiveEnvironment). */
  async function selectEnvironmentForTab(tabId: string, id: string): Promise<void> {
    if (useTabIncognitoStore().isIncognito(tabId)) {
      incognitoEnvByTab.set(tabId, id);
      return;
    }
    await setActiveEnvironment(id);
  }

  registerTabRuntimeCleanup((tabId) => {
    incognitoEnvByTab.delete(tabId);
  });

  // An incognito tab's own override is never cleared by anything else — left in place after its
  // environment is deleted, it kept substituting the deleted environment's plain values while Go
  // resolved no secrets for the (now missing) id. This now also fires for a delete made in
  // *another* window. Skipped while envQuery.data.value is undefined, so an unloaded list never
  // wipes overrides.
  watch(environments, (list) => {
    if (envQuery.data.value === undefined) return;
    const ids = new Set(list.map((e) => e.id));
    for (const [tabId, envId] of incognitoEnvByTab) {
      if (!ids.has(envId)) incognitoEnvByTab.delete(tabId);
    }
  });

  const createEnvironmentMutation = useMutation(
    {
      mutationKey: ['apiEnvironments', 'create'],
      mutationFn: (args: { name: string; description: string; color: PaletteColor }) =>
        control.variablesCreateEnvironment(args.name, args.description, args.color),
      onSuccess: afterEnvironmentsListChange,
    },
    queryClient,
  );

  async function createEnvironment(
    name: string,
    description = '',
    color: PaletteColor = 'none',
  ): Promise<ApiEnvironment | undefined> {
    let env: ApiEnvironment | undefined;
    await surface.run(async () => {
      env = await createEnvironmentMutation.mutateAsync({ name, description, color });
    });
    return env;
  }

  const reorderEnvironmentsMutation = useMutation(
    {
      mutationKey: ['apiEnvironments', 'reorder'],
      mutationFn: (ids: string[]) => control.variablesReorderEnvironments(ids),
      onSuccess: afterEnvironmentsListChange,
    },
    queryClient,
  );

  /** Applies a whole-list environment order. Returns whether it saved. */
  async function reorderEnvironments(ids: readonly string[]): Promise<boolean> {
    return surface.run(() => reorderEnvironmentsMutation.mutateAsync([...ids]));
  }

  const deleteEnvironmentMutation = useMutation(
    {
      mutationKey: ['apiEnvironments', 'delete'],
      mutationFn: (id: string) => control.variablesDeleteEnvironment(id),
      onSuccess: afterEnvironmentsListChange,
    },
    queryClient,
  );

  /** Removes the environment's file. Deleting the active one leaves none active, and its open
   *  column leaves the Variables tab with the next list. */
  async function deleteEnvironment(id: string): Promise<void> {
    await surface.run(async () => {
      await deleteEnvironmentMutation.mutateAsync(id);
    });
  }

  const duplicateEnvironmentMutation = useMutation(
    {
      mutationKey: ['apiEnvironments', 'duplicate'],
      mutationFn: (id: string) => control.variablesDuplicateEnvironment(id),
      onSuccess: afterEnvironmentsListChange,
    },
    queryClient,
  );

  /** Copies the environment file under a free name; the copy is never active. */
  async function duplicateEnvironment(id: string): Promise<ApiEnvironment | undefined> {
    let env: ApiEnvironment | undefined;
    await surface.run(async () => {
      env = await duplicateEnvironmentMutation.mutateAsync(id);
    });
    return env;
  }

  // ---- the environments surface ----
  //
  // The dialog flag and its open/close pair are gone. `openEnvironments()` is the one entry point
  // every caller uses; it opens (or focuses) the environments tab. It used to also refresh the
  // list explicitly — the observer created above is always live, so there is nothing left to
  // trigger here.
  function openEnvironments(collectionId?: string): void {
    openVariablesTab(collectionId);
  }

  return {
    error: surface.error,
    environments,
    loaded,
    activeEnvironment,
    activeEnvironmentId,
    activeEnvironmentColor,
    dismissError,
    setActiveEnvironment,
    environmentIdForTab,
    environmentColorForTab,
    selectEnvironmentForTab,
    createEnvironment,
    reorderEnvironments,
    deleteEnvironment,
    duplicateEnvironment,
    openEnvironments,
  };
});

// ---- the gated reveal and the history popover ----
//
// What a query cache cannot hold: a reveal or history failure's message, the gated reveal and the
// history popover — reachable from each other (revealHistoryEntry into setVariableSetError), which
// is why they share one store. Rows are query data (useVariableRows, useVariableMatrix).

interface HistoryMenuState {
  open: boolean;
  variableId: string | null;
  entries: ApiVariableHistoryEntry[];
}

// ---- the merged value/secret cache, shared by both protocols ----
//
// Pure functions over rows now, not store members reading a scope/owner-keyed cache —
// mergedValuesAndSecrets/overviewRows used to call `cachedVariables` internally; every caller now
// already holds its own rows (a useVariableRows observer's `.data`, or a loadVariableRows/
// getQueryData read) and passes them in directly.

/** D12: a duplicate name within one scope resolves first-wins by sort_order — the caller's own
 *  rows are already in that order, so the first row claiming a name is the one that wins; a later
 *  same-named row is skipped rather than overwriting it. */
function firstWinsByName(rows: ApiVariable[]): Map<string, { value: string; isSecret: boolean }> {
  const out = new Map<string, { value: string; isSecret: boolean }>();
  for (const v of rows) {
    if (!out.has(v.name)) out.set(v.name, { value: v.value, isSecret: v.isSecret });
  }
  return out;
}

/** D2's precedence (environment over collection). */
export function mergeVariableRows(
  collectionRows: ApiVariable[],
  environmentRows: ApiVariable[],
): { values: Record<string, string>; secretNames: string[] } {
  const merged = firstWinsByName(collectionRows);
  for (const [name, entry] of firstWinsByName(environmentRows)) {
    merged.set(name, entry); // environment wins over collection (D2), regardless of within-scope order
  }
  const values: Record<string, string> = {};
  const secretNames: string[] = [];
  for (const [name, entry] of merged) {
    if (entry.isSecret) secretNames.push(name);
    else values[name] = entry.value;
  }
  return { values, secretNames };
}

/** variablesForSend's own ids-only half. */
async function apiIdsForTab(
  tabId: string,
  itemId: string | null,
): Promise<{ collectionId: string; environmentId: string }> {
  // Awaits the tree and environments queries first (cache-first; a fetch only happens
  // before either store's own app-lifetime observer has resolved once) — closes the "tree not
  // loaded yet" gap at send/describe time, rather than reading whatever the store happened to have
  // synchronously.
  await Promise.all([loadCollectionsTree(), loadEnvironments()]);
  return {
    collectionId: useCollectionsStore().collectionIdFor({ itemId }),
    environmentId: useVariablesStore().environmentIdForTab(tabId),
  };
}

/** Replaces five duplicated sync blocks in views/httprequest/state.ts, views/grpcrequest/
 *  state.ts and (via variableCompletion.ts) both request views — each used to read
 *  collectionIdFor/environmentIdForTab then mergedValuesAndSecrets over a store cache that
 *  ensureVariablesLoaded had to be trusted to already have populated. Awaiting loadVariableRows
 *  means a send after an invalidation always refetches first, so stale plain values can never
 *  substitute. */
export async function variablesForSend(
  tabId: string,
  itemId: string | null,
): Promise<{
  collectionId: string;
  environmentId: string;
  values: Record<string, string>;
  secretNames: string[];
}> {
  const { collectionId, environmentId } = await apiIdsForTab(tabId, itemId);
  const [collectionRows, environmentRows] = await Promise.all([
    loadVariableRows('collection', collectionId),
    loadVariableRows('environment', environmentId),
  ]);
  return { collectionId, environmentId, ...mergeVariableRows(collectionRows, environmentRows) };
}

export const useVariableSetStore = defineStore('variableSet', () => {
  /** A reveal or history failure's message, shown by the Variables tab. */
  const error = ref<string | null>(null);

  registerTabRuntimeCleanup(() => {
    // A stale reveal left behind by one tab must never let a different, later-opened tab (or a
    // later-opened Copy as curl dialog) trust it in place of its own re-auth gate.
    clearRevealed();
  });

  function setVariableSetError(message: string | null): void {
    error.value = message;
  }

  // ---- the gated reveal (D5/D8/D9) ----

  /** A revealed variable's plaintext, keyed by variable id — transient, cleared on dialog close
   *  (never written to tab state, never to a collection row). Not reactive-persisted anywhere else:
   *  this is the one place a secret's plaintext exists in the renderer at all. Shared with
   *  state/curl.ts's own reveal loop (revealSecretValues calls the same revealVariable below), so
   *  clearRevealed stays a store method (not private) — every dialog that can populate this map
   *  must also be able to clear it (a stale entry left behind by one dialog must never
   *  let a *different*, later-opened dialog trust it in place of its own re-auth gate). Returned as
   *  a named property directly (not toRefs-spread) — same Record-with-dynamic-keys reasoning as
   *  variableSetRuntime/listCache above.
   */
  const revealedValues = reactive<Record<string, string>>({});

  const revealedValuesExpiry = createRevealExpiry(revealedValues);

  /** Drops every revealed secret's plaintext from memory — called by every dialog/popover close
   *  path that can populate revealedValues (closeVariablesDialog here, closeCopyAsCurlDialog in
   *  curl.ts), the honest "not scrubbed, just dropped" limit (JS offers no way to zero a string
   *  in memory). Also cancels this id's grace-expiry timer so a delayed callback can't fire
   *  against a map a later reveal has since repopulated. */
  function clearRevealed(): void {
    for (const id of Object.keys(revealedValues)) delete revealedValues[id];
    revealedValuesExpiry.clearAll();
  }

  /** Drops one variable's revealed plaintext, e.g. after its value was replaced. */
  function forgetRevealed(id: string): void {
    delete revealedValues[id];
    revealedValuesExpiry.clear(id);
  }

  /** Runs over http/reveal.ts's shared recurse-once switch — the pattern used to be
   *  hand-copied per caller (there was nothing importable to reuse); now it is the module's own
   *  shared loop, used here and by
   *  revealHistoryEntry below.
   *
   *  `onError` is now required rather than falling back to a singleton dialog's error field —
   *  there is no longer one such field to fall back to (a variable-set view writes into its own
   *  tab's runtime via setVariableSetError; the *Copy as curl* reveal loop, http/state/curl.ts, has
   *  its own error sink either way). */
  async function revealVariable(
    id: string,
    onError: (message: string) => void,
  ): Promise<string | undefined> {
    return runReveal(
      (confirmed) => control.variablesReveal(id, confirmed),
      (value) => {
        revealedValues[id] = value;
        revealedValuesExpiry.schedule(id);
      },
      onError,
      t('studio.api.reveal.variable.body'),
    );
  }

  // ---- the per-variable history popover (D13) ----

  const historyMenuState = reactive<HistoryMenuState>({
    open: false,
    variableId: null,
    entries: [],
  });

  /** A revealed history entry's plaintext, keyed by history entry id — the same transient-map
   *  discipline revealedValues follows, cleared when the popover closes. Returned as a named
   *  property directly, same reasoning as the other Record-shaped fields above. */
  const revealedHistoryValues = reactive<Record<string, string>>({});

  // This map used to be cleared only by the popover's own close path
  // (openHistoryMenu/closeHistoryMenu below) — nothing re-masked a revealed prior value once the
  // grace it came from had expired, so a popover left open (or, per the comment on
  // openHistoryMenu below, torn down without its own @close firing) could hold a decrypted secret
  // history value indefinitely. Same grace, same discipline as revealedValues above.
  const revealedHistoryValuesExpiry = createRevealExpiry(revealedHistoryValues);

  function clearRevealedHistory(): void {
    for (const id of Object.keys(revealedHistoryValues)) delete revealedHistoryValues[id];
    revealedHistoryValuesExpiry.clearAll();
  }

  /** VariablesMatrix.vue's popover is `v-if`-gated on `historyMenuState.variableId ===
   *  row.id`, so switching from one cell's history to another's unmounts the first popover
   *  without ever firing its own `@close` — closeHistoryMenu below would never run for it. Clearing
   *  here too, before the new popover's own state lands, closes that gap regardless of whether the
   *  previous popover ever closes "cleanly".
   *
   *  This had no guard against a *second* call landing before the first's own
   *  `variablesHistory` await resolved — opening one row's history, then another's before the
   *  first reply arrived, let the first call's stale reply win the race and overwrite `entries`
   *  with the wrong row's history right after the second call had already set `variableId` to the
   *  new row. `variableId` is captured up front and the write only commits if nothing (a newer
   *  open, or a close) has changed it since. */
  async function openHistoryMenu(variableId: string): Promise<void> {
    clearRevealedHistory();
    historyMenuState.variableId = variableId;
    historyMenuState.open = true;
    try {
      const entries = await control.variablesHistory(variableId);
      if (historyMenuState.variableId === variableId) {
        historyMenuState.entries = entries;
      }
    } catch (err) {
      if (historyMenuState.variableId === variableId) {
        setVariableSetError(errorText(err));
      }
    }
  }

  function closeHistoryMenu(): void {
    historyMenuState.open = false;
    historyMenuState.variableId = null;
    historyMenuState.entries = [];
    clearRevealedHistory();
  }

  /** P12 D13: runReveal's own second instantiation, over api_variable_history instead of
   *  api_variables. */
  async function revealHistoryEntry(historyId: string): Promise<string | undefined> {
    return runReveal(
      (confirmed) => control.variablesRevealHistory(historyId, confirmed),
      (value) => {
        revealedHistoryValues[historyId] = value;
        revealedHistoryValuesExpiry.schedule(historyId);
      },
      setVariableSetError,
      t('studio.api.reveal.history.body'),
    );
  }

  return {
    error,
    revealedValues,
    revealedHistoryValues,
    ...toRefs(historyMenuState),
    setVariableSetError,
    clearRevealed,
    forgetRevealed,
    revealVariable,
    openHistoryMenu,
    closeHistoryMenu,
    revealHistoryEntry,
  };
});
