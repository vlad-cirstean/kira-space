<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { floatingSurface } from '@theme/lib/frames';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../bridge/control';
import { useConfigSyncMutations, useConfigSyncStatus } from './queries';

// Persists until acted on: Review opens the window, dismiss hides it until the next new commit.
const status = useConfigSyncStatus();
const { dismiss } = useConfigSyncMutations();

function onReview(): void {
  fireAndForget(control.configSyncOpenReview(), 'configSync');
}
function onDismiss(): void {
  const sha = status.data.value?.fetched;
  if (sha) dismiss.mutate(sha);
}
</script>

<template>
  <div
    v-if="status.data.value?.showToast"
    role="status"
    aria-live="polite"
    :class="[
      floatingSurface,
      'fixed right-4 bottom-10 z-(--kira-z-dialog) flex items-center gap-3 p-3',
    ]"
    data-testid="config-sync-toast"
  >
    <div class="flex flex-col">
      <span>{{ t('space.configSync.toast.title.status') }}</span>
      <span class="text-kira-sm text-muted-foreground" data-testid="config-sync-toast-count">
        {{ t('space.configSync.toast.count.status', { count: status.data.value.changes }) }}
      </span>
    </div>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      data-testid="config-sync-toast-review"
      @click="onReview"
    >
      {{ t('space.configSync.toast.review.label') }}
    </Button>
    <TooltipIconButton
      icon="close"
      :label="t('common.action.dismiss.label')"
      data-testid="config-sync-toast-dismiss"
      @click="onDismiss"
    />
  </div>
</template>
