<script setup lang="ts">
import type { MemoryOrigin, MemoryScope } from '@shared/domain/memory';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { PopoverFooter } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import MenuItems from '../components/MenuItems.vue';
import type { MenuItem } from '../state/contextMenu';
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

const filterItems = computed<MenuItem[]>(() => [
  { type: 'label', label: t('common.field.scope.label') },
  ...SCOPES.map(
    (scope): MenuItem => ({
      type: 'checkbox',
      id: `scope-${scope}`,
      testId: `memory-filter-scope-${scope}`,
      label: t(SCOPE_LABEL[scope]),
      checked: checked(scopes.value, scope),
      keepOpen: true,
      run: () => {
        scopes.value = toggle(SCOPES, scopes.value, scope);
      },
    }),
  ),
  { type: 'separator' },
  { type: 'label', label: t('memory.import.confirm.origin.label') },
  ...ORIGINS.map(
    (origin): MenuItem => ({
      type: 'checkbox',
      id: `origin-${origin}`,
      testId: `memory-filter-origin-${origin}`,
      label: t(ORIGIN_LABEL[origin]),
      checked: checked(origins.value, origin),
      keepOpen: true,
      run: () => {
        origins.value = toggle(ORIGINS, origins.value, origin);
      },
    }),
  ),
]);

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
          <CodiconIcon name="filter" :size="12" />
          {{ t('memory.panel.filter.label') }}
          <Badge v-if="active > 0" data-testid="memory-filter-count">{{ active }}</Badge>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <MenuItems :items="filterItems" />
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
        <PopoverFooter v-if="project">
          <Button
            variant="toolbar"
            size="kira"
            data-testid="memory-boost-project-clear"
            @click="clear"
            ><CodiconIcon :name="ICON.clearAll" :size="13" />
            {{ t('memory.panel.boostProject.clear.label') }}
          </Button>
        </PopoverFooter>
      </template>
    </ProjectPicker>
  </div>
</template>
