<script setup lang="ts">
import type { CommitStore } from '@kira/git-core';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, useId } from 'vue';
import { ACTION_ICONS } from '../icons/index.ts';
import { shaLabel } from '../lib/text.ts';
import type { FileListMode } from '../state/detail.ts';

// The merge-parent selector, the file filter and the Tree/Flat toggle above a file tree. The filter
// and toggle hide when a panel above owns one toolbar for several trees (`showToolbar` false).
const props = defineProps<{
  parents: readonly string[];
  store: CommitStore;
  showToolbar?: boolean;
}>();
const parentIndex = defineModel<number>('parentIndex', { required: true });
const listMode = defineModel<FileListMode>('listMode', { required: true });
const filter = defineModel<string>('filter', { required: true });

const listModeItems = computed(() => {
  const options: readonly { id: FileListMode; icon: string; label: string }[] = [
    { id: 'tree', icon: ACTION_ICONS.listTree, label: t('git.fileTree.listTree.tooltip') },
    { id: 'flat', icon: ACTION_ICONS.listFlat, label: t('git.fileTree.listFlat.tooltip') },
  ];
  return options.map((o) => ({ value: o.id, label: o.label, icon: o.icon, tooltip: o.label }));
});

const parentSelectOptions = computed(() =>
  props.parents.map((sha, index) => {
    const row = props.store.rowOfSha(sha);
    const subject = row !== -1 ? props.store.subjectAt(row) : undefined;
    const sha7 = shaLabel(sha);
    const label = subject
      ? t('git.fileTree.parent.label', { number: index + 1, sha: sha7, subject })
      : t('git.fileTree.parentNoSubject.label', { number: index + 1, sha: sha7 });
    return { value: String(index), label };
  }),
);

// biome's noLabelWithoutControl can't see through NativeSelect's `inheritAttrs: false`: an explicit
// for/id pair keeps the association.
const parentSelectId = useId();
</script>

<template>
  <Field v-if="parents.length > 1" class="gap-0.5 px-3 pb-2 text-graph-md">
    <FieldLabel :for="parentSelectId">{{ t('git.fileTree.diffAgainst.label') }}</FieldLabel>
    <NativeSelect
      :id="parentSelectId"
      :model-value="String(parentIndex)"
      variant="bordered"
      size="kira"
      @update:model-value="(value) => (parentIndex = Number(value))"
    >
      <option v-for="option in parentSelectOptions" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </NativeSelect>
  </Field>

  <div v-if="showToolbar !== false" class="flex gap-1 px-2 pb-1">
    <div class="min-w-0 flex-1">
      <SearchField
        v-model="filter"
        data-testid="file-filter"
        :placeholder="t('git.fileTree.filter.placeholder')"
        :aria-label="t('git.fileTree.filter.placeholder')"
      />
    </div>
    <SecondaryTabs
      variant="segmented"
      :aria-label="t('git.fileTree.display.aria')"
      :model-value="listMode"
      :items="listModeItems"
      @update:model-value="(v) => (listMode = v as FileListMode)"
    >
      <template #item="{ item }">
        <CodiconIcon :name="item.icon ?? ''" :size="13" />
        <span class="sr-only">{{ item.label }}</span>
      </template>
    </SecondaryTabs>
  </div>
</template>
