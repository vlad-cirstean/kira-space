<script setup lang="ts">
import type { Caps } from '@shared/caps';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { PopoverBar, PopoverContent, PopoverList } from '@theme/components/ui/popover';
import { Separator } from '@theme/components/ui/separator';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useFocus } from '@vueuse/core';
import { moveId, useSortableReorder } from '@workbench/behaviour/sortableReorder';
import {
  type ComponentPublicInstance,
  computed,
  onBeforeUnmount,
  ref,
  useTemplateRef,
  watch,
} from 'vue';
import { useTabsStore } from '../../../state/tabs';
import { filterColumnNames, nextProjectionFromSelectedColumns } from './menu';
import { getPage, pageVersion } from './page';
import { useGridViewStore } from './state';

const props = defineProps<{ tabId: string; caps: Caps | null }>();
const emit = defineEmits<{ revealColumn: [name: string] }>();
const gridViewStore = useGridViewStore();

const meta = computed(() => gridViewStore.runtime[props.tabId]?.meta ?? null);
const columnNames = computed(() => meta.value?.columns.map((c) => c.name) ?? []);
// PK columns can't be hidden — a row can't be identified/edited without it, and the grid's own
// mutation path assumes every visible PK column is present.
const pkNames = computed(
  () => new Set(meta.value?.columns.filter((c) => c.isPrimaryKey).map((c) => c.name) ?? []),
);

function currentProjection(): string[] | null {
  return useTabsStore().findDataTab(props.tabId)?.state.projection ?? null;
}
function currentColumnOrder(): string[] | null {
  return useTabsStore().findDataTab(props.tabId)?.state.columnOrder ?? null;
}

const selected = ref<Set<string>>(new Set(currentProjection() ?? columnNames.value));
// The drag-reorderable display order — seeded from whatever's stored (filtered/extended to the
// live column set), mirroring columns.ts's resolveColumnOrder() so this menu and the grid never
// disagree: stored order first (dropping any column that no longer exists), then new columns
// appended in their natural position.
function initialOrder(): string[] {
  const stored = currentColumnOrder();
  if (!stored) return columnNames.value;
  const known = new Set(columnNames.value);
  const kept = stored.filter((n) => known.has(n));
  const missing = columnNames.value.filter((n) => !kept.includes(n));
  return [...kept, ...missing];
}
const order = ref<string[]>(initialOrder());

// Search narrows this menu's rows only; the grid and the staged selection are untouched.
const query = ref('');
const visibleNames = computed(() => filterColumnNames(order.value, query.value));
const noMatch = computed(
  () => !!meta.value && query.value.trim() !== '' && visibleNames.value.length === 0,
);
const searchWrap = useTemplateRef<HTMLElement>('searchWrap');
const { focused: searchFocused } = useFocus(
  computed(() => searchWrap.value?.querySelector('input')),
);
function focusSearch(): void {
  searchFocused.value = true;
}
defineExpose({ focusSearch });

// The grid renders the committed projection; the page is not reactive, hence `pageVersion`.
const rendered = computed(() => {
  void pageVersion.n;
  return new Set(getPage(props.tabId)?.columns.map((c) => c.name) ?? []);
});

// Meta lands after the first page; a menu opened before then seeds once it arrives, and commits
// nothing if it closes first.
let seeded = columnNames.value.length > 0;
watch(columnNames, (names) => {
  if (seeded || names.length === 0) return;
  seeded = true;
  selected.value = new Set(currentProjection() ?? names);
  order.value = initialOrder();
});

function toggle(name: string): void {
  if (pkNames.value.has(name)) return; // primary key: always visible, checkbox is a no-op
  if (selected.value.has(name)) selected.value.delete(name);
  else selected.value.add(name);
}
function selectAll(): void {
  selected.value = new Set(columnNames.value);
}
function selectNone(): void {
  selected.value = new Set(pkNames.value);
}

function sameProjection(a: string[] | null, b: string[] | null): boolean {
  if (a === null || b === null) return a === b;
  if (a.length !== b.length) return false;
  const setA = new Set(a);
  return b.every((name) => setA.has(name));
}
function sameOrder(a: string[], b: string[] | null): boolean {
  if (b === null) return false;
  return a.length === b.length && a.every((name, i) => name === b[i]);
}

// Persistence stays in onBeforeUnmount below (commits `order` alongside `selected`). Drag clone
// stays inside the popover: a body-level one would count as an outside pointer and dismiss it.
const listComponent = useTemplateRef<ComponentPublicInstance>('listComponent');
const listEl = computed<HTMLElement | null>(() => listComponent.value?.$el ?? null);
useSortableReorder(
  listEl,
  () => visibleNames.value,
  (from, to) => {
    order.value = moveId(order.value, from, to);
  },
  {
    draggable: '[data-testid="columns-menu-row"]',
    direction: 'vertical',
    fallbackOnBody: false,
    keyboard: true,
    // Pressing the reveal button must not start a drag.
    filter: '[data-testid="columns-menu-reveal"]',
  },
);

// PopoverPanel's own @close (fired for every dismissal reason: outside click, Escape,
// the toggle button) becomes onBeforeUnmount — DataToolbar.vue's Popover mounts this component
// only while its own columnsOpen is true (v-if), so unmount fires exactly once, for the same set
// of reasons, and commits whatever's staged in `selected`/`order` regardless of why it closed.
onBeforeUnmount(() => {
  if (!seeded) return;
  const nextProjection = nextProjectionFromSelectedColumns([...selected.value], columnNames.value);
  if (!sameProjection(nextProjection, currentProjection())) {
    void gridViewStore.setProjection(props.tabId, nextProjection);
  }
  // A columnOrder is only ever stored non-null when it diverges from the column set's
  // own natural order. Comparing only against currentColumnOrder() (as this used to) meant
  // opening the menu and closing it without dragging anything would stage the default order as a
  // "custom" one the first time (currentColumnOrder() starts null, and sameOrder(_, null) is
  // always false) — stamping DataToolbar's Columns button with a "changed" dot for a change that
  // never happened.
  const nextOrder = sameOrder(order.value, columnNames.value) ? null : order.value;
  const current = currentColumnOrder();
  const orderChanged =
    (nextOrder === null) !== (current === null) ||
    (nextOrder !== null && !sameOrder(nextOrder, current));
  if (orderChanged) {
    gridViewStore.setColumnOrder(props.tabId, nextOrder);
  }
});
</script>

<template>
  <PopoverContent
    align="end"
    flush
    class="w-56"
    data-testid="columns-menu"
    @open-auto-focus="
      (e: Event) => {
        e.preventDefault();
        focusSearch();
      }
    "
  >
    <div class="max-h-80 flex flex-col">
      <div ref="searchWrap" class="px-1.5 pt-1.5">
        <SearchField
          v-model="query"
          :placeholder="t('studio.grid.columns.search.placeholder')"
          :aria-label="t('studio.grid.columns.search.aria')"
          data-testid="columns-menu-search"
        />
      </div>
      <PopoverBar>
        <Button variant="toolbar" size="kira" data-testid="columns-select-all" @click="selectAll"
          ><CodiconIcon name="check-all" :size="13" />{{ t('common.state.all.option') }}</Button
        >
        <Button variant="toolbar" size="kira" data-testid="columns-select-none" @click="selectNone"
          ><CodiconIcon :name="ICON.clearAll" :size="13" />
          {{ t('common.state.none.option') }}</Button
        >
      </PopoverBar>
      <!-- Drag a row to reorder — the same order the grid renders columns in
           (columns.ts's resolveColumnOrder). Checkbox toggles visibility; the PK's is locked.
           The container is always mounted: the sortable binds to it once, at mount. -->
      <PopoverList ref="listComponent" class="overflow-y-auto">
        <InlineState v-if="!meta" kind="loading">{{
          t('studio.grid.columns.loading.loading')
        }}</InlineState>
        <Label
          v-for="name in visibleNames"
          :key="name"
          :class="cn(rowVariants(), 'columns-menu-item', { 'is-pk': pkNames.has(name) })"
          data-testid="columns-menu-row"
          :data-name="name"
        >
          <span class="flex items-center shrink-0 text-subtle cursor-grab" aria-hidden="true"
            ><CodiconIcon name="gripper" :size="13" /></span
          >
          <TextTooltip v-if="pkNames.has(name)" :text="t('studio.grid.columns.primaryKey.tooltip')"
            ><Checkbox
              :model-value="selected.has(name)"
              :aria-label="name"
              disabled
              data-testid="columns-menu-item"
              @update:model-value="toggle(name)"
            /></TextTooltip
          >
          <Checkbox
            v-else
            :model-value="selected.has(name)"
            :aria-label="name"
            data-testid="columns-menu-item"
            @update:model-value="toggle(name)"
          />
          {{ name }}
          <span class="ml-auto flex shrink-0">
            <TooltipIconButton
              icon="eye"
              :label="
                rendered.has(name)
                  ? t('studio.grid.columns.reveal.tooltip')
                  : t('studio.grid.columns.revealHidden.tooltip')
              "
              :aria-label="t('studio.grid.columns.reveal.aria', { column: name })"
              variant="toolbar"
              class="size-5 text-subtle"
              :disabled="!rendered.has(name)"
              :disabled-trigger="!rendered.has(name)"
              data-testid="columns-menu-reveal"
              @click.stop.prevent="emit('revealColumn', name)"
            />
          </span>
        </Label>
        <InlineState v-if="noMatch" kind="empty" data-testid="columns-menu-no-match">{{
          t('studio.grid.columns.noMatch.status')
        }}</InlineState>
      </PopoverList>
      <Separator class="my-1" />
      <div class="px-1.5 pb-1.5 text-kira-sm text-subtle" data-testid="columns-menu-footer">
        {{
          t(
            caps?.projection
              ? 'studio.grid.columns.footerServer.hint'
              : 'studio.grid.columns.footerClient.hint',
          )
        }}
      </div>
    </div>
  </PopoverContent>
</template>
