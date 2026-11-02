<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { useOpenHostView } from '../../../hostview/queries';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import AdeTip from '../AdeTip.vue';
import { tagLabel } from '../board/actions';
import { integrationChips } from '../board/labels';
import { statusWhy, taskPatch } from '../board/panelFacts';
import { useHostLabels } from '../hostLabels';
import { type CardModel, usePlanModel } from '../plan/usePlanModel';
import { useAddTaskRepo, useUpdateTask } from '../queries';
import { useAdeAddUiStore } from '../state/adeAddUi';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeBranchRequestStore } from '../state/adeBranchRequest';
import type { TaskPatch } from '../wire';
import AdeEstimateField from './AdeEstimateField.vue';
import AdeLinkRow from './AdeLinkRow.vue';
import AdeStatusPicker from './AdeStatusPicker.vue';
import AdeTaskBranchMenu from './AdeTaskBranchMenu.vue';
import AdeTaskIssueLine from './AdeTaskIssueLine.vue';
import AdeWorkflowBlock from './AdeWorkflowBlock.vue';
import { useLinkFields } from './useLinkFields';

// Task tab: the editable fields, then the branches.
const props = defineProps<{ card: CardModel }>();
const { liveNow } = usePlanModel();

const ui = useAdeBoardUiStore();
const addUi = useAdeAddUiStore();
const requestStore = useAdeBranchRequestStore();
const update = useUpdateTask();
const addRepo = useAddTaskRepo();

const task = computed(() => props.card.task);
const review = computed(() => props.card.review);
const fieldError = ref('');
const estError = ref<string | null>(null);

async function write(patch: Partial<TaskPatch>): Promise<boolean> {
  fieldError.value = '';
  try {
    await update.mutateAsync({ taskId: task.value.id, patch: taskPatch(patch) });
    return true;
  } catch (err) {
    fieldError.value = errorText(err);
    return false;
  }
}

// ---- handover
const currentStepId = computed(() =>
  props.card.progress.finished ? 'done' : (props.card.progress.step?.id ?? ''),
);
/** The current stage's open draft, else what the last move left for it. */
const handoverSnippet = computed(() => {
  const draft = task.value.handoverDrafts.find((d) => d.fromStepId === currentStepId.value);
  if (draft?.body) return draft.body;
  const h = task.value.handoverIn;
  return h && h.toStepId === currentStepId.value ? h.body : '';
});

// ---- status
const why = computed(() => statusWhy(task.value, props.card.blocked, props.card.progress));

// ---- links
const jira = computed(() => task.value.jira);
const links = useLinkFields(
  () => task.value.hostLink,
  async (patch) => ((await write(patch)) ? null : fieldError.value),
);
const hostLink = computed(() => task.value.hostLink);
const hostLabels = useHostLabels();
const openView = useOpenHostView();

// ---- estimate
async function saveEst(value: string): Promise<void> {
  estError.value = null;
  try {
    await update.mutateAsync({ taskId: task.value.id, patch: taskPatch({ est: value }) });
  } catch (err) {
    estError.value = errorText(err);
  }
}
const spanDays = computed(() => props.card.entry.span);

// ---- branches
const usedRepoIds = computed(() => props.card.rows.map((r) => r.branch.codeRepoId));
const addProjects = useProjectOptions(usedRepoIds);
const addRepoValue = ref<string | null>(null);
async function onAddRepo(id: string): Promise<void> {
  addRepoValue.value = null;
  fieldError.value = '';
  try {
    await addRepo.mutateAsync({ taskId: task.value.id, codeRepoId: id, base: null });
  } catch (err) {
    fieldError.value = errorText(err);
  }
}

function branchLabel(row: CardModel['rows'][number]): string {
  return row.draft ? row.context : row.name;
}

const chipClass = (t: 'muted' | 'stale' | 'unknown'): string =>
  t === 'stale' ? 'text-tone-amber' : 'text-muted-foreground';
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-3 pb-3 pt-3 text-kira-md"
    data-testid="ade-task-tab"
  >
    <KeyValueList class="items-center">
      <dt>{{ t('common.field.status.label') }}</dt>
      <dd class="flex min-h-row items-center gap-2">
        <AdeStatusPicker :card="card" />
        <AdeChip
          v-if="card.blocked"
          :label="t('space.common.status.blocked.status')"
          tone="red"
          data-testid="ade-task-blocked"
        />
        <span class="text-kira-sm text-subtle" data-testid="ade-task-status-why">{{ why }}</span>
      </dd>
      <AdeLinkRow
        id="ade-task-jira"
        :label="t('space.ade.taskTab.issue.label')"
        chip=""
        :text="jira?.key ?? ''"
        :url="jira?.url ?? ''"
        :placeholder="t('space.ade.taskTab.jiraPlaceholder.placeholder')"
        :error="links.jiraError.value"
        @save="links.saveJira"
        @clear="links.clearJira"
      />
      <template v-if="jira">
        <dt />
        <dd class="min-w-0"
          ><AdeTaskIssueLine :task-id="task.id" :link="`${jira.key} ${jira.url}`" /></dd
        >
      </template>
      <AdeLinkRow
        id="ade-task-host"
        :label="t('space.ade.taskTab.pr.label')"
        :chip="hostLabels.chipOf(hostLink)"
        :text="hostLink?.ref ?? ''"
        :url="hostLink ? task.hostUrl : ''"
        :placeholder="
          t('space.ade.taskTab.linkPlaceholder.placeholder', {
            labels: hostLabels.labels.value,
            shorts: hostLabels.shorts.value,
          })
        "
        :error="links.hostError.value"
        :viewable="hostLink?.kind === 'changeRequest' && !!task.hostUrl"
        @view="fireAndForget(openView.mutateAsync(task.hostUrl), 'hostView.open')"
        @save="links.saveHost"
        @clear="links.clearHost"
      />
      <dt>{{ t('space.ade.estimate.field.label') }}</dt>
      <dd
        ><AdeEstimateField :est="task.est" :days="spanDays" :error="estError" @save="saveEst" /></dd
      >
      <template v-if="!review && handoverSnippet">
        <dt>{{ t('space.ade.taskTab.handover.label') }}</dt>
        <dd>
          <button
            type="button"
            class="line-clamp-2 w-full text-left text-kira-md hover:underline"
            data-testid="ade-task-handover-row"
            @click="ui.taskTab = 'notes'"
            >{{
              handoverSnippet
            }}</button
          >
        </dd>
      </template>
      <template v-if="task.branchRequestId">
        <dt>{{ t('space.ade.taskTab.branchRequest.label') }}</dt>
        <dd class="flex min-h-row items-center gap-2" data-testid="ade-task-branch-request-row">
          <span class="text-kira-md">{{ t('space.ade.taskTab.branchRequest.status') }}</span>
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="ade-task-branch-request-open"
            @click="requestStore.open({ id: task.branchRequestId })"
            >{{
              t('space.ade.board.branchTag.review.label')
            }}</Button
          >
        </dd>
      </template>
      <template v-if="!review">
        <dt />
        <dd class="flex min-h-row items-center gap-2">
          <Switch
            id="ade-task-parked"
            :model-value="card.parked"
            data-testid="ade-task-parked"
            @update:model-value="(v: boolean) => write({ kind: v ? 'parked' : 'task' })"
          />
          <Label for="ade-task-parked" class="text-kira-md">{{
            t('space.ade.board.notMerging.label')
          }}</Label>
          <span class="text-kira-sm text-subtle">{{ t('space.ade.taskTab.notMerging.hint') }}</span>
        </dd>
      </template>
    </KeyValueList>
    <Alert v-if="fieldError" variant="error" data-testid="ade-task-error"
      ><AlertDescription>{{ fieldError }}</AlertDescription></Alert
    >

    <AdeWorkflowBlock v-if="!review && !card.parked" :card="card" />

    <div class="flex flex-col gap-0.5">
      <div class="flex items-center gap-2 pb-0.5">
        <span class="text-kira-sm text-subtle">{{ t('common.module.branches.label') }}</span>
        <span class="flex-1" />
        <template v-if="!review">
          <ProjectPicker
            v-if="addProjects.options.value.length > 0"
            v-model="addRepoValue"
            size="kira"
            testid="ade-add-repo"
            :projects="addProjects.options.value"
            :placeholder="t('space.ade.taskTab.addRepo.option')"
            :aria-label="t('space.ade.taskTab.addRepo.aria')"
            :title="t('space.ade.taskTab.addRepo.tooltip')"
            class="max-w-50"
            @select="(o) => onAddRepo(o.value)"
          />
        </template>
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-add-branch"
          @click="addUi.openAttach(task.id, card.title)"
        >
          {{ t('space.ade.taskTab.addBranch.label') }}
        </Button>
      </div>
      <div v-for="row in card.rows" :key="row.id" class="flex items-center gap-1 bg-elevated">
        <Button
          size="kira"
          variant="toolbar"
          class="min-h-row min-w-0 flex-1 justify-start gap-2 bg-elevated px-1.5 text-left text-fg"
          data-testid="ade-task-branch"
          :data-branch-id="row.id"
          @click="ui.selectBranch(task.id, row.id)"
        >
          <AdeChip :label="tagLabel(row.tag, liveNow.getTime())" :tone="row.tag.tone" wide />
          <AdeRepoTag :code-repo-id="row.branch.codeRepoId" :label="row.repo" />
          <AdeTip v-for="c in integrationChips(row.branch)" :key="c.label" :text="c.tip">
            <span class="shrink-0 text-kira-sm font-medium" :class="chipClass(c.tone)">{{
              c.label
            }}</span>
          </AdeTip>
          <span
            class="min-w-0 flex-1 truncate font-data text-kira-md"
            :class="row.draft ? 'text-muted-foreground' : ''"
            >{{
              branchLabel(row)
            }}</span
          >
        </Button>
        <AdeTaskBranchMenu
          v-if="!review"
          :row="row"
          :used-repo-ids="usedRepoIds"
          @error="(m: string) => (fieldError = m)"
        />
      </div>
    </div>
  </div>
</template>
