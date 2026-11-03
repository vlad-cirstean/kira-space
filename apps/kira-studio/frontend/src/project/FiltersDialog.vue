<script setup lang="ts">
import type { NodeKind } from '@shared/domain/tree';
import { EMPTY_VISIBILITY, type TreeVisibility } from '@shared/domain/treeFilter';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { rowMeta } from '@theme/lib/rowVariants';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useCappedList } from '@workbench/behaviour/cappedList';
import { revealElement } from '@workbench/behaviour/focus';
import { compareNatural } from '@workbench/behaviour/match';
import ShowMoreButton from '@workbench/components/ShowMoreButton.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, nextTick, ref, watch } from 'vue';
import { useConnectionsStore } from '../state/connections';
import {
  type FilterNodeRow,
  kindRows,
  nodeRows,
  previewCounts,
  toggleKind,
  toggleNode,
} from './filterTree';
import { useFiltersDialogStore, useTreeStore } from './state/tree';

// P28 D10-D19: checkboxes, not rules. Two sections over the same cached-node model — Object
// types (kind, flat) and Objects (path, expandable) — plus a live-consequence strip. Nothing here
// fetches; the dialog offers exactly what the tree has already cached (D21).

const connectionsStore = useConnectionsStore();
const filtersDialogStore = useFiltersDialogStore();
const treeStore = useTreeStore();
const draft = ref<TreeVisibility>(EMPTY_VISIBILITY);
const expandedPaths = ref<Set<string>>(new Set());
const nameFilter = ref('');
const sig = (v: TreeVisibility): string =>
  JSON.stringify([
    [...v.hiddenKinds].sort(compareNatural),
    [...v.hiddenPaths].sort(compareNatural),
  ]);
const baseline = ref(sig(EMPTY_VISIBILITY));
const dirty = computed(() => sig(draft.value) !== baseline.value);
// Saving replaces the whole persisted set, so the draft must seed from the loaded set, never from
// an empty default: Filters... is offered on connections that were never expanded.
const {
  run: runLoad,
  pending: loading,
  error: loadError,
} = useAsyncAction((connectionId: string) => treeStore.loadVisibility(connectionId));
const {
  run: runSave,
  pending: saving,
  error: saveError,
} = useAsyncAction(async (connectionId: string) => {
  await treeStore.saveVisibility(connectionId, draft.value);
  filtersDialogStore.closeFiltersDialog();
});

// D20: every ancestor segment of `path`, outermost first — mirrors state/tree.ts's own
// revealPath() accumulation, since a dialog row and a tree row share the same path encoding.
function ancestorsOf(path: string): Set<string> {
  const segments = path.split('/');
  const out = new Set<string>();
  let acc = '';
  for (let i = 0; i < segments.length - 1; i++) {
    acc = acc ? `${acc}/${segments[i]}` : segments[i];
    out.add(acc);
  }
  return out;
}

watch(
  () => filtersDialogStore.connectionId,
  async (connectionId) => {
    const focusPath = filtersDialogStore.focusPath;
    expandedPaths.value = focusPath ? ancestorsOf(focusPath) : new Set();
    nameFilter.value = '';
    loadError.value = null;
    saveError.value = null;
    draft.value = EMPTY_VISIBILITY;
    baseline.value = sig(EMPTY_VISIBILITY);
    if (!connectionId) return;
    await runLoad(connectionId);
    if (filtersDialogStore.connectionId !== connectionId) return;
    const existing = treeStore.visibility[connectionId];
    if (!existing) {
      loadError.value ??= t('studio.project.filtersDialog.loadFailed.error');
      return;
    }
    draft.value = {
      hiddenKinds: [...existing.hiddenKinds],
      hiddenPaths: [...existing.hiddenPaths],
    };
    baseline.value = sig(draft.value);
    if (!focusPath) return;
    await nextTick();
    const row = document.querySelector(
      `[data-testid="filter-object-row"][data-path="${CSS.escape(focusPath)}"]`,
    );
    if (row) revealElement(row);
  },
  { immediate: true },
);

const kinds = computed(() =>
  filtersDialogStore.connectionId ? kindRows(filtersDialogStore.connectionId, draft.value) : [],
);

const objectRows = computed(() =>
  filtersDialogStore.connectionId
    ? nodeRows(filtersDialogStore.connectionId, draft.value, expandedPaths.value, nameFilter.value)
    : [],
);
// A Redis namespace with 20 000 cached keys must not render 20 000 checkboxes (D18); the name
// filter is the intended way through such a level.
const objects = useCappedList(objectRows, 500);
watch(nameFilter, objects.reset);

// A live preview computed from the same filterTree.ts previewCounts() the tree itself is
// evaluated with, so this dialog cannot disagree with what the tree will actually show.
const preview = computed(() => {
  const connectionId = filtersDialogStore.connectionId;
  if (!connectionId) return { shown: 0, total: 0 };
  return previewCounts(connectionId, draft.value);
});

function onToggleKind(kind: NodeKind): void {
  if (loading.value) return;
  draft.value = toggleKind(draft.value, kind);
}

function onToggleNode(row: FilterNodeRow): void {
  if (row.disabled || loading.value) return;
  draft.value = toggleNode(draft.value, row);
}

function onToggleExpand(path: string): void {
  const next = new Set(expandedPaths.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  expandedPaths.value = next;
}

// D18: All/None act on the currently listed subset only — under a name filter, that is just the
// matching rows and their ancestors, not the whole cached tree.
function allObjects(): void {
  if (loading.value) return;
  let v = draft.value;
  for (const row of objectRows.value) {
    if (row.state !== 'on' && !row.disabled) v = toggleNode(v, row);
  }
  draft.value = v;
}

function noneObjects(): void {
  if (loading.value) return;
  let v = draft.value;
  for (const row of objectRows.value) {
    if (row.state !== 'off' && !row.disabled) v = toggleNode(v, row);
  }
  draft.value = v;
}

function allKinds(): void {
  if (loading.value) return;
  draft.value = { ...draft.value, hiddenKinds: [] };
}

function noneKinds(): void {
  if (loading.value) return;
  draft.value = { ...draft.value, hiddenKinds: kinds.value.map((r) => r.kind) };
}

async function onSave(): Promise<void> {
  const connectionId = filtersDialogStore.connectionId;
  if (!connectionId || saving.value) return;
  await runSave(connectionId);
}

// Title identity (FiltersDialog.html: "Tree filters — prod-analytics") — reads the name off
// the store that already has it; adds no new state.
const connectionName = computed(
  () => connectionsStore.connectionRecord(filtersDialogStore.connectionId)?.name ?? '',
);
</script>

<template>
  <Dialog
    v-if="filtersDialogStore.open"
    :open="true"
    @update:open="(v) => !v && filtersDialogStore.closeFiltersDialog()"
  >
    <DialogContent size="lg" :dirty="dirty" data-testid="filters-dialog">
      <DialogHeader close-testid="filters-dialog-close">
        <DialogTitle>{{
          connectionName
            ? t('studio.project.filtersDialog.dialog.named.title', { connectionName })
            : t('studio.project.filtersDialog.dialog.title')
        }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <FieldDescription>{{ t('studio.project.filtersDialog.intro.body') }}</FieldDescription>

        <section class="flex flex-col gap-1">
          <SectionHeading :label="t('studio.project.filtersDialog.kinds.heading.label')">
            <span class="flex gap-1">
              <Button variant="link" size="inline" @click="allKinds">{{
                t('common.state.all.option')
              }}</Button>
              <Button variant="link" size="inline" @click="noneKinds">{{
                t('common.state.none.option')
              }}</Button>
            </span>
          </SectionHeading>
          <Card
            variant="list"
            class="flex max-h-56 flex-col gap-px overflow-y-auto"
            data-testid="filter-kind-list"
          >
            <Label
              v-for="row in kinds"
              :key="row.kind"
              class="flex items-center cursor-default gap-1 h-6.5"
              :data-testid="`filter-kind-row-${row.kind}`"
              :data-state="row.hidden ? 'off' : 'on'"
            >
              <Checkbox
                :model-value="!row.hidden"
                class="size-3.5 after:inset-0"
                @update:model-value="onToggleKind(row.kind)"
              >
                <CodiconIcon name="check" :size="12" />
              </Checkbox>
              <span class="min-w-0 flex-1 truncate text-kira-md">{{ row.label }}</span>
              <span :class="rowMeta">{{ row.count }}</span>
            </Label>
            <InlineState v-if="kinds.length === 0" kind="empty">{{
              t('studio.project.filtersDialog.cache.empty')
            }}</InlineState>
          </Card>
        </section>

        <section class="flex flex-col gap-1">
          <SectionHeading :label="t('studio.project.filtersDialog.objects.heading.label')">
            <span class="flex gap-1">
              <Button variant="link" size="inline" @click="allObjects">{{
                t('common.state.all.option')
              }}</Button>
              <Button variant="link" size="inline" @click="noneObjects">{{
                t('common.state.none.option')
              }}</Button>
            </span>
          </SectionHeading>
          <SearchField
            v-model="nameFilter"
            size="kira-lg"
            :placeholder="t('studio.project.filtersDialog.nameFilter.placeholder')"
            data-testid="filter-name-input"
          />
          <Card
            variant="list"
            class="flex max-h-56 flex-col gap-px overflow-y-auto"
            data-testid="filter-object-list"
          >
            <div
              v-for="row in objects.visible.value"
              :key="row.path"
              class="flex items-center cursor-default gap-1 h-6.5"
              data-testid="filter-object-row"
              :data-path="row.path"
              :data-state="row.state"
              :style="{ paddingLeft: `${8 + row.depth * 14}px` }"
            >
              <TreeTwisty
                :expanded="expandedPaths.has(row.path)"
                :has-children="row.hasChildren"
                @toggle="onToggleExpand(row.path)"
              />
              <Tooltip :disabled="!row.disabledReason">
                <TooltipTrigger as-child>
                  <Label class="object-checkbox-label flex items-center flex-1 min-w-0 gap-1">
                    <Checkbox
                      :model-value="row.state === 'partial' ? 'indeterminate' : row.state !== 'off'"
                      :disabled="row.disabled"
                      class="size-3.5 after:inset-0"
                      @update:model-value="onToggleNode(row)"
                    >
                      <template #default="{ state }">
                        <CodiconIcon
                          :name="state === 'indeterminate' ? 'dash' : 'check'"
                          :size="12"
                        />
                      </template>
                    </Checkbox>
                    <span class="min-w-0 flex-1 truncate text-kira-md">{{ row.name }}</span>
                  </Label>
                </TooltipTrigger>
                <TooltipContent v-if="row.disabledReason">{{ row.disabledReason }}</TooltipContent>
              </Tooltip>
              <span v-if="row.hasChildren" :class="rowMeta">{{ row.childCount }}</span>
            </div>
            <InlineState v-if="objectRows.length === 0" kind="empty">{{
              t('studio.project.filtersDialog.cache.empty')
            }}</InlineState>
            <ShowMoreButton
              :hidden-count="objects.hiddenCount.value"
              :step="objects.step"
              data-testid="filter-object-show-more"
              @click="objects.showMore"
            />
          </Card>
        </section>

        <Alert variant="info" data-testid="filters-preview">
          <AlertDescription>
            <CatalogText k="studio.project.filtersDialog.preview.body">
              <template #shown><b>{{ preview.shown }}</b></template>
              <template #total><b>{{ preview.total }}</b></template>
            </CatalogText>
          </AlertDescription>
        </Alert>

        <FieldDescription class="cached-note">{{
          t('studio.project.filtersDialog.cachedNote.hint')
        }}</FieldDescription>

        <Alert v-if="loadError || saveError" variant="error" data-testid="filters-error">
          <AlertDescription>{{ loadError ?? saveError }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <template #start>
          <FieldDescription
            ><CatalogText k="studio.project.connectionScope.hint">
              <template #connectionName
                ><span class="font-data">{{ connectionName }}</span></template
              >
            </CatalogText></FieldDescription
          >
        </template>
        <Button variant="dialog" size="kira-lg" @click="filtersDialogStore.closeFiltersDialog">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="filters-save"
          :disabled="!!loadError"
          :pending="loading || saving"
          @click="onSave"
          >{{
            t('common.action.save.label')
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
