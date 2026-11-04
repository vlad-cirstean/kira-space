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
import type { SettingsPaneProps } from './types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Appearance'"` branch — markup, classes and data-testids unchanged.
const props = defineProps<SettingsPaneProps>();

function onRowColoringChange(checked: boolean): void {
  props.draft.appearance.rowColoring = checked;
}

// Fake sample data for the density preview, not wording.
const PREVIEW = {
  idColumn: 'id',
  emailColumn: 'email',
  rows: [
    { id: 'c1d0-88ae', email: 'rowan.brooks@example.com' },
    { id: '7f2b-19cd', email: 'amari.osei@example.com' },
  ],
};

const rowPreviewHeight = computed(() => ROW_HEIGHT_PX[props.draft.appearance.rowDensity]);

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
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
        t('studio.settings.appearance.rowDensity.description')
      }}</FieldDescription>
      <!-- Every cell below carries the full computed style of the old shared
           .row-preview-cell/.row-preview-gutter/.row-preview-grow/.row-preview-head rules, resolved
           per-instance rather than stacking conflicting same-property utilities -- the head
           row's `.row-preview-head .row-preview-cell` descendant selector outranked the gutter/grow
           siblings' own color/font-size on specificity alone, and .row-preview-grow's flex-1 always
           beat .row-preview-cell's own flex-basis by source order. That descendant selector's own
           `font-family: inherit` is dropped rather than ported: none of these 3 header cells carry
           font-data (unlike their data-row counterparts below), so they already inherit naturally
           -- porting `inherit` as a class would re-state the default. -->
      <div class="overflow-hidden rounded-kira-sm border border-border">
        <div class="flex bg-elevated border-b border-border">
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-muted-foreground text-kira-sm bg-elevated"
            :style="{ height: `${rowPreviewHeight}px` }"
          />
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-muted-foreground text-kira-sm"
            :style="{ height: `${rowPreviewHeight}px` }"
            >{{
              PREVIEW.idColumn
            }}</span
          >
          <span
            class="flex items-center truncate px-2 border-r border-border flex-1 text-muted-foreground text-kira-sm"
            :style="{ height: `${rowPreviewHeight}px` }"
            >{{
              PREVIEW.emailColumn
            }}</span
          >
        </div>
        <div class="flex" :style="{ height: `${rowPreviewHeight}px` }">
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-subtle text-kira-sm bg-elevated font-data"
            >1</span
          >
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-fg text-kira-md font-data"
            >{{
              PREVIEW.rows[0].id
            }}</span
          >
          <span
            class="flex items-center truncate px-2 border-r border-border flex-1 text-fg text-kira-md font-data"
            >{{
              PREVIEW.rows[0].email
            }}</span
          >
        </div>
        <div class="flex border-t border-border" :style="{ height: `${rowPreviewHeight}px` }">
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-9 justify-end text-subtle text-kira-sm bg-elevated font-data"
            >2</span
          >
          <span
            class="flex items-center truncate px-2 border-r border-border flex-none basis-37.5 text-fg text-kira-md font-data"
            >{{
              PREVIEW.rows[1].id
            }}</span
          >
          <span
            class="flex items-center truncate px-2 border-r border-border flex-1 text-fg text-kira-md font-data"
            >{{
              PREVIEW.rows[1].email
            }}</span
          >
        </div>
      </div>
    </RowDensityField>

    <WordWrapField
      :appearance="draft.appearance"
      :is-at-default="isAtDefault"
      :reset-leaf="resetLeaf"
    >
      <FieldDescription>{{
        t('studio.settings.appearance.wordWrap.description')
      }}</FieldDescription>
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
  </div>
</template>
