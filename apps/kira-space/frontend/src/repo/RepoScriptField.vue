<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Input } from '@theme/components/ui/input';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import RunScriptRefPicker from './RunScriptRefPicker.vue';
import type { RepoScriptRefValue } from './state/repoFormTypes';

// A repo script leaf: inline command text or a Run script reference. The parent keeps at most one
// of the pair in the model. Switching the tab alone writes nothing.
const props = defineProps<{
  id: string;
  testid: string;
  place: 'worktree' | 'repo';
  rows?: number;
  placeholder?: string;
  ariaLabel?: string;
}>();
const text = defineModel<string>({ default: '' });
const runRef = defineModel<RepoScriptRefValue | undefined>('runRef');

const mode = ref<'command' | 'run'>(runRef.value ? 'run' : 'command');
// "None" in the picker clears the ref but keeps the tab; a file edit that drops it leaves the tab.
let keepRun = false;
watch(
  () => runRef.value !== undefined,
  (present) => {
    if (!present && keepRun) {
      keepRun = false;
      return;
    }
    mode.value = present ? 'run' : 'command';
  },
);
function onPicked(v: RepoScriptRefValue | undefined): void {
  keepRun = v === undefined;
  runRef.value = v;
}

const items = computed(() => [
  {
    value: 'command',
    label: t('common.field.command.label'),
    testid: `${props.testid}-mode-command`,
  },
  {
    value: 'run',
    label: t('common.script.label'),
    testid: `${props.testid}-mode-run`,
  },
]);
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col gap-2">
    <SecondaryTabs
      :model-value="mode"
      :items="items"
      :aria-label="t('space.repo.config.scriptRef.mode.aria')"
      :data-testid="`${testid}-mode`"
      @update:model-value="(v) => (mode = v === 'run' ? 'run' : 'command')"
    />
    <template v-if="mode === 'command'">
      <Textarea
        v-if="props.rows"
        :id="id"
        :model-value="text"
        :rows="props.rows"
        class="field-sizing-fixed min-h-32 resize-y font-data leading-normal"
        :data-testid="testid"
        @update:model-value="(v) => (text = String(v))"
      />
      <Input
        v-else
        :id="id"
        :model-value="text"
        :placeholder="placeholder"
        :aria-label="ariaLabel"
        class="min-w-0 flex-1 font-data"
        :data-testid="testid"
        @update:model-value="(v) => (text = String(v))"
      />
    </template>
    <RunScriptRefPicker
      v-else
      :id="`${id}-run`"
      :model-value="runRef"
      :testid="testid"
      :place="place"
      @update:model-value="onPicked"
    />
  </div>
</template>
