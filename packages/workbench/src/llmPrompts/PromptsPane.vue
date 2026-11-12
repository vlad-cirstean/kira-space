<script setup lang="ts">
import type { LlmPromptList, LlmPromptView } from '@shared/domain/llmPrompts';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { type MessageKey, t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { computed, ref } from 'vue';
import QueryState from '../components/QueryState.vue';
import { useLlmPromptDraftsStore } from '../state/llmPromptDrafts';
import PromptDetail from './PromptDetail.vue';
import PromptTitle from './PromptTitle.vue';
import { useLlmPrompts } from './queries';

// Settings > Prompts: every LLM prompt this app sends, grouped by area. Edits stay drafts until the
// dialog's Save (SettingsShell `extra`).
defineProps<{ active: boolean }>();

interface Area {
  id: string;
  prefix: string;
  label: MessageKey;
}

const AREAS: readonly Area[] = [
  { id: 'memory', prefix: 'memory.', label: 'common.module.memory.label' },
  { id: 'agentRuns', prefix: 'headless.', label: 'llmPrompts.area.agentRuns.label' },
  { id: 'rebaseReview', prefix: 'ade.', label: 'llmPrompts.area.rebaseReview.label' },
  { id: 'databaseMcp', prefix: 'dbmcp.', label: 'studio.settings.nav.studioMcp.label' },
];

const MIN_LIST_WIDTH = 180;
const MAX_LIST_WIDTH = 480;

const list = useLlmPrompts();
const drafts = useLlmPromptDraftsStore();
const prompts = computed<readonly LlmPromptView[]>(() => list.data.value?.prompts ?? []);

const entries = computed(() => prompts.value.filter((p) => p.kind !== 'argument'));
const argsOf = (parent: string): LlmPromptView[] =>
  prompts.value.filter((p) => p.kind === 'argument' && p.parent === parent);

const groups = computed(() =>
  AREAS.map((area) => ({
    area,
    rows: entries.value.filter((p) => p.id.startsWith(area.prefix)),
  })).filter((g) => g.rows.length > 0),
);

const selectedId = ref<string | null>(null);
const selected = computed(
  () => entries.value.find((p) => p.id === selectedId.value) ?? entries.value[0] ?? null,
);

const isEdited = (p: LlmPromptView): boolean => {
  const draft = drafts.draftFor(p.id);
  return draft === undefined ? p.overridden : draft !== p.defaultText;
};
const rowEdited = (p: LlmPromptView): boolean => isEdited(p) || argsOf(p.id).some(isEdited);

const listWidth = ref(260);
const liveWidth = ref<number | null>(null);
function commitWidth(size: number): void {
  listWidth.value = size;
  liveWidth.value = null;
}
</script>

<template>
  <div v-show="active" class="contents">
    <QueryState
      :query="list"
      variant="inline"
      :is-empty="(data: LlmPromptList) => data.prompts.length === 0"
    >
      <template #empty>
        <InlineState kind="empty">{{ t('llmPrompts.list.empty') }}</InlineState>
      </template>
      <template #default="{ data }">
        <Alert v-if="data.storeError" variant="error" data-testid="llm-prompts-store-error">
          <AlertDescription>{{ data.storeError }}</AlertDescription>
        </Alert>
        <div class="flex min-h-0 flex-1" data-testid="llm-prompts-pane">
          <nav
            class="flex shrink-0 flex-col gap-2 overflow-auto pr-1"
            :style="{ flexBasis: `${liveWidth ?? listWidth}px` }"
          >
            <section v-for="g in groups" :key="g.area.id" class="flex flex-col gap-px">
              <SectionHeading :label="t(g.area.label)" />
              <button
                v-for="p in g.rows"
                :key="p.id"
                type="button"
                class="flex items-center justify-between gap-2 border-none text-left"
                :class="rowVariants({ layout: 'nav', selected: selected?.id === p.id })"
                :aria-current="selected?.id === p.id ? 'true' : undefined"
                data-testid="llm-prompt-row"
                :data-id="p.id"
                @click="selectedId = p.id"
              >
                <span class="min-w-0 truncate"><PromptTitle :view="p" /></span>
                <span class="flex shrink-0 gap-1">
                  <Badge v-if="rowEdited(p)" variant="info" data-testid="llm-prompt-badge">{{
                    t('llmPrompts.badge.edited.label')
                  }}</Badge>
                  <Badge v-if="p.defaultChanged" variant="warn" data-testid="llm-prompt-badge">{{
                    t('llmPrompts.badge.defaultChanged.label')
                  }}</Badge>
                  <Badge v-if="p.invalid !== ''" variant="error" data-testid="llm-prompt-badge">{{
                    t('llmPrompts.badge.invalid.label')
                  }}</Badge>
                </span>
              </button>
            </section>
          </nav>
          <ResizeHandle
            :label="t('llmPrompts.resize.label')"
            :size="listWidth"
            :min="MIN_LIST_WIDTH"
            :max="MAX_LIST_WIDTH"
            line
            @live="liveWidth = $event"
            @commit="commitWidth"
            @cancel="liveWidth = null"
          />
          <main class="min-w-0 flex-1 overflow-auto pl-3">
            <PromptDetail
              v-if="selected"
              :key="selected.id"
              :view="selected"
              :args="argsOf(selected.id)"
            />
          </main>
        </div>
      </template>
    </QueryState>
  </div>
</template>
