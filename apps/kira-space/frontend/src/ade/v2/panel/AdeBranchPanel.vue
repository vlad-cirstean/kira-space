<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import BranchChip from '@theme/components/BranchChip.vue';
import InlineState from '@theme/components/InlineState.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { hostWords, type MessageKey, t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import Identifier from '@workbench/components/Identifier.vue';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import { control } from '../../../bridge/control';
import { useOpenHostView } from '../../../hostview/queries';
import { useRepos } from '../../../repo/state/reposQueries';
import AdeChip from '../AdeChip.vue';
import AdeForcePushDialog from '../AdeForcePushDialog.vue';
import AdeTip from '../AdeTip.vue';
import { useRunAutomation } from '../automation/useRunAutomation';
import type { Tone } from '../board/actions';
import { type RebaseAct, rebaseActs } from '../board/rebaseActions';
import { baseNameOf } from '../board/reviewCode';
import { type CardModel, type PlanModel, usePlanModel } from '../plan/usePlanModel';
import { useChangeRequests, useRetrySetup } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import AdeSessionsTab from '../sessions/AdeSessionsTab.vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { ACTION_CLASS, TONE_SOLID_CLASS } from '../tones';
import type { ChangeRequest, Deployment, Integration, Run } from '../wire';
import AdePanelFrame from './AdePanelFrame.vue';
import AdeRunOutcome from './AdeRunOutcome.vue';
import AdeWorktreeSetup from './AdeWorktreeSetup.vue';
import { type HeaderAction, headerActions } from './headerActions';
import { PR_TONE, prStateLabel } from './prState';

// Branch mode: header (back link, git status, name, facts, Force push), Details and Sessions tabs.
const props = defineProps<{ card: CardModel; row: CardModel['rows'][number]; model: PlanModel }>();

const ui = useAdeBoardUiStore();
const tabs = useAdePanelTabsStore();
const tab = computed({
  get: () => tabs.branchTab(props.row.id),
  set: (v) => tabs.setBranchTab(props.row.id, v),
});
const dialogs = useAdeDialogsStore();
const { sessions } = usePlanModel();
const sessionCount = computed(
  () => sessions.value.filter((x) => x.branchId === props.row.id && x.state === 'running').length,
);
const changeRequests = useChangeRequests();
const repos = useRepos();
const retrySetup = useRetrySetup();
const reviewCode = useReviewCode();
const automation = useRunAutomation();
const contextMenu = useContextMenuStore();
const setupError = ref('');
const forcePush = ref(false);
const { copy, copied } = useCopy();
const openView = useOpenHostView();
const copiedKey = ref('');

const branch = computed(() => props.row.branch);
const graph = computed(() => props.model.view.graph);
const baseName = computed(() => baseNameOf(props.row, graph.value));
const reviewChoice = computed(() => reviewCode.choiceOf(props.row));
const reviewOff = computed(
  () => reviewCode.pending.value || !reviewChoice.value || reviewChoice.value.disabled,
);
const facts = computed(() =>
  t('space.ade.branchPanel.facts.label', {
    repo: props.row.repo,
    base: baseName.value,
    ahead: branch.value.ahead,
    behind: branch.value.behind,
  }),
);
const chipLabel = computed(() => {
  const state = branch.value.setup?.state;
  if (state === 'running') return t('space.ade.branchPanel.preparing.status');
  return state === 'failed' ? t('space.ade.needs.kind.setupFailed.label') : props.row.tag.label;
});
const setupFailed = computed(() => branch.value.setup?.state === 'failed');
async function onRetrySetup(): Promise<void> {
  setupError.value = '';
  try {
    await retrySetup.mutateAsync({ branchId: branch.value.id });
  } catch (err) {
    setupError.value = errorText(err);
  }
}
const runs = computed(() => props.card.task.runs);
const mainName = computed(
  () =>
    props.model.board.repos.find((r) => r.codeRepoId === branch.value.codeRepoId)?.mainName ?? '',
);
const actions = computed(() =>
  headerActions({
    branch: branch.value,
    graph: graph.value,
    after: props.model.view.after,
    hadSession: sessions.value.some((x) => x.branchId === branch.value.id),
    runs: runs.value,
    mainName: mainName.value,
  }),
);
const lastRebase = computed(() =>
  runs.value
    .filter((r) => r.purpose === 'rebase' && r.branchId === branch.value.id)
    .reduce<Run | null>((best, r) => (!best || r.attempt >= best.attempt ? r : best), null),
);
// The acts that follow a finished rebase: try again, or abort what it left behind.
const outcomeActs = computed((): RebaseAct[] =>
  rebaseActs({
    branch: branch.value,
    graph: graph.value,
    after: props.model.view.after,
    runs: runs.value,
    mainName: mainName.value,
  }).filter((a) => a.id === 'rebase-retry' || a.id === 'abort-rebase'),
);
function run(a: HeaderAction, ev?: MouseEvent): void {
  const id = branch.value.id;
  if (a.kind === 'rebase') dialogs.act(a.act);
  else if (a.kind === 'remerge') dialogs.merge(id, a.target);
  else if (a.kind === 'start') dialogs.start(id);
  else if (a.kind === 'automation') {
    const r = (ev?.currentTarget as Element | null)?.getBoundingClientRect();
    if (r)
      contextMenu.openContextMenuAt(r.left, r.bottom, automation.menuItems(props.card.task.id, id));
  } else if (a.kind === 'review') {
    setupError.value = '';
    reviewCode.open(props.card.task.id, id, null, { onError: (msg) => (setupError.value = msg) });
  }
}
const actionClass = (a: HeaderAction): string => ACTION_CLASS[a.tone];
const canForcePush = computed(() => props.row.tag.actions.some((a) => a.kind === 'forcePush'));

// ---- Details rows
const changeRequest = computed(
  (): ChangeRequest | null => changeRequests.data.value?.branches[branch.value.id] ?? null,
);
const words = computed(() =>
  hostWords(
    changeRequests.data.value?.repos.find((r) => r.codeRepoId === branch.value.codeRepoId)?.host,
  ),
);
const branchUrl = computed(() => changeRequests.data.value?.branchUrls[branch.value.id] ?? '');
const FORCE_PUSH_CMD = 'git push --force-with-lease';

function copyLink(key: string, text: string): void {
  copiedKey.value = key;
  void copy(text);
}
function open(url: string, e: MouseEvent): void {
  e.preventDefault();
  fireAndForget(control.openExternal(url), 'ade.branch');
}

const repoInfo = computed(() =>
  repos.data.value?.repos.find((r) => r.codeRepoId === branch.value.codeRepoId),
);

// The repo's own integration branches and environments, each filled from the branch when it has an
// entry, else `not merged` / `not deployed`; an entry the repo no longer lists still shows.
const intoRows = computed((): Integration[] => {
  const have = new Map(branch.value.integration.map((i) => [i.target, i]));
  const rows = (repoInfo.value?.integrationBranches ?? []).map(
    (target): Integration =>
      have.get(target) ?? { target, status: 'not merged', note: '', recorded: false },
  );
  const listed = new Set(rows.map((r) => r.target));
  return [...rows, ...branch.value.integration.filter((i) => !listed.has(i.target))];
});
const deployRows = computed((): Deployment[] => {
  const have = new Map(branch.value.deployments.map((d) => [d.env, d]));
  const rows = (repoInfo.value?.environments ?? []).map(
    (e): Deployment =>
      have.get(e.name) ?? {
        env: e.name,
        deployedSha: '',
        checkedAt: 0,
        status: 'not deployed',
        missingCommits: 0,
        note: '',
        error: '',
      },
  );
  const listed = new Set(rows.map((r) => r.env));
  return [...rows, ...branch.value.deployments.filter((d) => !listed.has(d.env))];
});

/** Merging acts on a created branch of mine. */
const canMerge = computed(() => branch.value.kind === 'mine' && branch.value.name !== '');

const INTO_TONE: Record<Integration['status'], Tone> = {
  merged: 'green',
  stale: 'amber',
  'not merged': 'grey',
};
const INTO_STATUS = {
  merged: 'common.merged.status',
  stale: 'common.stale.status',
  'not merged': 'space.ade.branchPanel.state.notMerged.status',
} as const satisfies Record<Integration['status'], MessageKey>;
function intoNote(i: Integration): string {
  if (i.note) return i.note;
  if (i.status === 'merged') return t('space.ade.branchPanel.upToDate.status');
  return i.status === 'stale'
    ? t('space.ade.branchPanel.hasChangesNotIn.status', { target: i.target })
    : '';
}

const DEPLOY_TONE: Record<Deployment['status'], Tone> = {
  deployed: 'green',
  stale: 'amber',
  'not deployed': 'grey',
  unknown: 'grey',
};
const DEPLOY_STATUS = {
  deployed: 'space.ade.branchPanel.state.deployed.status',
  stale: 'common.stale.status',
  'not deployed': 'space.ade.branchPanel.state.notDeployed.status',
  unknown: 'space.ade.branchPanel.state.unknown.status',
} as const satisfies Record<Deployment['status'], MessageKey>;
function deployNote(d: Deployment): string {
  if (d.status === 'deployed')
    return t('space.ade.branchPanel.runs.status', { sha: d.deployedSha });
  if (d.status === 'stale')
    return d.note || t('space.ade.branchPanel.runsStale.status', { sha: d.deployedSha });
  return d.status === 'unknown' ? d.error : '';
}
</script>

<template>
  <AdePanelFrame
    v-model="tab"
    :tabs="[
      { value: 'details', label: t('common.details.title') },
      { value: 'sessions', label: t('space.ade.panel.sessionsTab.label', { count: sessionCount }) },
    ]"
  >
    <template #header>
      <Button
        variant="link"
        size="inline"
        class="max-w-full justify-start self-start truncate"
        data-testid="ade-panel-back"
        @click="ui.select(card.task.id)"
      >
        {{ t('space.ade.branchPanel.back.label', { title: card.title }) }}
      </Button>
      <div class="flex items-start gap-2">
        <span
          class="mt-1 box-border size-3 shrink-0 rounded-kira-xs border-2"
          :class="card.review && 'border-tone-blue-solid'"
          :style="card.review ? undefined : { borderColor: card.color }"
          data-testid="ade-panel-dot"
        />
        <AdeTip :text="row.tag.tip">
          <AdeChip :label="chipLabel" :tone="row.tag.tone" />
        </AdeTip>
        <AdeTip v-if="branch.origin === 'agent'" :text="t('space.ade.agentOrigin.tooltip')">
          <span class="mt-px inline-flex shrink-0 text-subtle" data-testid="ade-panel-agent">
            <CodiconIcon name="sparkle" :size="13" />
          </span>
        </AdeTip>
        <h3
          class="m-0 line-clamp-2 min-w-0 flex-1 break-all font-data text-kira-lg font-medium text-fg"
          data-testid="ade-panel-title"
        >
          {{
            row.draft
              ? t('space.ade.branchPanel.draftTitle.title', { repo: row.repo })
              : branch.name
          }}
        </h3>
      </div>
      <div class="truncate text-kira-sm text-muted-foreground" data-testid="ade-panel-facts">{{
        facts
      }}</div>
      <Alert v-if="setupError" variant="error" data-testid="ade-panel-setup-error"
        ><AlertDescription>{{ setupError }}</AlertDescription></Alert
      >
      <Alert
        v-if="ui.actionError[card.task.id]"
        variant="error"
        data-testid="ade-panel-action-error"
        ><AlertDescription>{{ ui.actionError[card.task.id] }}</AlertDescription></Alert
      >
      <div v-if="canForcePush || setupFailed || actions.length" class="flex flex-wrap gap-1.5 pt-1">
        <Button
          variant="dialog"
          v-if="setupFailed"
          size="kira-lg"
          :class="TONE_SOLID_CLASS.amber"
          data-testid="ade-panel-retry-setup"
          @click="onRetrySetup"
        >
          {{ t('space.ade.setup.retry.label') }}
        </Button>
        <AdeTip v-if="canForcePush" :text="FORCE_PUSH_CMD">
          <Button
            variant="dialog"
            size="kira-lg"
            :class="TONE_SOLID_CLASS.amber"
            data-testid="ade-panel-force-push"
            @click="forcePush = true"
          >
            {{ t('common.forcePush.label') }}
          </Button>
        </AdeTip>
        <template v-for="a in actions" :key="(a.kind === 'rebase' ? a.act.id : a.kind) + a.label">
          <AdeTip v-if="a.kind === 'review'" :parts="reviewChoice?.tip ?? []">
            <TooltipDisabledTrigger :disabled="reviewOff">
              <Button
                variant="dialog"
                size="kira-lg"
                :class="actionClass(a)"
                :disabled="reviewOff"
                :data-testid="`ade-panel-action-${a.kind}`"
                @click="run(a)"
              >
                <CodiconIcon name="git-compare" :size="13" />
                {{ a.label }}
              </Button>
            </TooltipDisabledTrigger>
          </AdeTip>
          <AdeTip v-else-if="a.kind === 'rebase'" :parts="a.act.tip">
            <TooltipDisabledTrigger :disabled="a.act.disabled">
              <Button
                variant="dialog"
                size="kira-lg"
                :class="actionClass(a)"
                :disabled="a.act.disabled"
                :data-testid="`ade-panel-action-${a.act.id}`"
                @click="run(a)"
              >
                {{ a.label }}
              </Button>
            </TooltipDisabledTrigger>
          </AdeTip>
          <Button
            variant="dialog"
            v-else
            size="kira-lg"
            :class="actionClass(a)"
            :disabled="a.kind === 'created'"
            :data-testid="`ade-panel-action-${a.kind}`"
            @click="(ev: MouseEvent) => run(a, ev)"
          >
            {{ a.label }}
          </Button>
        </template>
      </div>
    </template>

    <div
      v-if="tab === 'details'"
      class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-3 pb-3 pt-3 text-kira-md"
      data-testid="ade-branch-details"
    >
      <KeyValueList class="items-center">
        <dt>{{ t('common.field.branch.label') }}</dt>
        <dd class="flex min-h-row min-w-0 items-center gap-2">
          <AdeChip :label="chipLabel" :tone="row.tag.tone" wide />
          <template v-if="branch.name">
            <AdeTip :text="branch.name">
              <a
                v-if="branchUrl"
                :href="branchUrl"
                class="min-w-0 max-w-1/2 rounded-kira-sm no-underline"
                @click="(e) => open(branchUrl, e)"
                ><BranchChip :name="branch.name" class="max-w-full" /></a
              >
              <BranchChip v-else :name="branch.name" class="max-w-1/2" />
            </AdeTip>
            <span class="flex-1" />
            <TooltipIconButton
              :icon="copied && copiedKey === 'branch' ? 'check' : 'copy'"
              :label="t('space.ade.branchPanel.copyBranchLink.tooltip')"
              class="shrink-0"
              @click="copyLink('branch', branchUrl || branch.name)"
            />
          </template>
        </dd>
        <dt>{{ words.changeRequestShort }}</dt>
        <dd class="flex min-h-row min-w-0 items-center gap-2" data-testid="ade-branch-pr">
          <AdeChip
            :label="
              changeRequest ? prStateLabel(changeRequest.state) : t('common.state.none.option')
            "
            :tone="changeRequest ? (PR_TONE[changeRequest.state] ?? 'grey') : 'grey'"
            wide
          />
          <template v-if="changeRequest">
            <a
              :href="changeRequest.url"
              class="max-w-1/2 shrink-0 truncate font-data text-kira-md text-info"
              @click="(e) => open(changeRequest!.url, e)"
              >#{{ changeRequest.number }}</a
            >
            <span class="min-w-0 flex-1 truncate text-kira-md">{{ changeRequest.title }}</span>
            <TooltipIconButton
              icon="eye"
              :label="t('space.ade.link.view.tooltip', { label: words.changeRequestShort })"
              class="shrink-0"
              data-testid="ade-pr-view"
              @click="fireAndForget(openView.mutateAsync(changeRequest.url), 'hostView.open')"
            />
            <TooltipIconButton
              :icon="copied && copiedKey === 'pr' ? 'check' : 'copy'"
              :label="t('space.ade.link.copy.tooltip', { label: words.changeRequestShort })"
              class="shrink-0"
              @click="copyLink('pr', changeRequest.url)"
            />
          </template>
        </dd>
      </KeyValueList>

      <AdeWorktreeSetup :row="row" />

      <AdeRunOutcome
        v-if="lastRebase"
        :run="lastRebase"
        :title="t('space.ade.branchPanel.lastRebase.label')"
        :acts="outcomeActs"
        listen
      />

      <div class="flex flex-col gap-0.5">
        <div class="pb-0.5 text-kira-sm text-muted-foreground">{{
          t('space.ade.branchPanel.mergedInto.label')
        }}</div>
        <div
          v-for="i in intoRows"
          :key="i.target"
          class="flex min-h-row items-center gap-2 rounded-kira bg-elevated px-1.5"
          data-testid="ade-branch-into"
          :data-target="i.target"
        >
          <AdeChip :label="t(INTO_STATUS[i.status])" :tone="INTO_TONE[i.status]" wide />
          <Identifier :value="i.target" />
          <span class="min-w-0 flex-1 truncate text-kira-sm text-muted-foreground">{{
            intoNote(i)
          }}</span>
          <Button
            variant="dialog"
            v-if="canMerge && i.status !== 'merged'"
            size="kira"
            class="shrink-0 "
            :class="ACTION_CLASS[i.status === 'stale' ? 'amber' : 'agent']"
            :pending="dialogs.pending.has(`merge:${branch.id}:${i.target}`)"
            data-testid="ade-branch-merge"
            @click="dialogs.merge(branch.id, i.target)"
          >
            {{
              i.status === 'stale'
                ? t('space.ade.needs.action.reMerge.label')
                : t('common.merge.label')
            }}
          </Button>
        </div>
        <InlineState v-if="intoRows.length === 0" kind="empty">{{
          t('space.ade.branchPanel.noIntegration.emptyHint')
        }}</InlineState>
      </div>

      <div class="flex flex-col gap-0.5">
        <div class="pb-0.5 text-kira-sm text-muted-foreground">{{
          t('space.ade.branchPanel.deployedTo.label')
        }}</div>
        <div
          v-for="d in deployRows"
          :key="d.env"
          class="flex min-h-row items-center gap-2 rounded-kira bg-elevated px-1.5"
          data-testid="ade-branch-deploy"
          :data-env="d.env"
        >
          <AdeChip :label="t(DEPLOY_STATUS[d.status])" :tone="DEPLOY_TONE[d.status]" wide />
          <Identifier :value="d.env" />
          <span class="min-w-0 truncate text-kira-sm text-muted-foreground">{{
            deployNote(d)
          }}</span>
        </div>
        <InlineState v-if="deployRows.length === 0" kind="empty">{{
          t('space.ade.branchPanel.noEnvironments.emptyHint')
        }}</InlineState>
      </div>
    </div>
    <AdeSessionsTab v-else :task-id="card.task.id" :branch-id="branch.id" />

    <AdeForcePushDialog
      :target="forcePush ? { branchId: branch.id, branchName: branch.name, repo: row.repo } : null"
      @close="forcePush = false"
    />
  </AdePanelFrame>
</template>
