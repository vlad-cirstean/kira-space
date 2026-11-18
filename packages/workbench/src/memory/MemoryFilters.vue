<script setup lang="ts">
import type { MemoryOrigin, MemoryScope } from '@shared/domain/memory';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { ORIGIN_LABEL, SCOPE_LABEL } from './labels';
import { useMemoryProjectOptions } from './queries';

// P275: search narrowing (scope and origin checkboxes) and the project whose facts rank first.
// Every box checked and an empty list both mean "all".
const scopes = defineModel<MemoryScope[]>('scopes', { required: true });
const origins = defineModel<MemoryOrigin[]>('origins', { required: true });
const project = defineModel<string>('project', { required: true });

const SCOPES: readonly MemoryScope[] = ['company', 'project', 'personal'];
const ORIGINS: readonly MemoryOrigin[] = ['user', 'agent', 'external'];

function toggle<T extends string>(all: readonly T[], current: T[], value: T): T[] {
  const set = new Set(current.length === 0 ? all : current);
  if (set.has(value)) set.delete(value);
  else set.add(value);
  const next = all.filter((v) => set.has(v));
  return next.length === all.length || next.length === 0 ? [] : next;
}

const checked = <T extends string>(current: T[], value: T): boolean =>
  current.length === 0 || current.includes(value);

const active = computed(
  () => (scopes.value.length > 0 ? 1 : 0) + (origins.value.length > 0 ? 1 : 0),
);

const projects = useMemoryProjectOptions();
const pickerOpen = ref(false);

function clear(): void {
  project.value = '';
  pickerOpen.value = false;
}
</script>

<template>
  <div class="flex items-center gap-1">
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <Button variant="toolbar" size="status" data-testid="memory-filter">
          <CodiconIcon name="filter" :size="13" />
          {{ t('memory.panel.filter.label') }}
          <Badge v-if="active > 0" data-testid="memory-filter-count">{{ active }}</Badge>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>{{ t('common.field.scope.label') }}</DropdownMenuLabel>
        <!-- checkbox: MenuItem has no checkbox kind -->
        <DropdownMenuCheckboxItem
          v-for="scope in SCOPES"
          :key="scope"
          :model-value="checked(scopes, scope)"
          :data-testid="`memory-filter-scope-${scope}`"
          @select.prevent
          @update:model-value="scopes = toggle(SCOPES, scopes, scope)"
        >
          {{ t(SCOPE_LABEL[scope]) }}
        </DropdownMenuCheckboxItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>{{ t('memory.import.confirm.origin.label') }}</DropdownMenuLabel>
        <DropdownMenuCheckboxItem
          v-for="origin in ORIGINS"
          :key="origin"
          :model-value="checked(origins, origin)"
          :data-testid="`memory-filter-origin-${origin}`"
          @select.prevent
          @update:model-value="origins = toggle(ORIGINS, origins, origin)"
        >
          {{ t(ORIGIN_LABEL[origin]) }}
        </DropdownMenuCheckboxItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <ProjectPicker
      v-model:open="pickerOpen"
      :model-value="project || null"
      custom
      size="kira"
      trigger="toolbar"
      class="max-w-48"
      testid="memory-boost-project"
      :projects="projects.options.value"
      :placeholder="t('memory.panel.boostProject.label')"
      :loading="projects.isPending.value"
      @update:model-value="(v) => (project = v ?? '')"
    >
      <template #footer>
        <Button
          v-if="project"
          variant="toolbar"
          size="status"
          class="w-full"
          data-testid="memory-boost-project-clear"
          @click="clear"
        >
          {{ t('memory.panel.boostProject.clear.label') }}
        </Button>
      </template>
    </ProjectPicker>
  </div>
</template>
