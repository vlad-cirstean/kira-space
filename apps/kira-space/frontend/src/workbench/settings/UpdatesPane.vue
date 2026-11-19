<script setup lang="ts">
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
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
      :data-testid="`settings-updates-${group.kind}`"
    >
      <FieldLegend>
        {{ t(`space.settings.updates.${group.kind}.title`) }}
        <template #actions>
          <ResetToDefaultButton
            :data-testid="`settings-reset-tracking-${group.kind}`"
            :at-default="isAtDefault('tracking', group.kind)"
            @click="resetLeaf('tracking', group.kind)"
          />
        </template>
      </FieldLegend>
      <FieldDescription>{{
        t(`space.settings.updates.${group.kind}.description`)
      }}</FieldDescription>
      <Field v-for="c in group.concepts" :key="c.id" orientation="horizontal">
        <Switch
          :id="`${idBase}-${group.kind}-${c.id}`"
          :model-value="draft.tracking[group.kind][c.id] === true"
          :data-testid="`settings-updates-${group.kind}-${c.id}`"
          @update:model-value="(v) => toggle(group.kind, c.id, v === true)"
        />
        <FieldContent>
          <FieldLabel :for="`${idBase}-${group.kind}-${c.id}`">{{
            conceptText(group.kind, c.id, 'label')
          }}</FieldLabel>
          <FieldDescription>{{ conceptText(group.kind, c.id, 'description') }}</FieldDescription>
        </FieldContent>
      </Field>
    </FieldSet>
  </div>
</template>
