<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Skeleton } from '@theme/components/ui/skeleton';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import TitleBarTitle from '@workbench/components/TitleBarTitle.vue';

// The chrome both views share: title bar (badge, title, ref, refresh / copy / open), the loading
// skeleton and the failure alert with Retry. The body is the default slot.
defineProps<{
  title: string;
  refText: string;
  fetching: boolean;
  pending: boolean;
  failure: string;
  copyLabel: string;
  openLabel: string;
  openDisabled?: boolean;
}>();
const emit = defineEmits<{ refresh: []; copy: []; open: [] }>();
</script>

<template>
  <div class="flex h-full flex-col bg-bg">
    <TitleBarBase>
      <div class="flex min-w-0 flex-1 items-center gap-2" data-testid="host-view-header">
        <slot name="badge" />
        <TitleBarTitle data-testid="host-view-title">{{ title }}</TitleBarTitle>
        <span
          class="shrink-0 font-data text-kira-sm text-muted-foreground"
          data-testid="host-view-ref"
        >
          {{ refText }}
        </span>
      </div>
      <div class="wails-no-drag flex shrink-0 items-center gap-1">
        <TooltipIconButton
          icon="refresh"
          :label="t('common.action.refresh.label')"
          :icon-size="16"
          variant="title"
          size="title"
          :disabled="fetching"
          data-testid="host-view-refresh"
          @click="emit('refresh')"
        />
        <TooltipIconButton
          icon="link"
          :label="copyLabel"
          :icon-size="16"
          variant="title"
          size="title"
          :disabled="openDisabled"
          data-testid="host-view-copy"
          @click="emit('copy')"
        />
        <TooltipIconButton
          icon="link-external"
          :label="openLabel"
          :icon-size="16"
          variant="title"
          size="title"
          :disabled="openDisabled"
          data-testid="host-view-open-host"
          @click="emit('open')"
        />
      </div>
    </TitleBarBase>

    <div v-if="pending" class="flex flex-col gap-3 p-3" data-testid="host-view-loading">
      <Skeleton class="h-control-lg w-1/3" />
      <Skeleton class="h-4 w-full" />
      <Skeleton class="h-4 w-2/3" />
    </div>
    <EmptyState
      v-else-if="failure"
      fill
      icon="warning"
      data-testid="host-view-failure"
      :title="t('common.state.loadFailed.title')"
      :description="failure"
    >
      <Button size="kira" variant="toolbar" data-testid="host-view-retry" @click="emit('refresh')"
        ><CodiconIcon :name="ICON.refresh" :size="13" />
        {{ t('common.action.retry.label') }}
      </Button>
    </EmptyState>
    <slot v-else />
  </div>
</template>
