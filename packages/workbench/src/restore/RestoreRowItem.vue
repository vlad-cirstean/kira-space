<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldError } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import type { RestoreRow } from './types';

// One listed row of the restore dialog: a checkbox when selectable, a spinner while it starts on its own.
defineProps<{ row: RestoreRow; checked: boolean; optionOn?: boolean }>();
const emit = defineEmits<{ toggle: [on: boolean]; toggleOption: [on: boolean] }>();
</script>

<template>
  <!-- The option sits outside the row label: a click on it must not toggle the row. -->
  <component
    :is="row.state === 'ready' ? Label : 'div'"
    :for="row.state === 'ready' ? `restore-${row.id}` : undefined"
    :class="[
      'flex w-full items-start gap-2 px-3 py-2 text-kira-md font-normal leading-normal',
      row.state === 'ready' ? 'cursor-default hover:bg-field' : 'text-muted-foreground',
    ]"
  >
    <Checkbox
      v-if="row.state === 'ready'"
      :id="`restore-${row.id}`"
      class="mt-0.5"
      :model-value="checked"
      data-testid="restore-row-toggle"
      @update:model-value="(v) => emit('toggle', v === true)"
    />
    <Spinner v-else label="" class="mt-0.5" />
    <div class="flex min-w-0 flex-1 flex-col gap-0.5">
      <div class="flex min-w-0 items-center gap-1.5">
        <span class="min-w-0 truncate font-medium">{{ row.label }}</span>
        <Badge v-for="b in row.badges" :key="b">{{ b }}</Badge>
        <span v-if="row.state === 'waiting' && row.reason" class="ml-auto shrink-0 text-kira-sm">{{
          row.reason
        }}</span>
        <span
          v-else-if="row.toggleLabel"
          class="ml-auto shrink-0 text-kira-sm text-muted-foreground"
          data-testid="restore-row-toggle-label"
          >{{
            row.toggleLabel
          }}</span
        >
      </div>
      <div
        v-if="row.detail || row.folder"
        class="flex min-w-0 items-center gap-3 font-data text-kira-sm text-muted-foreground"
      >
        <TruncatedText v-if="row.detail" :text="row.detail" class="min-w-0" />
        <span v-if="row.folder" class="flex min-w-0 shrink items-center gap-1">
          <CodiconIcon name="folder" :size="12" class="shrink-0" />
          <TruncatedText :text="row.folder" />
        </span>
      </div>
      <ul
        v-if="row.members?.length"
        class="m-0 flex list-none flex-col gap-0.5 p-0 pl-3 text-kira-sm text-muted-foreground"
      >
        <li v-for="m in row.members" :key="m.id" data-testid="restore-member" :data-id="m.id">
          {{ m.label }}
          <FieldError v-if="m.error">{{ m.error }}</FieldError>
        </li>
      </ul>
      <FieldError v-if="row.error" data-testid="restore-row-error">{{ row.error }}</FieldError>
    </div>
  </component>
  <Tooltip v-if="row.option && row.state === 'ready'" :disabled="!row.option.unsupportedReason">
    <TooltipTrigger as-child>
      <div class="flex items-center gap-2 px-3 pb-2 pl-9">
        <Checkbox
          :id="`restore-option-${row.id}`"
          :model-value="!!optionOn"
          :disabled="!checked || row.option.required || !!row.option.unsupportedReason"
          data-testid="restore-option-toggle"
          @update:model-value="(v) => emit('toggleOption', v === true)"
        />
        <Label
          :for="`restore-option-${row.id}`"
          class="text-kira-sm font-normal text-muted-foreground"
          >{{
            row.option.label
          }}</Label
        >
      </div>
    </TooltipTrigger>
    <TooltipContent v-if="row.option.unsupportedReason">{{
      row.option.unsupportedReason
    }}</TooltipContent>
  </Tooltip>
</template>
