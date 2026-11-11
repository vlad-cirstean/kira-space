<script setup lang="ts">
import AdeActivityIcon from '@ade/AdeActivityIcon.vue';
import AdeChip from '@ade/AdeChip.vue';
import { activityKind } from '@ade/activity';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Progress } from '@theme/components/ui/progress';
import { Textarea } from '@theme/components/ui/textarea';
import { formatPercent } from '@theme/format';
import { t } from '@theme/i18n';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { useAuthStore } from '../state/auth';
import type { PlanCard } from '../state/useAgentsModel';
import { useTaskActions } from '../state/useTaskActions';

// One planned task: status, current step and progress, expanding to every step.
const props = defineProps<{ card: PlanCard; open: boolean }>();
defineEmits<{ toggle: [] }>();

const { permissions } = storeToRefs(useAuthStore());
const actions = useTaskActions(() => props.card);
const canMove = computed(() => permissions.value.write && props.card.blocks.length > 0);
const stopFirst = computed(() => props.card.moves.live);
</script>

<template>
  <li class="border-b border-border" data-testid="plan-task" :data-status="card.status.id">
    <button
      type="button"
      class="flex w-full items-start gap-3 bg-transparent px-3 py-3 text-left text-fg"
      :aria-expanded="open"
      @click="$emit('toggle')"
    >
      <StatusDot class="mt-1.5" :color="card.color" />
      <span class="flex min-w-0 flex-1 flex-col gap-1">
        <span class="text-kira-md">{{ card.title }}</span>
        <span class="flex flex-wrap items-center gap-1.5 text-kira-sm">
          <AdeChip :label="card.status.name" :color="card.status.color" />
          <AdeChip
            v-if="card.blocked"
            :label="t('space.common.status.blocked.status')"
            tone="red"
            data-testid="plan-blocked"
          />
          <span v-if="card.progress.label" class="text-muted-foreground" data-testid="plan-step">{{
            card.progress.label
          }}</span>
          <AdeChip v-for="repo in card.repos" :key="repo" :label="repo" tone="grey" />
        </span>
        <span v-if="card.progress.showBar" class="flex items-center gap-2">
          <Progress :model-value="card.progress.percent" />
          <span class="text-kira-sm text-fg">{{ formatPercent(card.progress.percent) }}</span>
        </span>
        <span v-if="card.attention" class="text-kira-sm text-warn" data-testid="plan-attention">{{
          card.attention
        }}</span>
      </span>
    </button>

    <ol
      v-if="open"
      class="m-0 flex list-none flex-col gap-2 px-3 pb-3 pl-8"
      data-testid="plan-steps"
    >
      <li v-if="!card.blocks.length" class="text-kira-sm text-subtle">{{
        t('mobile.plan.noWorkflow.empty')
      }}</li>
      <li
        v-for="block in card.blocks"
        :key="block.step.id"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="block.state"
      >
        <StatusDot :color="`var(--color-tone-${block.tone}-solid)`" />
        <span
          class="min-w-0 flex-1 truncate"
          :class="block.state === 'now' ? 'font-medium text-fg' : 'text-muted-foreground'"
          >{{
            block.step.name
          }}</span
        >
        <span class="text-subtle">{{ block.statusText }}</span>
      </li>
    </ol>

    <div
      v-if="open && canMove"
      class="flex flex-wrap gap-2 px-3 pb-3 pl-8"
      data-testid="plan-actions"
    >
      <Button
        size="touch"
        variant="dialog"
        :disabled="stopFirst || !card.moves.prev"
        data-testid="plan-step-back"
        @click="actions.ask('back')"
      >
        {{ t('common.action.back.label') }}
      </Button>
      <Button
        size="touch"
        variant="dialog"
        :disabled="stopFirst || !card.moves.next"
        data-testid="plan-step-next"
        @click="actions.ask('next')"
      >
        {{ t('common.action.next.label') }}
      </Button>
      <Button
        size="touch"
        v-if="actions.startKind.value"
        variant="dialog-primary"
        data-testid="plan-start"
        @click="actions.ask('start')"
      >
        {{ t('common.action.start.label') }}
      </Button>
      <FieldDescription v-if="stopFirst" class="w-full">{{
        t('mobile.plan.stopFirst.hint')
      }}</FieldDescription>
    </div>

    <div
      v-if="open && permissions.agentInput && card.tuiSessions.length"
      class="flex flex-col gap-1 px-3 pb-3 pl-8"
    >
      <Button
        size="touch"
        v-for="session in card.tuiSessions"
        :key="session.id"
        as-child
        variant="dialog"
        class="no-underline"
      >
        <RouterLink
          :to="{ name: 'terminal', params: { sessionId: session.id } }"
          class="inline-flex items-center gap-2"
          data-testid="plan-terminal"
          :data-activity="activityKind(session)"
        >
          <AdeActivityIcon :kind="activityKind(session)" bare />
          {{ t('common.module.terminal.label') }}</RouterLink
        >
      </Button>
    </div>
    <ConfirmDialogView
      touch
      tone="primary"
      :open="actions.intent.value !== null"
      :title="actions.dialog.value.title"
      :message="actions.dialog.value.text"
      :confirm-label="actions.dialog.value.confirmLabel"
      :pending="actions.busy.value"
      :error="actions.error.value"
      :testids="{
        root: 'confirm-dialog',
        cancel: 'confirm-cancel',
        confirm: 'confirm-yes',
        error: 'confirm-error',
      }"
      @cancel="actions.cancel"
      @confirm="actions.confirm"
    >
      <Textarea
        v-if="actions.intent.value === 'back' || actions.intent.value === 'next'"
        v-model="actions.notes.value"
        :placeholder="t('mobile.plan.notes.label')"
        :aria-label="t('mobile.plan.notes.label')"
        data-testid="plan-step-notes"
      />
    </ConfirmDialogView>
  </li>
</template>
