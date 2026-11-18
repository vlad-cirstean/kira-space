<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

// Review pane failure: EmptyState plus a toolbar Retry beside it (side-panel EmptyState takes no
// button, U78). `retryTestid` keeps each call site's own Retry hook.
defineProps<{ description: string; retryTestid: string }>();
defineEmits<(e: 'retry') => void>();
</script>

<template>
  <div class="flex h-full flex-col items-center justify-center gap-3">
    <EmptyState
      icon="warning"
      :title="t('common.state.loadFailed.title')"
      :description="description"
    />
    <Button size="kira" variant="toolbar" :data-testid="retryTestid" @click="$emit('retry')">
      <CodiconIcon name="refresh" :size="13" />
      {{ t('common.action.retry.label') }}
    </Button>
  </div>
</template>
