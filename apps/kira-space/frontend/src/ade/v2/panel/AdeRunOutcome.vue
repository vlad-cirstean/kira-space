<script setup lang="ts">
import { shortSha } from '@kira/git-core';
import CodeBlock from '@theme/components/CodeBlock.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, ref, watch } from 'vue';
import AdeChip from '../AdeChip.vue';
import type { Tone } from '../board/actions';
import type { RebaseAct } from '../board/rebaseActions';
import { runStateLabel } from '../runState';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { ACTION_CLASS } from '../tones';
import type { Run } from '../wire';
import AdeRunLog from './AdeRunLog.vue';
import AdeRunSandboxBadge from './AdeRunSandboxBadge.vue';
import { outcomeText } from './outcomeText';

// How one run ended: the agent's own report, then what git showed. Used for a branch's last
// rebase and for the report of a failed step run.
const props = defineProps<{
  run: Run;
  title: string;
  /** The rebase acts that can follow it. */
  acts?: readonly RebaseAct[];
  /** Opens the log when a See log click asks for it. */
  listen?: boolean;
}>();

const dialogs = useAdeDialogsStore();
const ui = useAdeBoardUiStore();
const { copy: copyText, copied, error: copyError } = useCopy();
const showLog = ref(false);
watch(
  () => props.listen && ui.focusRebaseLog,
  (want) => {
    if (!want) return;
    showLog.value = true;
    ui.focusRebaseLog = false;
  },
  { immediate: true },
);

const outcome = computed(() => props.run.outcome);
const rebase = computed(() => outcome.value?.rebase ?? null);
const files = computed(() => [
  ...new Set([
    ...(rebase.value?.conflictedFiles ?? []),
    ...(outcome.value?.report?.conflictedFiles ?? []),
  ]),
]);

const STATE_TONE: Record<string, Tone> = {
  done: 'green',
  failed: 'red',
  stuck: 'red',
  back: 'amber',
  running: 'blue',
  pending: 'grey',
  review: 'blue',
};
const state = computed(() => runStateLabel(props.run.state));

// A step's own result, shown next to the state; the implicit done and failed say nothing more.
const result = computed(() => {
  const id = outcome.value?.result ?? '';
  return id && id !== 'done' && id !== 'failed' ? id : '';
});
const resultTone = computed<Tone>(() => (outcome.value?.status === 'done' ? 'green' : 'red'));
const route = computed(() => {
  const r = outcome.value?.route ?? '';
  switch (r) {
    case '':
    case 'next':
      return '';
    case 'end':
      return t('space.ade.outcome.routeEnd.label');
    case 'stop':
      return t('space.ade.activity.stopped.status');
    case 'retry':
      return `↻ ${props.run.note}`;
    default:
      return r.startsWith('back:') ? `↩ ${props.run.note}` : `→ ${r}`;
  }
});

const endedBy = computed(() => {
  const o = outcome.value;
  if (!o) return '';
  return [
    t(o.reported ? 'space.ade.outcome.endedByReported.label' : 'space.ade.outcome.endedBy.label', {
      source: o.source,
    }),
    ...(rebase.value?.aborted ? [t('common.aborted.status')] : []),
    ...(rebase.value?.inProgress ? [t('space.ade.outcome.inProgress.status')] : []),
  ].join(' · ');
});

function copy(): void {
  void copyText(outcomeText(props.run, props.title, state.value));
}
</script>

<template>
  <div
    class="flex flex-col gap-1.5 rounded-kira bg-elevated p-2"
    data-testid="ade-run-outcome"
    :data-run-id="run.id"
  >
    <div class="flex items-center gap-2">
      <span class="text-kira-sm text-muted-foreground">{{ title }}</span>
      <AdeChip :label="state" :tone="STATE_TONE[run.state] ?? 'grey'" />
      <span v-if="result" data-testid="ade-outcome-result"
        ><AdeChip :label="result" :tone="resultTone" /></span
      >
      <span
        v-if="route"
        class="text-kira-sm text-muted-foreground"
        data-testid="ade-outcome-route"
        >{{
          route
        }}</span
      >
      <AdeChip
        v-if="rebase"
        :label="
          rebase.verified
            ? t('space.ade.outcome.verified.status')
            : t('space.ade.outcome.notVerified.status')
        "
        :tone="rebase.verified ? 'green' : 'amber'"
      />
      <AdeRunSandboxBadge
        v-if="run.sandbox && run.sandbox.denials > 0"
        :run-id="run.id"
        :badge="run.sandbox"
      />
      <span class="flex-1" />
      <Button variant="toolbar" size="kira" data-testid="ade-outcome-copy" @click="copy">
        {{ copied ? t('common.copy.copied.announce') : t('common.copyForAgent.label') }}
      </Button>
    </div>
    <p v-if="outcome?.reason" class="m-0 text-kira-md" data-testid="ade-outcome-reason">{{
      outcome.reason
    }}</p>
    <FieldDescription v-if="outcome" data-testid="ade-outcome-source">{{
      endedBy
    }}</FieldDescription>
    <ul
      v-if="files.length"
      class="m-0 flex list-none flex-col gap-0.5 p-0 font-data text-kira-sm"
      data-testid="ade-outcome-files"
    >
      <li v-for="f in files" :key="f" class="truncate text-tone-red">{{ f }}</li>
    </ul>
    <CodeBlock
      v-if="outcome?.report?.lastGitError"
      class="text-error"
      data-testid="ade-outcome-git-error"
      >{{
        outcome.report.lastGitError
      }}</CodeBlock
    >
    <FieldDescription v-if="outcome?.report?.tried" data-testid="ade-outcome-tried">
      {{ t('space.ade.outcome.tried.label', { tried: outcome.report.tried }) }}
    </FieldDescription>
    <div
      v-for="b in rebase?.branches ?? []"
      :key="b.branchId"
      class="flex items-center gap-1.5 font-data text-kira-sm"
      data-testid="ade-outcome-branch"
    >
      <span class="min-w-0 truncate">{{ b.name }}</span>
      <span class="inline-flex items-center gap-1 text-subtle"
        ><Identifier :value="b.before" :short="shortSha" />
        →
        <Identifier :value="b.after" :short="shortSha" /></span
      >
      <span v-if="!b.onBase" class="text-tone-amber">{{
        t('space.ade.outcome.notOnBase.status')
      }}</span>
    </div>
    <Alert v-if="copyError" variant="error"
      ><AlertDescription>{{ copyError }}</AlertDescription></Alert
    >
    <div v-if="acts?.length" class="flex flex-wrap gap-1.5">
      <TextTooltip v-for="a in acts" :key="a.id" :parts="a.tip">
        <Button
          variant="dialog"
          size="kira"
          :class="ACTION_CLASS[a.kind === 'abortRebase' ? 'red' : 'amber']"
          :disabled="a.disabled"
          :data-testid="`ade-outcome-act-${a.id}`"
          @click="dialogs.act(a)"
        >
          <VarText :parts="a.label" />
        </Button>
      </TextTooltip>
    </div>
    <Button
      variant="toolbar"
      size="kira"
      class="self-start"
      data-testid="ade-outcome-log-toggle"
      @click="showLog = !showLog"
    >
      {{ showLog ? t('space.common.hideLog.label') : t('space.common.showLog.label') }}
    </Button>
    <AdeRunLog v-if="showLog" kind="run" :id="run.id" />
  </div>
</template>
