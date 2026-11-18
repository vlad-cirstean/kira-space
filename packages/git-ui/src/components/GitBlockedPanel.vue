<script setup lang="ts">
/**
 * §4.2's block state: git too old, not found, or unusable. Nothing else in the UI renders while
 * this holds — "the app does not start into a degraded mode" — so `App.vue` (W11) shows this in
 * place of the toolbar and list entirely, not layered over them.
 */
import type { GitStatus } from '@kira/git-ipc';
import EmptyState from '@theme/components/EmptyState.vue';
import { computed } from 'vue';
import { STATE_ICONS } from '../icons/index.ts';
import { type BlockedGitStatus, detectPlatform, gitBlockedCopy } from '../lib/gitBlockedCopy.ts';

const props = defineProps<{ status: GitStatus }>();

const copy = computed(() => {
  if (props.status.kind === 'ok') return null;
  return gitBlockedCopy(props.status as BlockedGitStatus, detectPlatform(navigator.userAgent));
});
</script>

<template>
  <EmptyState
    v-if="copy"
    fill
    role="alert"
    :icon="STATE_ICONS.warning"
    data-testid="git-blocked-panel"
    :title="copy.title"
    :description="copy.detail"
  />
</template>
