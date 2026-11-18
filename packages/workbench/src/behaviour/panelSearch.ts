import { tryOnScopeDispose, useEventListener } from '@vueuse/core';
import { nextTick, type Ref, ref } from 'vue';
import { useShortcut } from '../shortcuts/useShortcut';
import { useKeptState } from './viewState';

const SIDE_PANEL = '[data-slot="side-panel"]';

interface PanelFindEntry {
  root: () => HTMLElement | null;
  searchable: () => boolean;
  open: () => void;
}

// Handler registry for the menu Find event and the palette: plain module state, like commands.ts
// (git-ui mounts have no Pinia).
const panelFinds = new Set<PanelFindEntry>();

function sideEntries(): { root: HTMLElement; entry: PanelFindEntry }[] {
  const out: { root: HTMLElement; entry: PanelFindEntry }[] = [];
  for (const entry of panelFinds) {
    const root = entry.root();
    if (root?.closest(SIDE_PANEL) && entry.searchable()) out.push({ root, entry });
  }
  return out;
}

/** Cmd/Ctrl+F from the menu: open the innermost searchable left panel holding focus. */
export function openFocusedPanelSearch(): boolean {
  const active = document.activeElement;
  if (!active) return false;
  const hits = sideEntries().filter(({ root }) => root.contains(active));
  const inner = hits.find(
    ({ root }) => !hits.some((o) => o.root !== root && root.contains(o.root)),
  );
  if (!inner) return false;
  inner.entry.open();
  return true;
}

/** Palette: open the outermost searchable left panel (the module's own panel). */
export function openActivePanelSearch(): boolean {
  const all = sideEntries();
  const outer = all.find(({ root }) => !all.some((o) => o.root !== root && o.root.contains(root)));
  if (!outer) return false;
  outer.entry.open();
  return true;
}

async function focusField(container: HTMLElement, testid: string, select: boolean): Promise<void> {
  await nextTick();
  const host = container.querySelector<HTMLElement>(`[data-testid="${testid}"]`);
  const input = host instanceof HTMLInputElement ? host : host?.querySelector('input');
  input?.focus();
  if (select) input?.select();
}

// P104 §3 (PanelShell -> inline composition): the reveal/toggle/type-ahead-redirect behaviour
// PanelShell.vue used to own, factored out so GitPanel.vue/AutomationsPanel.vue/ProjectPanel.vue can
// each inline their own header markup without tripling this logic. Ported verbatim from
// PanelShell.vue's own onPanelKeydown/toggleSearch/revealSearch.
//
// P105 §5.1: the keydown listener binds here via VueUse's useEventListener against the caller's
// own root-element ref, instead of each caller wiring `@keydown` on a plain div itself (Biome's
// noStaticElementInteractions/useKeyWithClickEvents flag a non-interactive element carrying a raw
// keyboard listener; this delegated scope genuinely isn't an interactive element).
export function usePanelHeaderSearch(
  rootEl: Ref<HTMLElement | null | undefined>,
  opts: {
    searchable: () => boolean;
    getSearch: () => string;
    setSearch: (value: string) => void;
    searchTestid?: string;
    /** View-state key prefix: bar visibility survives an unmount. Null keeps it a local ref (git-ui has no Pinia, P660 Part 2). */
    stateKey: string | null;
    /** Runs when Cmd/Ctrl+F reveals the bar, for a panel with its own visibility flag. */
    onReveal?: () => void;
  },
) {
  const testid = opts.searchTestid ?? 'tree-search';
  const showSearch =
    opts.stateKey === null ? ref(false) : useKeptState(`${opts.stateKey}:showSearch`, false);
  // Never a hidden active filter.
  if (opts.getSearch() !== '') showSearch.value = true;

  function revealSearch(): void {
    if (showSearch.value) return;
    showSearch.value = true;
  }

  function toggleSearch(): void {
    showSearch.value = !showSearch.value;
    if (!showSearch.value) opts.setSearch('');
  }

  // VS Code's own file-explorer "type to search" pattern: a printable keystroke landing on the
  // panel (not already inside a field) redirects into the search box, revealing it first if hidden.
  function onPanelKeydown(e: KeyboardEvent): void {
    if (!opts.searchable()) return;
    if (e.defaultPrevented || e.isComposing) return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key.length !== 1 || e.key === ' ') return;
    const target = e.target as HTMLElement | null;
    if (target?.closest('input, textarea, [contenteditable="true"]')) return;
    const container = e.currentTarget as HTMLElement;
    e.preventDefault();
    opts.setSearch(opts.getSearch() + e.key);
    revealSearch();
    void focusField(container, testid, false);
  }

  function openSearch(): void {
    const root = rootEl.value;
    if (!root) return;
    revealSearch();
    opts.onReveal?.();
    void focusField(root, testid, true);
  }

  const inSidePanel = (): boolean => !!rootEl.value?.closest(SIDE_PANEL);

  useEventListener(rootEl, 'keydown', onPanelKeydown);
  // Bubble phase: the innermost panel root sees the key first, stopPropagation shields outer ones.
  useShortcut(
    rootEl,
    ['panel.find'],
    (_, e) => {
      e.stopPropagation();
      openSearch();
    },
    { when: () => opts.searchable() && inSidePanel() },
  );
  const entry: PanelFindEntry = {
    root: () => rootEl.value ?? null,
    searchable: opts.searchable,
    open: openSearch,
  };
  panelFinds.add(entry);
  tryOnScopeDispose(() => panelFinds.delete(entry));

  return { showSearch, toggleSearch };
}
