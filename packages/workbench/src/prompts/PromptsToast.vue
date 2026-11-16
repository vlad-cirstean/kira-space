<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { floatingSurface } from '@theme/lib/frames';
import { usePromptVisibility } from './promptsQueries';

// Stands in for a routed prompt while the user types; Show opens it at once.
const { deferred, release } = usePromptVisibility();
</script>

<template>
  <div
    v-if="deferred"
    role="status"
    aria-live="polite"
    :class="[floatingSurface, 'flex items-center gap-3 p-3']"
    data-testid="prompts-toast"
  >
    <div class="flex min-w-0 flex-col">
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
    </div>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      data-testid="prompts-toast-show"
      @click="release"
    >
      {{ t('common.action.show.label') }}
    </Button>
  </div>
</template>
