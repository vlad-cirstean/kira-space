<script setup lang="ts">
import { type KibanaLanguage, kibanaLanguageSchema } from '@shared/domain/kibana';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { InputGroup } from '@theme/components/ui/input-group';
import { t } from '@theme/i18n';
import { computed, shallowRef } from 'vue';
import type {
  EditorAsyncCompletionSource,
  EditorCompletionSource,
} from '../../../editor/completion';
import MonacoHost from '../../../editor/MonacoHost.vue';
import { KQL_TRIGGER_CHARACTERS } from './kqlCompletion';
import { kqlMonacoLanguage } from './kqlLanguage';
import { luceneMonacoLanguage } from './luceneLanguage';

const props = defineProps<{
  completionSources: readonly EditorCompletionSource[];
  asyncCompletionSources: readonly EditorAsyncCompletionSource[];
  running: boolean;
  canRun: boolean;
  /** Reconnecting before the run. */
  starting: boolean;
}>();
const emit = defineEmits<{ run: []; stop: []; save: [] }>();
const doc = defineModel<string>('doc', { required: true });
const language = defineModel<KibanaLanguage>('language', { required: true });

const LANGUAGES = [
  {
    value: 'kuery',
    label: t('studio.kibana.language.kuery.option'),
    testid: 'kibana-language-kuery',
  },
  {
    value: 'lucene',
    label: t('studio.kibana.language.lucene.option'),
    testid: 'kibana-language-lucene',
  },
];

const languageModel = computed<string>({
  get: () => language.value,
  set: (value) => {
    const parsed = kibanaLanguageSchema.safeParse(value);
    if (parsed.success) language.value = parsed.data;
  },
});

const grammar = computed(() =>
  language.value === 'kuery' ? kqlMonacoLanguage : luceneMonacoLanguage,
);
const triggers = computed(() => (language.value === 'kuery' ? [...KQL_TRIGGER_CHARACTERS] : [':']));

const editorHost = shallowRef<{ insertText: (text: string) => void } | null>(null);

function onSubmit(): void {
  if (props.canRun && !props.running) emit('run');
}

defineExpose({ insertText: (text: string) => editorHost.value?.insertText(text) });
</script>

<template>
  <div class="flex items-center gap-1.5 px-2 py-1" data-testid="kibana-query-bar">
    <SecondaryTabs
      v-model="languageModel"
      variant="segmented"
      :items="LANGUAGES"
      :aria-label="t('studio.kibana.language.select.aria')"
      data-testid="kibana-language"
    />
    <InputGroup class="min-w-0 flex-1" data-testid="kibana-query">
      <div class="kibana-query-host min-w-0 flex-1">
        <MonacoHost
          ref="editorHost"
          v-model:doc="doc"
          language="plain"
          single-line
          save-command
          autocomplete
          auto-close-brackets
          :monaco-language="grammar"
          :trigger-characters="triggers"
          :completion-sources="completionSources"
          :async-completion-sources="asyncCompletionSources"
          @submit="onSubmit"
          @save="emit('save')"
        />
      </div>
    </InputGroup>
    <slot />
    <Button
      v-if="running"
      variant="toolbar"
      size="kira"
      class="text-error"
      data-testid="kibana-stop"
      @click="emit('stop')"
    >
      <CodiconIcon name="debug-stop" :size="13" />
      {{ t('common.action.stop.label') }}
    </Button>
    <Button
      v-else
      variant="dialog-primary"
      size="kira"
      data-testid="kibana-run"
      :disabled="!canRun"
      :pending="starting"
      @click="emit('run')"
    >
      <CodiconIcon name="play" :size="13" />
      {{ t('common.action.run.label') }}
    </Button>
  </div>
</template>

<style scoped>
@reference "@theme/base.css";

/* A single-line host sizes to the editor's own height, so give Monaco one line. */
.kibana-query-host :deep(.monaco-editor) {
  @apply h-control!;
}

.kibana-query-host :deep(.monaco-scrollable-element) {
  @apply overflow-hidden;
}

.kibana-query-host :deep(.scrollbar),
.kibana-query-host :deep(.decorationsOverviewRuler) {
  @apply hidden;
}
</style>
