<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@theme/components/ui/input-group';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { useTimeoutFn, useToggle } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import {
  type ComponentPublicInstance,
  computed,
  onBeforeUnmount,
  ref,
  useAttrs,
  useTemplateRef,
  watch,
} from 'vue';

// Secret entry with an eye. It never receives a stored value: `set` only says whether one exists.
// The eye shows what is typed; a saved value shows only through `reveal` (a gated fetch), is held
// here alone (never emitted) and re-masks on hide, typing, unmount and after the reveal grace.
// `class` lands on the group; every other attribute (testid, aria-label, listeners) on the input.
defineOptions({ inheritAttrs: false });
const props = defineProps<{
  id?: string;
  set?: boolean;
  disabled?: boolean;
  placeholder?: string;
  /** Fetches the saved value; `undefined` means cancelled or failed (the caller reports it). */
  reveal?: () => Promise<string | undefined>;
}>();
const value = defineModel<string>({ required: true });
// A caller's v-model target can start undefined (a draft map entry).
const typed = computed(() => value.value ?? '');
const attrs = useAttrs();

const [visible, toggleVisible] = useToggle(false);
const savedPlain = ref<string | null>(null);
let seq = 0;
const inputRef = useTemplateRef<ComponentPublicInstance>('inputRef');
defineExpose({ input: computed(() => inputRef.value?.$el as HTMLInputElement | undefined) });

const grace = useTimeoutFn(() => drop(), TIMING.revealGrace, { immediate: false });

function drop(): void {
  seq++;
  grace.stop();
  savedPlain.value = null;
  visible.value = false;
}

const shown = computed(() => (typed.value !== '' ? typed.value : (savedPlain.value ?? '')));
const placeholder = computed(() => {
  if (props.placeholder !== undefined) return props.placeholder;
  if (props.set === undefined) return undefined;
  return props.set ? t('common.secret.saved.placeholder') : t('docker.edit.unset.status');
});
const notRevealable = computed(
  () => props.set === true && typed.value === '' && savedPlain.value === null && !props.reveal,
);
const eyeTestid = computed(() => {
  const id = attrs['data-testid'];
  return typeof id === 'string' ? `${id}-reveal` : 'secret-input-reveal';
});
const inputAttrs = computed(() => {
  const { class: _class, ...rest } = attrs;
  return rest;
});

async function onEye(): Promise<void> {
  if (savedPlain.value !== null) return drop();
  if (typed.value !== '' || props.set !== true) return void toggleVisible();
  if (!props.reveal) return;
  const mine = ++seq;
  const plain = await props.reveal();
  if (mine !== seq || plain === undefined) return;
  savedPlain.value = plain;
  visible.value = true;
  grace.start();
}

watch(typed, (v) => {
  if (v !== '' && savedPlain.value !== null) drop();
  else if (v === '' && savedPlain.value === null) visible.value = false;
});
watch([() => props.set, () => !!props.reveal], () => drop());
watch(
  () => props.disabled,
  (d) => d && drop(),
);
onBeforeUnmount(drop);
</script>

<template>
  <InputGroup :class="cn('w-full', attrs.class as string | undefined)">
    <InputGroupInput
      v-bind="inputAttrs"
      ref="inputRef"
      :id="id"
      :model-value="shown"
      :type="visible ? 'text' : 'password'"
      autocomplete="new-password"
      autocapitalize="off"
      :spellcheck="false"
      class="font-data"
      :readonly="savedPlain !== null"
      :disabled="disabled"
      :placeholder="placeholder"
      @update:model-value="(v: string | number) => (value = String(v))"
    />
    <InputGroupAddon align="inline-end">
      <TooltipIconButton
        :icon="visible ? 'eye-closed' : 'eye'"
        :label="
          notRevealable
            ? t('common.secret.notRevealable.tooltip')
            : visible
              ? t('common.hideValue.label')
              : t('common.secret.show.label')
        "
        :aria-label="visible ? t('common.hideValue.label') : t('common.secret.show.label')"
        :pressed="visible"
        :disabled="disabled || notRevealable"
        disabled-trigger
        :data-testid="eyeTestid"
        type="button"
        @click="onEye"
      />
    </InputGroupAddon>
  </InputGroup>
</template>
