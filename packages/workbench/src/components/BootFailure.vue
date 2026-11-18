<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

// main.ts's own boot-failure fallback, both apps — mounted in place of App.vue when
// bootstrap()'s own hydrates reject, so a bad DB read or a busy DB shows this instead of a
// permanently blank window with the rejection only visible in the webview console. Deliberately
// standalone (no workbench/theme host, no Pinia store reads beyond the two shadcn-vue primitives
// already safe to use un-hydrated) — the stores that failed to hydrate cannot be trusted to render
// anything else. `appName` is the one thing kira-space's and kira-studio's own copies differed in.
defineProps<{ appName: string; message: string }>();
const emit = defineEmits<{ retry: [] }>();
</script>

<template>
  <div class="h-full flex items-center justify-center overflow-auto p-3" data-testid="boot-failure">
    <div class="w-105 max-w-full">
      <EmptyState
        icon="warning"
        :title="t('workbench.bootFailure.title', { product: appName })"
        :description="message"
        class="whitespace-pre-wrap"
      >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="boot-failure-retry"
          @click="emit('retry')"
        >
          <CodiconIcon name="refresh" :size="13" />
          {{ t('common.action.retry.label') }}
        </Button>
      </EmptyState>
    </div>
  </div>
</template>
