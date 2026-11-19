<script setup lang="ts">
import { t } from '@theme/i18n';
import PendingToast from '@workbench/components/PendingToast.vue';
import { usePromptVisibility } from './promptsQueries';

// Stands in for a routed prompt while the user types; Show opens it at once.
const { deferred, release } = usePromptVisibility();
</script>

<template>
  <PendingToast
    v-if="deferred"
    testid="prompts-toast"
    show-testid="prompts-toast-show"
    @show="release"
  >
    <span class="truncate" data-testid="prompts-toast-title">{{ deferred.entry.title }}</span>
    <span
      v-if="deferred.more > 0"
      class="text-kira-sm text-muted-foreground"
      data-testid="prompts-toast-more"
    >
      {{ t('common.moreWaiting.status', { count: deferred.more }) }}
    </span>
    <span class="text-kira-sm text-muted-foreground" data-testid="prompts-toast-typing">
      {{ t('workbench.prompts.toast.typing.status') }}
    </span>
  </PendingToast>
</template>
