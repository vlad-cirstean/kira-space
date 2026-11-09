<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Card } from '@theme/components/ui/card';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { storeToRefs } from 'pinia';
import { computed } from 'vue';
import { formatDateTime as when } from '../behaviour/format';
import Identifier from '../components/Identifier.vue';
import ModuleStart from '../components/ModuleStart.vue';
import QueryState from '../components/QueryState.vue';
import DistillView from './distill/DistillView.vue';
import ImportView from './import/ImportView.vue';
import { eventText, GRADE_TEXT, KIND_LABEL, originText, SCOPE_LABEL } from './labels';
import MemoryContext from './MemoryContext.vue';
import { useMemoryHistory } from './queries';
import { useMemoryUiStore } from './store';

// P201: the main area of the Memory module — the selected memory and its version trail.
const ui = useMemoryUiStore();
const { selectedId } = storeToRefs(ui);
const history = useMemoryHistory(selectedId);

const selected = computed(() =>
  history.data.value?.memories.find((m) => m.id === selectedId.value),
);
const trail = computed(() => [...(history.data.value?.memories ?? [])].reverse());

function eventsFor(id: string) {
  return (history.data.value?.events ?? []).filter((e) => e.memoryId === id);
}
</script>

<template>
  <ImportView v-if="ui.view === 'imports'" />
  <DistillView v-else-if="ui.view === 'distill'" />
  <div v-else class="h-full overflow-y-auto" data-testid="memory-start">
    <ModuleStart
      v-if="!selectedId"
      class="h-full"
      data-testid="memory-detail-empty"
      icon="book"
      :title="t('memory.start.empty')"
      :description="t('memory.start.emptyHint')"
    />
    <QueryState v-else :query="history" :is-empty="() => false">
      <template #error="{ message }">
        <Alert variant="error" strip>
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
      </template>
      <div v-if="selected" class="flex flex-col gap-3 p-4" data-testid="memory-detail">
        <div class="flex flex-col gap-1.5">
          <div class="flex items-center gap-1">
            <Badge
              :variant="
                selected.origin === 'user'
                  ? 'info'
                  : selected.origin === 'external'
                    ? 'warn'
                    : 'default'
              "
              data-testid="memory-detail-origin"
              >{{
                originText(selected.origin, selected.sourceTag)
              }}</Badge
            >
            <Badge v-if="selected.kind === 'rule'" data-testid="memory-detail-kind">{{
              t(KIND_LABEL.rule)
            }}</Badge>
            <Badge data-testid="memory-detail-scope">{{ t(SCOPE_LABEL[selected.scope]) }}</Badge>
            <Badge v-if="selected.historical" variant="warn">{{
              t('memory.common.historical.status')
            }}</Badge>
            <span class="text-kira-sm text-muted-foreground"
              >{{ t('memory.common.version.label', { version: selected.version }) }}
              · {{ when(selected.createdAt) }}</span
            >
          </div>
          <p class="m-0 text-kira-md whitespace-pre-wrap" data-testid="memory-detail-fact">{{
            selected.fact
          }}</p>
          <p class="m-0 text-kira-md text-muted-foreground whitespace-pre-wrap">
            <span class="font-medium">{{ t('common.field.reason.label') }}</span>
            {{ selected.reason }}
          </p>
          <div v-if="selected.keywords.length" class="flex flex-wrap gap-1">
            <Badge v-for="k in selected.keywords" :key="k">{{ k }}</Badge>
          </div>
          <FieldDescription data-testid="memory-detail-grade">{{
            t(GRADE_TEXT[selected.origin])
          }}</FieldDescription>
          <dl class="m-0 grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-1 text-kira-sm">
            <dt class="text-muted-foreground">{{ t('memory.start.id.label') }}</dt>
            <dd class="m-0 min-w-0">
              <Identifier :value="selected.lineageId" copyable data-testid="memory-detail-id" />
            </dd>
            <template v-if="selected.project">
              <dt class="text-muted-foreground">{{ t('memory.common.project.label') }}</dt>
              <dd class="m-0 min-w-0">
                <span class="font-data" data-testid="memory-detail-project"
                  ><TruncatedText :text="selected.project" /></span
                >
              </dd>
            </template>
            <dt class="text-muted-foreground">{{ t('memory.start.session.label') }}</dt>
            <dd class="m-0 min-w-0" data-testid="memory-detail-session">
              <Identifier v-if="selected.sessionId" :value="selected.sessionId" copyable />
              <span v-else class="text-muted-foreground">{{
                t('memory.start.noSession.description')
              }}</span>
            </dd>
          </dl>
        </div>
        <MemoryContext :memory-id="selected.id" @select="selectedId = $event" />
        <div v-if="trail.length > 1" class="flex flex-col gap-1.5" data-testid="memory-trail">
          <SectionHeading :label="t('memory.start.versions.label')" />
          <Card
            v-for="version in trail"
            :key="version.id"
            class="gap-0.5"
            :class="{ 'opacity-50': version.historical }"
            :data-testid="`memory-version-${version.version}`"
          >
            <div class="flex items-center gap-1">
              <span class="text-kira-sm text-muted-foreground"
                >{{ t('memory.common.version.label', { version: version.version }) }}
                · {{ when(version.createdAt) }}</span
              >
              <Badge v-if="version.historical" variant="warn">{{
                t('memory.common.historical.status')
              }}</Badge>
            </div>
            <span class="text-kira-md whitespace-pre-wrap">{{ version.fact }}</span>
            <span
              v-for="event in eventsFor(version.id)"
              :key="event.seq"
              class="text-kira-sm text-muted-foreground"
            >
              {{ eventText(event) }}
              <span v-if="event.sessionId" class="font-data">{{
                t('memory.start.event.session.label', { session: event.sessionId })
              }}</span>
            </span>
          </Card>
        </div>
      </div>
    </QueryState>
  </div>
</template>
