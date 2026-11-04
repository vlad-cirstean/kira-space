<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { setStageValue, stageValueValid } from '../../board/stageSettings';
import { useWorkflows } from '../../queries';
import type { StageSettingSpec } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';

// A step's per-provider agent settings, one field per catalog entry.
const props = defineProps<{ settings: Record<string, Record<string, string>>; stepId: string }>();
const emit = defineEmits<{ patch: [settings: Record<string, Record<string, string>>] }>();
const workflows = useWorkflows();
const groups = computed(() => {
  const out = new Map<string, { label: string; specs: StageSettingSpec[] }>();
  for (const s of workflows.data.value?.stageSettings ?? []) {
    const g = out.get(s.provider) ?? { label: s.providerLabel, specs: [] };
    g.specs.push(s);
    out.set(s.provider, g);
  }
  return [...out].map(([provider, g]) => ({ provider, ...g }));
});
const value = (s: StageSettingSpec): string => props.settings[s.provider]?.[s.key] ?? '';
const key = (s: StageSettingSpec): string => `${s.provider}-${s.key}`;
const id = (s: StageSettingSpec): string => `ade-wf-setting-${key(s)}-${props.stepId}`;
const onInput = (s: StageSettingSpec, v: string | number): void =>
  emit('patch', setStageValue(props.settings, s.provider, s.key, String(v).trim()));
</script>

<template>
  <div v-if="groups.length > 0" class="flex flex-col gap-3" data-testid="ade-wf-step-settings">
    <template v-for="g in groups" :key="g.provider">
      <span v-if="groups.length > 1" class="text-kira-sm text-subtle">{{ g.label }}</span>
      <AdeInspectorRow v-for="s in g.specs" :key="s.key" :label="s.label" :for="id(s)">
        <Input
          :id="id(s)"
          type="number"
          :min="s.min"
          :max="s.max"
          :model-value="value(s)"
          :placeholder="t('common.state.default.option')"
          :aria-invalid="!stageValueValid(s, value(s)) || undefined"
          class="w-24 font-data"
          :data-testid="`ade-wf-setting-${key(s)}`"
          @update:model-value="(v: string | number) => onInput(s, v)"
        />
        <template v-if="!stageValueValid(s, value(s))" #error>
          <span :data-testid="`ade-wf-setting-error-${key(s)}`">{{
            t('space.ade.workflows.settings.range.error', { min: s.min, max: s.max })
          }}</span>
        </template>
        <template v-else #hint>
          <span :data-testid="`ade-wf-setting-hint-${key(s)}`">{{ s.hint }}</span>
        </template>
      </AdeInspectorRow>
    </template>
  </div>
</template>
