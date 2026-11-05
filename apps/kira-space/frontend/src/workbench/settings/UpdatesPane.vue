<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldDescription, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { type MessageKey, t } from '@theme/i18n';
import { useId } from 'vue';
import { TRACKED_KINDS, type TrackedKind } from '../../state/trackingConcepts';
import type { SettingsPaneProps } from './types';

// Which changes to a tracked item count as an update worth flagging, per concept. The draft holds
// one boolean map per kind; a toggle replaces the map so a reset never aliases the defaults.
const props = defineProps<SettingsPaneProps>();

const idBase = useId();

function conceptText(kind: TrackedKind, id: string, part: 'label' | 'description'): string {
  return t(`space.settings.updates.${kind}.${id}.${part}` as MessageKey);
}

function toggle(kind: TrackedKind, id: string, value: boolean): void {
  props.draft.tracking[kind] = { ...props.draft.tracking[kind], [id]: value };
}
</script>

<template>
  <div class="contents" v-show="active">
    <FieldDescription>{{ t('space.settings.updates.intro.description') }}</FieldDescription>
    <FieldSet
      v-for="group in TRACKED_KINDS"
      :key="group.kind"
      class="gap-3"
      :data-testid="`settings-updates-${group.kind}`"
    >
      <div class="flex items-center justify-between gap-1">
        <FieldLegend>{{ t(`space.settings.updates.${group.kind}.title`) }}</FieldLegend>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          :data-testid="`settings-reset-tracking-${group.kind}`"
          :disabled-trigger="isAtDefault('tracking', group.kind)"
          :disabled="isAtDefault('tracking', group.kind)"
          @click="resetLeaf('tracking', group.kind)"
        />
      </div>
      <FieldDescription>{{
        t(`space.settings.updates.${group.kind}.description`)
      }}</FieldDescription>
      <Field
        v-for="c in group.concepts"
        :key="c.id"
        orientation="horizontal"
        class="items-start justify-between"
      >
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <Label :for="`${idBase}-${group.kind}-${c.id}`">{{
            conceptText(group.kind, c.id, 'label')
          }}</Label>
          <FieldDescription>{{ conceptText(group.kind, c.id, 'description') }}</FieldDescription>
        </div>
        <Switch
          :id="`${idBase}-${group.kind}-${c.id}`"
          :model-value="draft.tracking[group.kind][c.id] === true"
          :data-testid="`settings-updates-${group.kind}-${c.id}`"
          @update:model-value="(v) => toggle(group.kind, c.id, v === true)"
        />
      </Field>
    </FieldSet>
  </div>
</template>
