<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import { computed, ref, watch } from 'vue';
import { useRepos } from '../../repo/state/reposQueries';
import AdeLinkTarget from './AdeLinkTarget.vue';
import { firstWork, offsetToIso } from './board/calendar';
import { candidateOf, candidateOptions } from './branchOptions';
import { parseJira } from './jira';
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
const error = ref('');

const repos = useRepos();
const repoLabel = (id: string): string => {
  const r = repos.data.value?.repos.find((x) => x.codeRepoId === id);
  return r?.nickname || r?.name || id;
};

const title = ref('');
const jira = ref('');
const notes = ref('');
const createTask = useCreateTask();
const { model, today } = usePlanModel();

watch(open, (o) => {
  if (!o) {
    addUi.attachTo = null;
    return;
  }
  error.value = '';
  if (addUi.attachTo) tab.value = 'branch';
});

const cantAdd = computed(() => title.value.trim() === '' || createTask.isPending.value);

async function addNew(): Promise<void> {
  error.value = '';
  const j = parseJira(jira.value);
  try {
    const task = await createTask.mutateAsync({
      title: title.value.trim(),
      jira: j.key ? j : null,
      hostUrl: '',
      notes: notes.value,
      codeRepoIds: [],
      workflowId: '',
      bases: {},
      day: model.value ? offsetToIso(today.value, firstWork(model.value.cal, 0)) : today.value,
    });
    ui.select(task.id);
    title.value = '';
    jira.value = '';
    notes.value = '';
    open.value = false;
  } catch (err) {
    error.value = errorText(err);
  }
}

const q = ref('');
const candidates = useCandidates(
  () => '',
  () => open.value && tab.value === 'branch',
);
const addBranch = useAddExistingBranch();
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
  error.value = '';
  try {
    const res = await addLink.mutateAsync({
      url: linkUrl.value,
      taskId: attachTo.value?.taskId ?? '',
    });
    ui.select(res.task.id);
    open.value = false;
    q.value = '';
  } catch (err) {
    error.value = errorText(err);
  }
}

async function pick(value: string): Promise<void> {
  error.value = '';
  const b = candidateOf(value);
  try {
    const res = await addBranch.mutateAsync({
      codeRepoId: b.codeRepoId,
      name: b.name,
      taskId: attachTo.value?.taskId ?? '',
    });
    ui.select(res.task.id);
    open.value = false;
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button variant="dialog" size="kira-lg" data-testid="ade-add">
        <CodiconIcon name="add" :size="12" />
        {{ t('space.ade.add.trigger.label') }}
      </Button>
    </PopoverTrigger>
    <PopoverContent
      align="end"
      class="w-120 gap-0 overflow-hidden p-0"
      data-testid="ade-add-popover"
    >
      <div class="p-1">
        <SecondaryTabs v-model="tab" :items="tabItems" size="kira-lg" class="w-full" />
      </div>
      <div
        v-if="tab === 'new'"
        class="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-2 p-3 text-kira-md"
      >
        <Label for="ade-nw-title" class="text-muted-foreground">{{
          t('common.field.title.label')
        }}</Label>
        <Input
          id="ade-nw-title"
          v-model="title"
          :placeholder="t('space.ade.add.title.placeholder')"
          data-testid="ade-nw-title"
        />
        <Label for="ade-nw-jira" class="text-muted-foreground">{{
          t('space.ade.taskTab.issue.label')
        }}</Label>
        <Input
          id="ade-nw-jira"
          v-model="jira"
          :placeholder="t('space.ade.add.jira.placeholder')"
          data-testid="ade-nw-jira"
        />
        <Label for="ade-nw-notes" class="self-start pt-1.5 text-muted-foreground">{{
          t('space.common.notes.label')
        }}</Label>
        <Textarea
          id="ade-nw-notes"
          v-model="notes"
          :placeholder="t('space.ade.add.notes.placeholder')"
          class="resize-y"
          data-testid="ade-nw-notes"
        />
        <span />
        <div class="flex items-center gap-3">
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :disabled="cantAdd"
            data-testid="ade-nw-add"
            @click="addNew"
          >
            {{ t('space.ade.add.addToToday.label') }}
          </Button>
          <span class="text-kira-sm text-subtle">{{ t('space.ade.add.noBranchesYet.hint') }}</span>
        </div>
      </div>
      <div v-else>
        <FieldDescription v-if="attachTo" class="truncate px-3 py-1.5" data-testid="ade-add-attach">
          <CatalogText k="space.ade.add.attach.label">
            <template #title
              ><span class="font-medium text-fg">{{ attachTo.title }}</span></template
            >
          </CatalogText>
        </FieldDescription>
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
      <FieldError v-if="error" class="px-3 pb-2" data-testid="ade-add-error">{{
        error
      }}</FieldError>
    </PopoverContent>
  </Popover>
</template>
