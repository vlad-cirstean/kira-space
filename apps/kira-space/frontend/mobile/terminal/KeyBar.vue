<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { type MessageKey, t } from '@theme/i18n';

// Keys a soft keyboard lacks. Arrows honour the application cursor mode Claude Code may enable.
const props = defineProps<{ appCursor: boolean }>();
const ctrl = defineModel<boolean>('ctrl', { required: true });
const emit = defineEmits<{ key: [seq: string] }>();

const arrow = (letter: string): string => (props.appCursor ? `\x1bO${letter}` : `\x1b[${letter}`);

const KEYS: { id: string; label: MessageKey; icon?: string; seq: () => string }[] = [
  { id: 'esc', label: 'common.key.escape.label', seq: () => '\x1b' },
  { id: 'tab', label: 'common.key.tab.label', seq: () => '\t' },
  { id: 'shift-tab', label: 'mobile.terminal.keys.shiftTab.label', seq: () => '\x1b[Z' },
  { id: 'ctrl-c', label: 'mobile.terminal.keys.ctrlC.label', seq: () => '\x03' },
  { id: 'up', label: 'mobile.terminal.keys.up.label', icon: 'arrow-up', seq: () => arrow('A') },
  {
    id: 'down',
    label: 'mobile.terminal.keys.down.label',
    icon: 'arrow-down',
    seq: () => arrow('B'),
  },
  {
    id: 'left',
    label: 'mobile.terminal.keys.left.label',
    icon: 'arrow-left',
    seq: () => arrow('D'),
  },
  {
    id: 'right',
    label: 'mobile.terminal.keys.right.label',
    icon: 'arrow-right',
    seq: () => arrow('C'),
  },
  { id: 'enter', label: 'common.key.enter.label', seq: () => '\r' },
];
</script>

<template>
  <div class="flex gap-1 overflow-x-auto px-2 py-1 scrollbar-none" data-testid="term-keybar">
    <Button
      size="touch"
      variant="dialog"
      class="shrink-0"
      :aria-pressed="ctrl"
      data-testid="term-key-ctrl"
      @click="ctrl = !ctrl"
    >
      {{ t('mobile.terminal.keys.ctrl.label') }}
    </Button>
    <Button
      size="touch"
      v-for="key in KEYS"
      :key="key.id"
      variant="dialog"
      class="shrink-0"
      :aria-label="t(key.label)"
      :data-testid="`term-key-${key.id}`"
      @click="emit('key', key.seq())"
    >
      <CodiconIcon v-if="key.icon" :name="key.icon" :size="13" />
      <template v-else>{{ t(key.label) }}</template>
    </Button>
  </div>
</template>
