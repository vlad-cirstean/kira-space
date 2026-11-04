<script setup lang="ts">
import type { Caps } from '@shared/caps';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { Separator } from '@theme/components/ui/separator';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { onUnmounted, ref } from 'vue';
import { useTabsStore } from '../../state/tabs';
import { fieldNamesOnPage } from './page';
import { nextDocumentProjectionOnClose } from './projection';
import { useDocumentViewStore } from './state';

// Mirrors views/grid/ColumnsMenu.vue's UI pattern exactly (same header buttons, same list, same
// footer line), but a document collection has no catalog to list fields from — so the candidate
// list is `fieldNamesOnPage()`, the union of the loaded page's own top-level field names, shared
// with DocumentView.vue's toolbar badge so the two can't drift on how a body is parsed into field
// names.
const props = defineProps<{ tabId: string; caps: Caps | null }>();
const documentViewStore = useDocumentViewStore();

// A snapshot, not a computed: the picker's checkbox list shouldn't reshuffle under the user's
// cursor if a background refresh lands while the popover is open (ColumnsMenu.vue's own
// `columnNames` is a computed only because ObjectMeta is comparatively static; a document page's
// field set is not).
const fieldNames = fieldNamesOnPage(props.tabId);

function currentProjection(): string[] | null {
  return useTabsStore().findDocumentTab(props.tabId)?.state.projection ?? null;
}

// The projection the menu opened with, kept apart from `selected` (which the
// checkboxes mutate) so an untouched close can tell "nothing changed" from "the user chose the
// same fields All would" — see onUnmounted below. Note fieldNames is itself the *already
// projected* page's own field set, so it can legitimately equal the active projection exactly
// (a projection to [a, b] leaves the page showing only _id, a, b) — that equality is not, on its
// own, evidence the user wants "everything".
const initialProjection = currentProjection();
const initialSelected = new Set(initialProjection ?? fieldNames);
const selected = ref<Set<string>>(new Set(initialSelected));

// "everything" is only ever what an explicit All press means, never inferred from `selected`
// happening to match fieldNames.length by coincidence (the projected-page-equals-its-own-
// projection case above). Toggling a single checkbox, or None, is never "everything" either, even
// if a toggle-back-on later makes the set match fieldNames again.
let explicitAll = false;

function toggle(name: string): void {
  explicitAll = false;
  if (selected.value.has(name)) selected.value.delete(name);
  else selected.value.add(name);
}
function selectAll(): void {
  explicitAll = true;
  selected.value = new Set(fieldNames);
}
function selectNone(): void {
  explicitAll = false;
  selected.value = new Set();
}

// PopoverPanel's own close-commits-selection contract, preserved by committing on
// unmount instead of on a `close` emit -- the caller's Popover (ui/popover) unmounts this
// component's content on close for every reason (outside click, Escape, or the toolbar toggle
// button), the same set PopoverPanel's own hand-rolled backdrop+Escape handling covered.
onUnmounted(() => {
  const next = nextDocumentProjectionOnClose(
    selected.value,
    initialSelected,
    fieldNames.length,
    explicitAll,
  );
  if (next === undefined) return;
  documentViewStore.setProjection(props.tabId, next);
});
</script>

<template>
  <div class="max-h-80 flex flex-col">
    <div class="flex gap-1 p-1">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="document-projection-select-all"
        @click="selectAll"
        >{{
          t('common.state.all.option')
        }}</Button
      >
      <Button
        variant="toolbar"
        size="kira"
        data-testid="document-projection-select-none"
        @click="selectNone"
        >{{
          t('common.state.none.option')
        }}</Button
      >
    </div>
    <InlineState v-if="fieldNames.length === 0" kind="empty">{{
      t('studio.documents.projection.empty')
    }}</InlineState>
    <div v-else class="overflow-y-auto p-0.5">
      <Label v-for="name in fieldNames" :key="name" :class="rowVariants()">
        <Checkbox
          :model-value="selected.has(name)"
          class="size-3.5"
          data-testid="document-projection-menu-item"
          @update:model-value="toggle(name)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        {{ name }}
      </Label>
    </div>
    <Separator class="my-1" />
    <div
      class="px-1.5 pb-1.5 text-kira-sm text-subtle"
      data-testid="document-projection-menu-footer"
    >
      {{
        t(
          caps?.projection
            ? 'studio.documents.projection.footerServer.hint'
            : 'studio.documents.projection.footerClient.hint',
        )
      }}
    </div>
  </div>
</template>
