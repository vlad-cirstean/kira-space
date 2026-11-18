<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@theme/components/ui/input-group';
import { t } from '@theme/i18n';
import { useTemplateRef } from 'vue';

// The one search/filter text box. Attrs (data-testid, @keydown, @keydown.enter, ...) land
// on the <input>. Esc clears a non-empty value and stops there; an empty field lets Esc bubble so
// the surrounding surface (dialog, popover, find bar) still closes. Callers focus via the exposed
// focus() (biome noAutofocus forbids the attribute).
defineOptions({ inheritAttrs: false });

const _props = withDefaults(
  defineProps<{
    placeholder?: string;
    ariaLabel?: string;
    size?: 'kira' | 'kira-lg';
  }>(),
  {
    placeholder: () => t('common.action.search.label'),
    ariaLabel: undefined,
    size: 'kira',
  },
);

const model = defineModel<string>({ required: true });

const input = useTemplateRef<{ $el: HTMLInputElement }>('input');

function focus(): void {
  input.value?.$el.focus();
}

function onEscape(e: KeyboardEvent): void {
  if (e.isComposing || e.defaultPrevented || model.value === '') return;
  e.stopPropagation();
  model.value = '';
}

function clear(): void {
  model.value = '';
  focus();
}

defineExpose({ focus });
</script>

<template>
  <InputGroup :variant="size" data-slot="search-field">
    <InputGroupAddon align="inline-start">
      <CodiconIcon name="search" :size="13" />
    </InputGroupAddon>
    <InputGroupInput
      ref="input"
      v-bind="$attrs"
      :model-value="model"
      :placeholder="placeholder"
      :aria-label="ariaLabel ?? placeholder"
      :size="size"
      @update:model-value="model = String($event)"
      @keydown.escape="onEscape"
    />
    <InputGroupAddon v-if="model !== ''" align="inline-end">
      <InputGroupButton
        size="icon-xs"
        class="size-4"
        :aria-label="t('common.action.clearSearch.label')"
        @click="clear"
      >
        <CodiconIcon name="close" :size="12" />
      </InputGroupButton>
    </InputGroupAddon>
  </InputGroup>
</template>
