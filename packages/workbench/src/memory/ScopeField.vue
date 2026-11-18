<script setup lang="ts">
import type { MemoryScope } from '@shared/domain/memory';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed } from 'vue';
import { SCOPE_LABEL } from './labels';
import { useMemoryProjectOptions } from './queries';

// Scope tabs plus, for scope project, the project combobox. Test ids are `<prefix>-scope-*` and `<prefix>-project-*`.
const props = defineProps<{ prefix: string; scopes: readonly MemoryScope[] }>();
const scope = defineModel<MemoryScope>('scope', { required: true });
const project = defineModel<string>('project', { required: true });

const projects = useMemoryProjectOptions();
const scopeItems = computed(() =>
  props.scopes.map((value) => ({
    value,
    label: t(SCOPE_LABEL[value]),
    testid: `${props.prefix}-scope-${value}`,
  })),
);
</script>

<template>
  <Field>
    <FieldLabel>{{ t('common.field.scope.label') }}</FieldLabel>
    <SecondaryTabs
      :model-value="scope"
      :items="scopeItems"
      variant="choice"
      size="kira-lg"
      :aria-label="t('common.field.scope.label')"
      :data-testid="`${prefix}-scope`"
      @update:model-value="(v) => (scope = v as MemoryScope)"
    />
  </Field>
  <Field v-if="scope === 'project'">
    <FieldLabel :for="`${prefix}-project`">{{ t('memory.common.project.label') }}</FieldLabel>
    <ProjectPicker
      :id="`${prefix}-project`"
      :model-value="project || null"
      custom
      :testid="`${prefix}-project`"
      :projects="projects.options.value"
      :placeholder="t('memory.add.project.placeholder')"
      :loading="projects.isPending.value"
      @update:model-value="(v) => (project = v ?? '')"
    />
  </Field>
</template>
