<script setup lang="ts">
import { moduleShortcut } from '@shared/domain/moduleShortcuts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import {
  Popover,
  PopoverBar,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
} from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { refDebounced } from '@vueuse/core';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import { computed, ref, watch } from 'vue';
import { useRepos } from '../../repo/state/reposQueries';
import AdeLinkTarget from './AdeLinkTarget.vue';
import { firstWork, offsetToIso } from './board/calendar';
import { candidateOf, candidateOptions } from './branchOptions';
import { parseJira } from './jira';
import { AdeNotesEditor, prefetchNotesEditor } from './notes/asyncEditor';
import { usePlanModel } from './plan/usePlanModel';
import {
  useAddExistingBranch,
  useAddHostLink,
  useCandidates,
  useCreateTask,
  useResolveHostLink,
} from './queries';
import { useAdeAddUiStore } from './state/adeAddUi';
import { useAdeBoardUiStore } from './state/adeBoardUi';
import { useAdeOpsStore } from './state/adeOps';

// Add: a new task (lands Today, or the next work day; repos are added later) or an existing branch.
const ui = useAdeBoardUiStore();
const addUi = useAdeAddUiStore();
const open = computed({
  get: () => addUi.addOpen,
  set: (v) => {
    addUi.addOpen = v;
  },
});
const attachTo = computed(() => addUi.attachTo);
const tab = ref('new');
const tabItems = computed(() => [
  ...(addUi.attachTo
    ? []
    : [{ value: 'new', label: t('space.ade.board.newTask.label'), testid: 'ade-add-tab-new' }]),
  { value: 'branch', label: t('space.ade.add.tabBranch.label'), testid: 'ade-add-tab-branch' },
]);
const surface = useTaskAction();
const error = surface.error;

const repos = useRepos();
const repoLabel = (id: string): string => {
  const r = repos.data.value?.repos.find((x) => x.codeRepoId === id);
  return r?.nickname || r?.name || id;
};

const title = ref('');
const jira = ref('');
const notes = ref('');
const kind = ref<'task' | 'orchestrator'>('task');
const kindItems = computed(() => [
  { value: 'task', label: t('common.task.label'), testid: 'ade-nw-kind-task' },
  {
    value: 'orchestrator',
    label: t('space.ade.orchestrator.badge.label'),
    testid: 'ade-nw-kind-orchestrator',
  },
]);
const createTask = useCreateTask();
const { model, today } = usePlanModel();

watch(open, (o) => {
  if (!o) {
    addUi.attachTo = null;
    return;
  }
  surface.reset();
  if (addUi.attachTo) tab.value = 'branch';
});

const cantAdd = computed(() => title.value.trim() === '' || createTask.isPending.value);

async function addNew(): Promise<void> {
  const j = parseJira(jira.value);
  await surface.run(async () => {
    const task = await createTask.mutateAsync({
      title: title.value.trim(),
      jira: j.key ? j : null,
      hostUrl: '',
      notes: notes.value,
      codeRepoIds: [],
      workflowId: '',
      bases: {},
      day: model.value ? offsetToIso(today.value, firstWork(model.value.cal, 0)) : today.value,
      kind: kind.value,
    });
    ui.select(task.id);
    title.value = '';
    jira.value = '';
    notes.value = '';
    kind.value = 'task';
    open.value = false;
  });
}

const q = ref('');
const candidates = useCandidates(
  () => '',
  () => open.value && tab.value === 'branch',
);
const addBranch = useAddExistingBranch();
const ops = useAdeOpsStore();
const options = computed(() =>
  candidateOptions(candidates.data.value?.branches ?? [], repoLabel, Date.now()),
);

const isLink = computed(() => /^https?:\/\//i.test(q.value.trim()));
const linkUrl = refDebounced(
  computed(() => q.value.trim()),
  TIMING.search,
);
const link = useResolveHostLink(() => (isLink.value ? linkUrl.value : ''));
const addLink = useAddHostLink();
const linkPending = computed(
  () => link.isFetching.value || (isLink.value && linkUrl.value !== q.value.trim()),
);

async function pickLink(): Promise<void> {
  await surface.run(async () => {
    const res = await addLink.mutateAsync({
      url: linkUrl.value,
      taskId: attachTo.value?.taskId ?? '',
    });
    ui.select(res.task.id);
    open.value = false;
    q.value = '';
  });
}

async function pick(value: string): Promise<void> {
  surface.reset();
  const b = candidateOf(value);
  const taskId = attachTo.value?.taskId ?? '';
  open.value = false;
  // Adds in the background: chip on the task when attaching to one, error toast on failure.
  await ops.run(
    {
      id: `attachBranch:${b.codeRepoId}:${b.name}`,
      kind: 'attachBranch',
      taskId: taskId || undefined,
    },
    async () => {
      const res = await addBranch.mutateAsync({ codeRepoId: b.codeRepoId, name: b.name, taskId });
      ui.select(res.task.id);
    },
  );
}
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <TextTooltip
        :text="t('space.ade.add.trigger.label')"
        :shortcut="moduleShortcut('ade', 'app.new')"
      >
        <Button
          variant="toolbar"
          size="kira"
          data-testid="ade-add"
          @pointerenter="prefetchNotesEditor"
        >
          <CodiconIcon name="add" :size="13" />
          {{ t('space.ade.add.trigger.label') }}
        </Button>
      </TextTooltip>
    </PopoverTrigger>
    <PopoverContent align="end" flush class="w-120" data-testid="ade-add-popover">
      <PopoverBar>
        <SecondaryTabs v-model="tab" :items="tabItems" class="w-full" />
      </PopoverBar>
      <PopoverBody v-if="tab === 'new'" class="@container">
        <Field orientation="responsive">
          <FieldLabel for="ade-nw-title">{{ t('common.field.title.label') }}</FieldLabel>
          <Input
            id="ade-nw-title"
            v-model="title"
            :placeholder="t('space.ade.add.title.placeholder')"
            data-testid="ade-nw-title"
          />
        </Field>
        <Field orientation="responsive">
          <FieldLabel for="ade-nw-jira">{{ t('space.ade.taskTab.issue.label') }}</FieldLabel>
          <Input
            id="ade-nw-jira"
            v-model="jira"
            :placeholder="t('space.ade.add.jira.placeholder')"
            data-testid="ade-nw-jira"
          />
        </Field>
        <Field orientation="responsive">
          <FieldLabel for="ade-nw-notes">{{ t('space.common.notes.label') }}</FieldLabel>
          <AdeNotesEditor
            id="ade-nw-notes"
            v-model="notes"
            compact
            :placeholder="t('space.ade.add.notes.placeholder')"
            data-testid="ade-nw-notes"
          />
        </Field>
        <Field orientation="responsive">
          <FieldLabel>{{ t('common.field.kind.label') }}</FieldLabel>
          <SecondaryTabs
            variant="choice"
            size="kira-lg"
            :aria-label="t('common.field.kind.label')"
            :items="kindItems"
            :model-value="kind"
            data-testid="ade-nw-kind"
            @update:model-value="(v) => (kind = v === 'orchestrator' ? 'orchestrator' : 'task')"
          />
          <FieldDescription v-if="kind === 'orchestrator'" data-testid="ade-nw-kind-hint">{{
            t('space.ade.orchestrator.kind.orchestrator.hint')
          }}</FieldDescription>
        </Field>
        <Field orientation="responsive">
          <div class="flex items-center gap-3">
            <Button
              variant="dialog-primary"
              size="kira"
              :disabled="cantAdd"
              data-testid="ade-nw-add"
              @click="addNew"
            >
              {{ t('space.ade.add.addToToday.label') }}
            </Button>
            <span class="text-kira-sm text-subtle">{{
              t('space.ade.add.noBranchesYet.hint')
            }}</span>
          </div>
        </Field>
      </PopoverBody>
      <div v-else>
        <PopoverBody v-if="attachTo">
          <FieldDescription class="truncate" data-testid="ade-add-attach">
            <CatalogText k="space.ade.add.attach.label">
              <template #title
                ><span class="font-medium text-fg">{{ attachTo.title }}</span></template
              >
            </CatalogText>
          </FieldDescription>
        </PopoverBody>
        <BranchPicker
          v-model:search="q"
          inline
          testid="ade-branch"
          :branches="options"
          :placeholder="t('space.ade.add.searchBranches.placeholder')"
          :search-placeholder="t('space.ade.add.searchBranches.placeholder')"
          :empty-text="isLink ? '' : undefined"
          :loading="candidates.isPending.value"
          @select="(o) => pick(o.value)"
        >
          <template #top>
            <AdeLinkTarget
              v-if="isLink"
              :target="link.data.value"
              :repo="repoLabel(link.data.value?.codeRepoId ?? '')"
              :pending="linkPending"
              :error="link.error.value ? errorText(link.error.value) : ''"
              :busy="addLink.isPending.value"
              :attach="!!attachTo"
              @add="pickLink"
            />
          </template>
        </BranchPicker>
      </div>
      <PopoverBody v-if="error">
        <FieldError data-testid="ade-add-error">{{ error }}</FieldError>
      </PopoverBody>
    </PopoverContent>
  </Popover>
</template>
