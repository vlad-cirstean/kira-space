<script setup lang="ts">
import Disclosure from '@theme/components/Disclosure.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { formatDateTime } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref, watch } from 'vue';
import { control } from '../../../bridge/control';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import AdeTip from '../AdeTip.vue';
import AdeArtifactList from '../artifacts/AdeArtifactList.vue';
import { useHostLabels } from '../hostLabels';
import AdeHandoverList from '../notes/AdeHandoverList.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useRestoreTask } from '../queries';
import { useArchivedTask } from '../readQueries';
import AdeSessionsTab from '../sessions/AdeSessionsTab.vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import type { ArchivedBranch, ArchivedComment, ArchivedTask } from '../wire';
import AdePanelFrame from './AdePanelFrame.vue';
import { PR_TONE, prStateLabel } from './prState';

// An archived task, read-only, with everything it kept. Restore brings it back to the Plan; edits
// need that first.
const props = defineProps<{ taskId: string }>();

const boardUi = useAdeBoardUiStore();
const { sessions, repoLabel } = usePlanModel();
const hostLabels = useHostLabels();
const query = useArchivedTask(() => props.taskId);
const restore = useRestoreTask();
const restoreError = ref('');
const tab = ref('task');

watch(
  () => props.taskId,
  () => {
    tab.value = 'task';
    restoreError.value = '';
  },
);

const count = computed(() => sessions.value.filter((s) => s.taskId === props.taskId).length);
const tabs = computed(() => [
  { value: 'task', label: t('common.task.label') },
  { value: 'notes', label: t('space.common.notes.label') },
  { value: 'artifacts', label: t('space.ade.artifacts.title.title') },
  { value: 'sessions', label: t('space.common.sessions.label'), count: count.value },
]);

function titleOf(a: ArchivedTask): string {
  const first = a.branches[0];
  return (
    a.task.title ||
    a.task.jira?.key ||
    first?.changeRequest?.title ||
    first?.name ||
    t('space.ade.board.newTask.label')
  );
}
function repos(a: ArchivedTask): string {
  return [...new Set(a.branches.map((b) => repoLabel(b.codeRepoId)))].join(' · ');
}
function status(a: ArchivedTask) {
  return a.statuses.find((s) => s.id === a.task.statusId);
}
function stepText(a: ArchivedTask): string {
  const wf = a.task.workflow;
  if (!wf) return '';
  const step =
    a.task.entryStepId === 'done'
      ? t('common.done.status')
      : (wf.steps.find((x) => x.id === a.task.entryStepId)?.name ?? '');
  return step ? `${wf.name} · ${step}` : wf.name;
}
function kindLabel(b: ArchivedBranch): string {
  if (b.kind === 'review') return t('space.ade.board.branchTag.review.label');
  return b.kind === 'parked' ? t('space.ade.board.notMerging.label') : '';
}

function commentKind(c: ArchivedComment): string {
  return c.kind === 'question'
    ? t('space.ade.needs.kind.question.label')
    : t('git.review.comments.kind.change.label');
}
function commentState(c: ArchivedComment): string {
  return c.state === 'addressed'
    ? t('git.review.comments.state.addressed.label')
    : t('common.action.open.label');
}

function open(url: string, e: MouseEvent): void {
  e.preventDefault();
  fireAndForget(control.openExternal(url), 'ade.archived');
}

async function onRestore(): Promise<void> {
  restoreError.value = '';
  try {
    await restore.mutateAsync({ taskId: props.taskId });
    boardUi.openTask(props.taskId);
  } catch (err) {
    restoreError.value = errorText(err);
  }
}
</script>

<template>
  <QueryState :query="query">
    <template #default="{ data: a }">
      <AdePanelFrame v-model="tab" :tabs="tabs">
        <template #header>
          <div class="flex items-start gap-2">
            <span class="mt-1 box-border size-3 shrink-0 rounded-kira-xs bg-disabled" />
            <AdeChip :label="t('space.ade.panel.archived.label')" tone="grey" />
            <AdeChip
              v-if="status(a)"
              :label="status(a)!.name"
              :color="status(a)!.color"
              data-testid="ade-archived-status"
            />
            <AdeTip :text="titleOf(a)">
              <h3
                class="m-0 line-clamp-2 min-w-0 flex-1 break-words text-kira-lg font-medium text-fg"
                data-testid="ade-panel-title"
              >
                {{ titleOf(a) }}
              </h3>
            </AdeTip>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="min-w-0 flex-1 truncate text-kira-sm text-muted-foreground"
              data-testid="ade-panel-facts"
              >{{
                [repos(a), formatDateTime(a.archivedAt)].filter(Boolean).join(' · ')
              }}</span
            >
            <span class="shrink-0 text-kira-sm text-subtle">{{
              t('space.ade.archived.restore.hint')
            }}</span>
            <Button
              variant="dialog"
              size="kira"
              class="shrink-0"
              :pending="restore.isPending.value"
              data-testid="ade-archived-restore"
              @click="onRestore"
            >
              {{ t('common.action.restore.label') }}
            </Button>
          </div>
          <Alert v-if="restoreError" variant="error" data-testid="ade-archived-restore-error"
            ><AlertDescription>{{ restoreError }}</AlertDescription></Alert
          >
          <Alert
            v-if="boardUi.actionError[taskId]"
            variant="error"
            data-testid="ade-archived-resume-error"
            ><AlertDescription>{{ boardUi.actionError[taskId] }}</AlertDescription></Alert
          >
        </template>

        <div
          v-if="tab === 'task'"
          class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-3 pb-3 pt-3 text-kira-md"
          data-testid="ade-archived-task-tab"
        >
          <KeyValueList class="items-center">
            <template v-if="a.task.jira">
              <dt>{{ t('space.ade.taskTab.issue.label') }}</dt>
              <dd class="min-h-row">
                <a
                  :href="a.task.jira.url"
                  class="text-info"
                  data-testid="ade-archived-jira-link"
                  @click="(e) => open(a.task.jira!.url, e)"
                  >{{
                    a.task.jira.key
                  }}</a
                >
              </dd>
            </template>
            <template v-if="a.task.hostLink">
              <dt>{{ t('space.ade.taskTab.pr.label') }}</dt>
              <dd class="flex min-h-row items-center gap-2">
                <AdeChip :label="hostLabels.chipOf(a.task.hostLink)" tone="grey" />
                <a
                  :href="a.task.hostUrl"
                  class="text-info"
                  data-testid="ade-archived-host-link"
                  @click="(e) => open(a.task.hostUrl, e)"
                  >{{
                    a.task.hostLink.ref
                  }}</a
                >
              </dd>
            </template>
            <template v-if="a.task.est">
              <dt>{{ t('space.ade.estimate.field.label') }}</dt>
              <dd class="min-h-row">{{ a.task.est }}</dd>
            </template>
            <template v-if="a.task.workflow">
              <dt>{{ t('space.common.workflow.label') }}</dt>
              <dd class="min-h-row" data-testid="ade-archived-workflow">{{ stepText(a) }}</dd>
            </template>
            <dt>{{ t('common.created.label') }}</dt>
            <dd class="min-h-row">{{ formatDateTime(a.task.createdAt) }}</dd>
          </KeyValueList>

          <div class="flex flex-col gap-1">
            <span class="pb-0.5 text-kira-sm text-subtle">{{
              t('common.module.branches.label')
            }}</span>
            <div
              v-for="b in a.branches"
              :key="b.id"
              class="flex flex-col gap-1 bg-elevated px-1.5 py-1"
              data-testid="ade-archived-branch"
              :data-branch-id="b.id"
            >
              <div class="flex min-w-0 items-center gap-2">
                <AdeRepoTag :code-repo-id="b.codeRepoId" :label="repoLabel(b.codeRepoId)" />
                <a
                  v-if="b.name && b.branchUrl"
                  :href="b.branchUrl"
                  class="min-w-0 truncate font-data text-kira-md text-info"
                  @click="(e) => open(b.branchUrl, e)"
                  >{{
                    b.name
                  }}</a
                >
                <span
                  v-else
                  class="min-w-0 truncate font-data text-kira-md"
                  :class="b.name ? '' : 'text-muted-foreground'"
                  >{{
                    b.name || t('space.ade.board.branchTag.notCreated.label')
                  }}</span
                >
                <span class="flex-1" />
                <span v-if="kindLabel(b)" class="shrink-0 text-kira-sm text-muted-foreground">{{
                  kindLabel(b)
                }}</span>
                <AdeChip v-if="b.mergedAt" :label="t('common.merged.status')" tone="purple" />
                <AdeChip v-if="b.removed" :label="t('common.removed.status')" tone="grey" />
              </div>
              <div v-if="b.base" class="text-kira-sm text-muted-foreground">{{
                t('space.ade.review.header.base.label', { base: b.base })
              }}</div>
              <div
                v-if="b.changeRequest"
                class="flex min-w-0 items-center gap-2"
                data-testid="ade-archived-pr"
              >
                <AdeChip
                  :label="prStateLabel(b.changeRequest.state)"
                  :tone="PR_TONE[b.changeRequest.state] ?? 'grey'"
                />
                <a
                  :href="b.changeRequest.url"
                  class="shrink-0 font-data text-kira-md text-info"
                  @click="(e) => open(b.changeRequest!.url, e)"
                  >#{{ b.changeRequest.number }}</a
                >
                <span class="min-w-0 flex-1 truncate">{{ b.changeRequest.title }}</span>
                <span
                  v-if="b.prSource === 'archived'"
                  class="shrink-0 text-kira-sm text-subtle"
                  data-testid="ade-archived-snapshot"
                  >{{
                    t('space.ade.archived.snapshot.hint')
                  }}</span
                >
              </div>
              <Disclosure
                v-if="b.comments.length"
                :label="t('space.ade.archived.comments.label', { count: b.comments.length })"
                data-testid="ade-archived-comments"
                content-class="flex flex-col gap-1 pt-1"
              >
                <div
                  v-for="(c, i) in b.comments"
                  :key="i"
                  class="flex flex-col gap-0.5 border-l-2 border-border pl-2"
                  data-testid="ade-archived-comment"
                >
                  <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
                    <span class="min-w-0 truncate font-data"
                      >{{ c.path }}:{{ c.start }}{{ c.end > c.start ? `-${c.end}` : '' }}</span
                    >
                    <span class="shrink-0">{{ commentKind(c) }}</span>
                    <span class="shrink-0">{{ commentState(c) }}</span>
                  </div>
                  <div class="whitespace-pre-wrap break-words">{{ c.body }}</div>
                  <div v-if="c.reply" class="whitespace-pre-wrap break-words text-muted-foreground"
                    >{{ t('space.common.reply.label') }}: {{ c.reply }}</div
                  >
                </div>
              </Disclosure>
            </div>
          </div>
        </div>
        <div
          v-else-if="tab === 'notes'"
          class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3"
          data-testid="ade-archived-notes"
        >
          <AdeNotesEditor :notes="a.task.notes" :item-id="a.task.id" readonly />
          <AdeHandoverList :task-id="a.task.id" readonly />
        </div>
        <div
          v-else-if="tab === 'artifacts'"
          class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3"
          data-testid="ade-archived-artifacts"
        >
          <AdeArtifactList :task-id="a.task.id" readonly />
        </div>
        <AdeSessionsTab v-else :task-id="a.task.id" archived />
      </AdePanelFrame>
    </template>
  </QueryState>
</template>
