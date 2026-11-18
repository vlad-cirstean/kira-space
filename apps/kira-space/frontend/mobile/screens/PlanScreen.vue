<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import QueryState from '@workbench/components/QueryState.vue';
import { ref } from 'vue';
import { useAgentsModel } from '../state/useAgentsModel';
import PlanTaskCard from './PlanTaskCard.vue';

// The plan by day, in plan order. Tap a task for its workflow steps.
const { model, boardQuery } = useAgentsModel();
const open = ref<string | null>(null);
</script>

<template>
  <section class="flex flex-col" data-testid="plan-screen">
    <QueryState :query="boardQuery" :is-empty="() => false">
      <template #error>
        <Alert variant="error" strip data-testid="plan-error">{{
          t('mobile.plan.loadError.error')
        }}</Alert>
      </template>
      <EmptyState
        v-if="model && !model.plan.length"
        fill
        icon="tasklist"
        data-testid="plan-empty"
        :title="t('mobile.plan.list.empty')"
        :description="t('mobile.plan.list.emptyHint')"
      />
      <template v-else-if="model">
        <div v-for="group in model.plan" :key="group.key" data-testid="plan-group">
          <SectionHeading :label="group.label" />
          <ul class="m-0 flex list-none flex-col p-0">
            <PlanTaskCard
              v-for="card in group.cards"
              :key="card.task.id"
              :card="card"
              :open="open === card.task.id"
              @toggle="open = open === card.task.id ? null : card.task.id"
            />
          </ul>
        </div>
      </template>
    </QueryState>
  </section>
</template>
