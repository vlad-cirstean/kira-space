<script setup lang="ts" generic="S extends string, V extends object">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import type { Ref } from 'vue';
import { computed, watch } from 'vue';
import QueryState from '../components/QueryState.vue';
import ViewToolbar from '../components/ViewToolbar.vue';
import CodeTextEditor from '../editor/CodeTextEditor.vue';
import { shortcutKeydown } from '../shortcuts/useShortcut';
import ConfigFileProblem from './ConfigFileProblem.vue';
import ConfigFormProblemAlert from './ConfigFormProblemAlert.vue';
import SaveConflictAlert from './SaveConflictAlert.vue';
import type { useConfigFormDraft } from './useConfigFormDraft';

// The one shell of a config file dialog: header, section tabs with error badges, the form or YAML
// body over one `useConfigFormDraft`, the single save error and the Discard and Save footer. The host
// keeps its query, field table, per-kind form (`form` slot) and title. The file's own problem, a
// form problem no section claims and a failed save each show once.
type Problem = { line: number; message: string };
interface QueryLike {
  data: Ref<unknown>;
  isPending: Ref<boolean>;
  isError: Ref<boolean>;
  error: Ref<unknown>;
  refetch: () => unknown;
}

const props = withDefaults(
  defineProps<{
    draft: ReturnType<typeof useConfigFormDraft<V>>;
    /** The file query: loading and load failure (with Retry) render through it. */
    query: QueryLike;
    title: string;
    /** `api-variables`, `connection-file` or `run-file`: derives every shell testid. */
    testidPrefix: string;
    /** Tabs in order; the last is `yaml`. */
    sections: { value: S; label: string; testid: string }[];
    tabsAria: string;
    yamlAria: string;
    /** JSON pointers each section claims: tab badges and the unclaimed form problem. */
    pointers: Partial<Record<S, readonly string[]>>;
    /** Client-side error count of a section, added to the pointer count. */
    extraErrors?: (section: S) => number;
    lastGood?: boolean;
    lastGoodHint: string;
    /** The file's own problem (it cannot be read as a config). */
    problem?: Problem | null;
    /** False: read-only hint, no tabs or form. */
    editable?: boolean;
    /** A failure outside the file write (connection Duplicate, Delete). */
    actionError?: string | null;
  }>(),
  {
    extraErrors: undefined,
    lastGood: false,
    problem: null,
    editable: true,
    actionError: null,
  },
);
const emit = defineEmits<{ close: [] }>();
const section = defineModel<S>('section', { required: true });

defineSlots<{
  description(): unknown;
  form(props: { section: S }): unknown;
  'yaml-after'(): unknown;
  'footer-start'(): unknown;
}>();

watch(section, (s) => {
  void props.draft.showTab(s === 'yaml' ? 'yaml' : 'form');
});

function errorCount(s: S): number {
  if (s === 'yaml') return 0;
  const server = (props.pointers[s] ?? []).some((p) => props.draft.errorFor(p)) ? 1 : 0;
  return server + (props.extraErrors?.(s) ?? 0);
}
const unclaimed = computed(() =>
  props.draft.unmatched(
    Object.values<readonly string[] | undefined>(props.pointers).flat() as string[],
  ),
);
const saveError = computed(() => props.draft.saveError.value || props.actionError || '');

function showYaml(): void {
  section.value = 'yaml' as S;
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void props.draft.saveNow());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent
      size="2xl"
      fixed-height
      :dirty="draft.dirty.value"
      :data-testid="`${testidPrefix}-dialog`"
      @keydown="onSubmitKey"
    >
      <DialogHeader :close-testid="`${testidPrefix}-close`">
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="$slots.description"><slot name="description" /></DialogDescription>
      </DialogHeader>
      <ViewToolbar v-if="query.data.value && editable" class="px-3">
        <SecondaryTabs
          :model-value="section"
          :items="sections"
          :aria-label="tabsAria"
          @update:model-value="(v) => (section = v as S)"
        >
          <template #item="{ item }">
            {{ item.label }}
            <Badge
              v-if="errorCount(item.value as S) > 0"
              variant="error"
              :data-testid="`${item.testid}-errors`"
              >{{
                errorCount(item.value as S)
              }}</Badge
            >
          </template>
        </SecondaryTabs>
      </ViewToolbar>

      <DialogBody>
        <QueryState :query="query" variant="inline" :error-testid="`${testidPrefix}-error`">
          <div class="flex min-w-0 flex-col gap-3">
            <div
              v-if="lastGood"
              class="text-kira-sm text-subtle"
              :data-testid="`${testidPrefix}-last-good`"
            >
              {{ lastGoodHint }}
            </div>
            <template v-if="!editable">
              <ConfigFileProblem :error="problem" />
              <div class="text-kira-sm text-subtle" :data-testid="`${testidPrefix}-read-only`">
                {{ t('common.content.readOnly.hint') }}
              </div>
            </template>
            <template v-else-if="section !== 'yaml'">
              <ConfigFileProblem :error="problem" :shown="unclaimed" :show-yaml="showYaml" />
              <ConfigFormProblemAlert
                v-if="unclaimed"
                :problem="unclaimed"
                :testid="`${testidPrefix}-problem`"
                show-yaml
                @show-yaml="showYaml"
              />
              <fieldset
                v-if="draft.model.value"
                :disabled="draft.formDisabled.value"
                class="m-0 flex min-w-0 flex-col gap-3 border-0 p-0"
              >
                <slot name="form" :section="section" />
              </fieldset>
            </template>
            <template v-else>
              <ConfigFileProblem :error="problem" />
              <CodeTextEditor
                :model-value="draft.draft.value"
                language="yaml"
                :problem="draft.problem.value"
                :ariaLabel="yamlAria"
                class="h-72"
                :data-testid="`${testidPrefix}-text`"
                @update:model-value="draft.onYamlInput"
                @save="draft.saveNow"
                @submit="draft.saveNow"
              />
              <slot name="yaml-after" />
            </template>
            <SaveConflictAlert
              v-if="saveError"
              :error="saveError"
              :conflict="draft.conflict.value"
              :testid-prefix="testidPrefix"
              @reload="draft.reload"
              @overwrite="draft.overwrite"
            />
          </div>
        </QueryState>
      </DialogBody>

      <DialogFooter>
        <template v-if="$slots['footer-start']" #start><slot name="footer-start" /></template>
        <!-- conventions-allow N17: second footer action; one emphasised button rightmost (RD.2) -->
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!draft.dirty.value"
          :data-testid="`${testidPrefix}-discard`"
          @click="draft.discard"
        >
          {{ t('common.action.discard.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!draft.canSave.value"
          :pending="draft.saving.value"
          :data-testid="`${testidPrefix}-save`"
          @click="draft.saveNow"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
