<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import AdeSessionsList from './AdeSessionsList.vue';
import type { SessionView } from './sessionView';

// Dropdown of every session in scope, Running then Finished or stopped: the switcher that reaches
// tabs the strip scrolled out of sight. `selectedId` is the row a reveal checks; `currentId` the
// open tab. The trigger flags a running session, other than the open tab, that needs input.
const props = defineProps<{
  running: SessionView[];
  stopped: SessionView[];
  selectedId: string | null;
  currentId: string | null;
}>();
const open = defineModel<boolean>('open', { default: false });

const count = computed(() => props.running.length + props.stopped.length);
const attention = computed(() =>
  props.running.some((v) => v.kind === 'input' && v.session.id !== props.currentId),
);
</script>

<template>
  <Popover v-if="count" v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="toolbar"
        size="kira-lg"
        class="shrink-0 gap-1"
        data-testid="ade-sessions-menu-trigger"
      >
        <span v-if="attention" class="inline-flex" data-testid="ade-sessions-menu-attention"
          ><AdeActivityIcon kind="input" bare /></span
        >
        {{ t('space.common.sessions.label') }}
        <Badge variant="count" data-testid="ade-sessions-menu-count">{{ count }}</Badge>
        <CodiconIcon name="chevron-down" :size="12" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="end" class="w-120 p-0" data-testid="ade-sessions-menu-popover">
      <AdeSessionsList
        :running="running"
        :stopped="stopped"
        :selected-id="selectedId"
        @done="open = false"
      />
    </PopoverContent>
  </Popover>
</template>
