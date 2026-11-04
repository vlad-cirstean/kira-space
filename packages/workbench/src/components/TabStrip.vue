<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { tabChipVariants } from '@theme/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowActions } from '@theme/lib/rowVariants';
import { computed, nextTick, ref, watch } from 'vue';
import { copyText } from '../behaviour/clipboard';
import { revealElement } from '../behaviour/focus';
import { useSortableReorder } from '../behaviour/sortableReorder';
import { type TabLike, type TabStripHost, useWorkbenchHost } from '../host';
import { wheelToHorizontal } from '../lib/wheelScroll';
import { type MenuItem, useContextMenuStore } from '../state/contextMenu';

// P103 Part 2 (§5.4): Kira Studio's own workbench/panels/TabStrip.vue and Kira Space's, unified.
// The strip, drag-reorder (vue-draggable-plus since P137), wheel-scroll, keyboard-scroll-into-view
// and the six generic context-menu items are exactly the two apps' shared skeleton (confirmed side by side); every
// per-app extra — Kira Studio's incognito eye glyph, Kira Space's seti file icons — arrives
// through the host's own `iconFor`/`tabIndicator`/`tabBadge` hooks (host.ts) instead of an
// app-local import. The trailing "+"
// new-tab affordance is real per-app
// divergence (Kira Studio: the Automations module's own plain-session menu; Kira Space: one repo-root
// terminal, no menu) — not a lookup a host hook can express cleanly, so it stays a `#new-tab` slot,
// each app supplying its own button exactly as before.
// Read once: a host object is stable for the component's life (both apps and ade build it once).
const props = defineProps<{ host?: TabStripHost<string, TabLike> }>();
const host: TabStripHost<string, TabLike> = props.host ?? useWorkbenchHost();
const contextMenuStore = useContextMenuStore();

function isPinned(tab: TabLike): boolean {
  return host.kinds[tab.kind]?.pinned === true;
}

function titleFor(tab: TabLike): string {
  return host.kinds[tab.kind]?.title(tab) ?? '';
}

function badgeFor(tab: TabLike): { icon: string; tooltip: string } | null {
  return host.tabBadge?.(tab) ?? null;
}

function indicatorFor(tab: TabLike): { icon: string; tooltip: string } | null {
  return host.tabIndicator?.(tab) ?? null;
}

function onClick(tab: TabLike): void {
  host.tabs.activateTab(tab.id);
}

// §6.1: a pinned tab has no middle-click close; nor does a host without `closeTab`.
function onMiddleClick(tab: TabLike): void {
  if (isPinned(tab)) return;
  host.tabs.closeTab?.(tab.id);
}

function onClose(e: MouseEvent, tab: TabLike): void {
  e.stopPropagation();
  host.tabs.closeTab?.(tab.id);
}

function isPreview(tab: TabLike): boolean {
  return host.tabs.isPreview?.(tab.id) ?? false;
}

// Each close item exists only when the host supplies its capability.
function closeItems(tab: TabLike): MenuItem[] {
  const { closeTab, closeOthers, closeToTheRight, closeAll } = host.tabs;
  const items: MenuItem[] = [];
  if (closeTab) {
    items.push({
      type: 'item',
      id: 'close',
      label: t('common.action.close.label'),
      icon: 'close',
      // P21 D13: `tab.close` always closes the *active* tab, not the clicked one — printed anyway
      // (VS Code does the same on this exact row) since it's the keyboard route to this command.
      shortcut: 'tab.close',
      run: () => closeTab(tab.id),
    });
  }
  if (closeOthers) {
    items.push({
      type: 'item',
      id: 'close-others',
      label: t('common.action.closeOthers.label'),
      run: () => closeOthers(tab.id),
    });
  }
  if (closeToTheRight) {
    items.push({
      type: 'item',
      id: 'close-to-the-right',
      label: t('common.action.closeToTheRight.label'),
      run: () => closeToTheRight(tab.id),
    });
  }
  if (closeAll) {
    items.push({
      type: 'item',
      id: 'close-all',
      label: t('common.action.closeAll.label'),
      run: () => closeAll(host.activeWorkspace.value),
    });
  }
  return items;
}

// §8.10's Tab row: Close · Close others · Close to the right · Close all · — · Duplicate tab ·
// Copy name · plus whatever the tab's own kind appends (D22).
// §6.1: a pinned tab's own menu is reduced to just "Copy name".
function onContextMenu(e: MouseEvent, tab: TabLike): void {
  if (isPinned(tab)) {
    contextMenuStore.openContextMenu(e, [
      {
        type: 'item',
        id: 'copy-name',
        label: t('common.action.copyName.label'),
        icon: 'copy',
        run: () => copyText(titleFor(tab)),
      },
    ]);
    return;
  }
  const closeGroup = closeItems(tab);
  const { duplicateTab } = host.tabs;
  const items: MenuItem[] = [
    ...closeGroup,
    ...(closeGroup.length > 0 ? [{ type: 'separator' } as const] : []),
    ...(duplicateTab
      ? [
          {
            type: 'item',
            id: 'duplicate-tab',
            label: t('workbench.tabStrip.duplicate.label'),
            icon: ICON.duplicate,
            run: () => void duplicateTab(tab.id),
          } as const,
        ]
      : []),
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      run: () => copyText(titleFor(tab)),
    },
    ...(host.kinds[tab.kind]?.menuExtras(tab) ?? []),
  ];
  contextMenuStore.openContextMenu(e, items);
}

const tabs = computed(() => host.tabs.tabsForWorkspace(host.activeWorkspace.value));

// P72 §7: split out of `tabs` so the template can render the pinned tab in a fixed leading slot,
// outside the scrolling tab row's own `overflow-x: auto` — it never scrolls away with everything
// else.
const pinnedTabs = computed(() =>
  tabs.value.filter((tab) => isPinned(tab)).map((tab) => ({ tab, icon: host.iconFor(tab) })),
);
const scrollingTabs = computed(() =>
  tabs.value.filter((tab) => !isPinned(tab)).map((tab) => ({ tab, icon: host.iconFor(tab) })),
);

// Selecting a tab from anywhere other than this strip itself previously left the strip's own
// scroll position untouched — the newly active tab could be selected yet scrolled out of view.
const activeTabId = computed(() => host.tabs.activeIdByWorkspace[host.activeWorkspace.value]);
const stripRef = ref<HTMLElement | null>(null);

watch(
  activeTabId,
  (id) => {
    if (!id) return;
    void nextTick().then(() => {
      const tab = stripRef.value?.querySelector<HTMLElement>(`[data-tab-id="${id}"]`);
      if (tab) revealElement(tab);
    });
  },
  { immediate: true },
);

// P31 D7/F9: this strip's own horizontal scrollbar is deliberately thin/subtle — without this, the
// strip is reachable only by trackpad.
function onWheel(e: WheelEvent): void {
  if (wheelToHorizontal(stripRef.value, e)) e.preventDefault();
}

const moveTab = host.tabs.moveTab;
if (moveTab) {
  // Pinned chips sit outside `stripRef`, so they never take part.
  useSortableReorder(stripRef, () => scrollingTabs.value.map(({ tab }) => tab.id), moveTab, {
    draggable: '[data-testid="tab"]',
    filter: '[data-testid="tab-close"]',
    direction: 'horizontal',
    keyboard: true,
  });
}
</script>

<template>
  <!-- P91 §8: always one wrapper — the Automations module's own normal initial state is zero tabs,
       which needs the "+" (below) just as much as a populated strip does. -->
  <div
    class="h-full flex items-center min-w-0"
    :class="{ 'px-1': tabs.length === 0 }"
    :data-testid="tabs.length > 0 ? 'tab-strip-wrapper' : 'tab-strip-empty'"
  >
    <!-- P72 §7: the pinned tab's own fixed leading slot — a sibling of the scrolling tab row,
         outside its `overflow-x: auto`, so it never scrolls away. Icon-only: the repo name moves to the
         tooltip/`aria-label` (`titleFor`), the chrome is one glyph. Never draggable (§6.1) and
         never has a close button, so neither is wired here at all rather than guarded per-tab. -->
    <div
      v-if="pinnedTabs.length > 0"
      class="h-full flex items-center gap-0.5 shrink-0 pl-1"
      data-testid="tab-strip-pinned"
    >
      <template v-for="{ tab, icon } in pinnedTabs" :key="tab.id">
        <Tooltip>
          <TooltipTrigger as-child>
            <button
              type="button"
              :class="tabChipVariants({ active: tab.active, size: 'icon' })"
              data-testid="tab"
              :data-tab-id="tab.id"
              :data-tab-kind="tab.kind"
              :data-active="tab.active"
              data-pinned="true"
              :aria-label="titleFor(tab)"
              @click="onClick(tab)"
              @contextmenu.prevent="onContextMenu($event, tab)"
            >
              <CodiconIcon
                v-if="'codicon' in icon"
                :name="icon.codicon"
                :size="13"
                class="shrink-0"
              />
              <span
                v-else
                class="shrink-0 tab-file-icon w-3.5 h-3.5 text-muted-foreground mask-contain mask-no-repeat mask-center"
                :style="icon.fileStyle"
                aria-hidden="true"
              />
            </button>
          </TooltipTrigger>
          <TooltipContent>{{ titleFor(tab) }}</TooltipContent>
        </Tooltip>
      </template>
      <span class="self-stretch shrink-0 w-px my-1 mr-0.5 bg-transparent" aria-hidden="true"></span>
    </div>
    <div
      ref="stripRef"
      class="h-full flex items-center gap-0.5 overflow-x-auto overflow-y-hidden min-w-0 scrollbar-none px-1"
      data-testid="tab-strip-row"
      @wheel="onWheel"
    >
      <!-- A focusable close control nested inside the tab's own <button> is invalid
           HTML and unreachable by keyboard — the close button is this tab's sibling now, not its
           child. The whole chip is the drag handle (vue-draggable-plus, on the row);
           click/dblclick/auxclick/contextmenu live on the tab's own inner button. -->
      <div
        v-for="{ tab, icon } in scrollingTabs"
        :key="tab.id"
        class="group"
        :class="tabChipVariants({ active: tab.active })"
        data-testid="tab"
        :data-tab-id="tab.id"
        :data-tab-kind="tab.kind"
        :data-active="tab.active"
        :data-preview="isPreview(tab)"
        data-pinned="false"
        :data-color="host.railColorFor(tab)"
      >
        <span :class="colorMarkClass('bar', host.railColorFor(tab))" />
        <button
          type="button"
          :aria-current="tab.active || undefined"
          class="flex flex-1 min-w-0 items-center gap-1 border-0 bg-transparent p-0"
          @click="onClick(tab)"
          @dblclick="host.tabs.promoteTab?.(tab.id)"
          @auxclick.middle="onMiddleClick(tab)"
          @contextmenu.prevent="onContextMenu($event, tab)"
        >
          <CodiconIcon v-if="'codicon' in icon" :name="icon.codicon" :size="13" class="shrink-0" />
          <span
            v-else
            class="shrink-0 tab-file-icon w-3.5 h-3.5 text-muted-foreground mask-contain mask-no-repeat mask-center"
            :style="icon.fileStyle"
            aria-hidden="true"
          />
          <slot name="tab-leading" :tab="tab" />
          <Tooltip v-if="indicatorFor(tab)">
            <TooltipTrigger as-child>
              <CodiconIcon
                :name="indicatorFor(tab)!.icon"
                :size="12"
                class="shrink-0 text-muted-foreground"
              />
            </TooltipTrigger>
            <TooltipContent>{{ indicatorFor(tab)!.tooltip }}</TooltipContent>
          </Tooltip>
          <span class="tab-title truncate min-w-0" :class="{ italic: isPreview(tab) }">{{
            titleFor(tab)
          }}</span>
          <Tooltip v-if="badgeFor(tab)">
            <TooltipTrigger as-child>
              <CodiconIcon
                :name="badgeFor(tab)!.icon"
                :size="12"
                class="shrink-0 text-muted-foreground"
                data-testid="tab-badge"
              />
            </TooltipTrigger>
            <TooltipContent>{{ badgeFor(tab)!.tooltip }}</TooltipContent>
          </Tooltip>
        </button>
        <Button
          v-if="host.tabs.closeTab"
          variant="toolbar"
          size="title"
          class="tab-close size-4 p-0"
          :class="tab.active ? 'opacity-100' : rowActions"
          :aria-label="t('docker.exec.close.tooltip', { title: titleFor(tab) })"
          data-testid="tab-close"
          @click="onClose($event, tab)"
        >
          <CodiconIcon name="close" :size="13" />
        </Button>
      </div>
    </div>
    <!-- A third fixed child, after the scrolling tab row, mirroring the pinned tab's own
         leading-edge fix at the other end. Per-app "new tab" affordance — the whole
         `data-testid="tab-strip-actions"` wrapper is slot content (not a wrapper this component
         owns), so each app keeps its own `v-if="showNewTab"` gating the element's very presence in
         the DOM. The wrapper and button are now plain Tailwind utility classes inlined in each
         app's own WorkbenchShell.vue, not a tab strip actions/tab new class published from
         workbench.css — the data-testid is what's shared now. -->
    <slot name="new-tab" />
    <!-- `.tab-chip`'s hover/attention/close-reveal rules moved into `:class` ternaries
         above -- `group` on each chip replaces `.tab-chip:hover .tab-close`/`.tab-chip.is-active
         .tab-close` (`group-hover/tab:opacity-100`, plus the active branch's own `opacity-100`).
         `.tab-chip` was marker-only (no CSS-class test locator); `.is-active`/`.tab-file-icon`/
         `.tab-title`/`.tab-close` stay real classes (slick-grid.spec.ts, budgets.spec.ts,
         repo-workspace.spec.ts, font-roles.spec.ts, multiwindow-real.spec.ts,
         definition.spec.ts). -->
  </div>
</template>
