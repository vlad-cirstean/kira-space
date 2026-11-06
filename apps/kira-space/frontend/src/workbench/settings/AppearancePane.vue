<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLegend,
} from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { ROW_HEIGHT_PX } from '@workbench/lib/rowHeight';
import FontFamilyField from '@workbench/settings/fields/FontFamilyField.vue';
import FontSizeField from '@workbench/settings/fields/FontSizeField.vue';
import RowDensityField from '@workbench/settings/fields/RowDensityField.vue';
import WordWrapField from '@workbench/settings/fields/WordWrapField.vue';
import { computed, useId } from 'vue';
import DateFormatField from './DateFormatField.vue';
import RowDensityPreview from './RowDensityPreview.vue';
import type { SettingsPaneProps } from './types';

// Trimmed to typography + repo-file-viewer leaves (no row density preview, no data-grid page
// size, none of which this app has a feature for).
const props = defineProps<SettingsPaneProps>();

function onRowColoringChange(checked: boolean): void {
  props.draft.appearance.rowColoring = checked;
}

const rowPreviewHeight = computed(() => ROW_HEIGHT_PX[props.draft.appearance.rowDensity]);

function onInlineBlameChange(checked: boolean): void {
  props.draft.appearance.inlineBlame = checked;
}

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const inlineBlameId = useId();
const rowColoringId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <FieldLegend class="pt-0">{{ t('common.typography.label') }}</FieldLegend>
    <FontFamilyField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    />

    <FontSizeField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
      :register-field-error="registerFieldError"
    />

    <RowDensityField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    >
      <FieldDescription>{{
        t('space.settings.appearance.rowDensity.description')
      }}</FieldDescription>
      <RowDensityPreview :height-px="rowPreviewHeight" />
    </RowDensityField>

    <WordWrapField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    >
      <FieldDescription>{{ t('space.settings.appearance.wordWrap.description') }}</FieldDescription>
    </WordWrapField>

    <FieldGroup>
      <Field orientation="horizontal" class="items-start">
        <Switch
          :id="rowColoringId"
          :model-value="draft.appearance.rowColoring"
          data-testid="settings-row-coloring"
          @update:model-value="(v) => onRowColoringChange(v === true)"
        />
        <FieldContent>
          <Label :for="rowColoringId">{{
            t('studio.settings.appearance.rowColoring.label')
          }}</Label>
          <FieldDescription>{{
            t('studio.settings.appearance.rowColoring.description')
          }}</FieldDescription>
        </FieldContent>
      </Field>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        class="ml-auto"
        data-testid="settings-reset-appearance-rowColoring"
        :disabled-trigger="isAtDefault('appearance', 'rowColoring')"
        :disabled="isAtDefault('appearance', 'rowColoring')"
        @click="resetLeaf('appearance', 'rowColoring')"
      />
    </FieldGroup>

    <FieldGroup>
      <Field orientation="horizontal" class="items-start">
        <Switch
          :id="inlineBlameId"
          :model-value="draft.appearance.inlineBlame"
          data-testid="settings-inline-blame"
          @update:model-value="(v) => onInlineBlameChange(v === true)"
        />
        <FieldContent>
          <Label :for="inlineBlameId">{{ t('space.settings.appearance.inlineBlame.label') }}</Label>
          <FieldDescription>{{
            t('space.settings.appearance.inlineBlame.description')
          }}</FieldDescription>
        </FieldContent>
      </Field>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        class="ml-auto"
        data-testid="settings-reset-appearance-inlineBlame"
        :disabled-trigger="isAtDefault('appearance', 'inlineBlame')"
        :disabled="isAtDefault('appearance', 'inlineBlame')"
        @click="resetLeaf('appearance', 'inlineBlame')"
      />
    </FieldGroup>

    <DateFormatField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    />
  </div>
</template>
