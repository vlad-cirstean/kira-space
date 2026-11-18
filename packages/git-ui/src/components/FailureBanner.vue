<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
/**
 * The visible half of every failure the live region already announces. An inline banner,
 * not a toast: it stays until dismissed or superseded, pushes layout instead of covering grid
 * rows, and needs no toast host in either app. `role="region"` replaces `Alert`'s own
 * `role="alert"` so the shared `sr-only` live region stays the single announcer.
 */
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { FailureNotice } from '../state/failureNotice.ts';

const props = defineProps<{
  notice: FailureNotice;
  /** Present only where the host can open Kira Space's Operations dock. */
  showOperations?: (() => void) | undefined;
}>();
const emit = defineEmits<{ dismiss: []; retry: [] }>();

const detail = computed(() => props.notice.detail);
const showLogHint = computed(() => props.notice.logged && props.showOperations === undefined);
const canShowOperations = computed(() => props.notice.logged && props.showOperations !== undefined);
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: Alert's root is a themed div; role="region" only replaces its role="alert" -->
  <Alert
    variant="error"
    strip
    class="shrink-0"
    role="region"
    :aria-label="t('git.failure.banner.aria')"
    data-testid="failure-banner"
  >
    <AlertTitle>{{ notice.title }}</AlertTitle>
    <AlertDescription v-if="detail" class="whitespace-pre-wrap">{{ detail }}</AlertDescription>
    <AlertDescription data-testid="failure-banner-hint"
      >{{ notice.hint
      }}<template v-if="showLogHint">
        {{ t('git.failure.hintLogged.hint') }}</template
      ></AlertDescription
    >
    <AlertAction class="flex items-center gap-1">
      <Button
        v-if="notice.retryGraph"
        variant="dialog"
        size="kira"
        data-testid="failure-banner-retry"
        @click="emit('retry')"
      >
        {{ t('common.action.retry.label') }}
      </Button>
      <Button
        v-if="canShowOperations"
        variant="dialog"
        size="kira"
        data-testid="failure-banner-operations"
        @click="showOperations?.()"
      >
        {{ t('git.failure.showOperations.label') }}
      </Button>
      <TooltipIconButton
        icon="close"
        :label="t('common.action.dismiss.label')"
        data-testid="failure-banner-dismiss"
        @click="emit('dismiss')"
      />
    </AlertAction>
  </Alert>
</template>
