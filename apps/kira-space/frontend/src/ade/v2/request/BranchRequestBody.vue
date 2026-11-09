<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { taskTitle } from '../board/labels';
import { useBoard } from '../queries';
import { type BranchRequestItem, branchRequestPayloadSchema } from './payload';

// One approvals item for a task's branch calls. Concurrent calls merge into it, so the items group by
// call (ids are c<call>i<k>). Unchecking every item of a call denies that call.
const props = defineProps<{ item: ApprovalItem }>();
const emit = defineEmits<{
  options: [{ itemIds: string[]; remoteItemIds: string[]; shownItemIds: string[] }];
}>();

const board = useBoard();
const payload = computed(() => branchRequestPayloadSchema.parse(props.item.payload));

const groups = computed(() => {
  const byCall = new Map<string, BranchRequestItem[]>();
  for (const it of payload.value.items) {
    const call = it.id.split('i')[0] ?? it.id;
    byCall.set(call, [...(byCall.get(call) ?? []), it]);
  }
  return [...byCall.entries()].map(([call, items]) => ({ call, items }));
});

const unchecked = ref<string[]>([]);
const remote = ref<string[]>([]);
const selected = computed(() =>
  payload.value.items.map((i) => i.id).filter((id) => !unchecked.value.includes(id)),
);

watch(
  [payload, unchecked, remote],
  () =>
    emit('options', {
      itemIds: selected.value,
      remoteItemIds: remote.value.filter((id) => selected.value.includes(id)),
      shownItemIds: payload.value.items.map((i) => i.id),
    }),
  { immediate: true },
);

function toggle(id: string): void {
  unchecked.value = unchecked.value.includes(id)
    ? unchecked.value.filter((u) => u !== id)
    : [...unchecked.value, id];
}

function toggleRemote(id: string): void {
  remote.value = remote.value.includes(id)
    ? remote.value.filter((r) => r !== id)
    : [...remote.value, id];
}

function source(it: BranchRequestItem): string {
  if (it.source === 'automation') return t('space.ade.branchRequest.source.automation.label');
  if (it.source === 'run')
    return t('space.ade.branchRequest.source.run.label', { step: it.stepName });
  return it.stepName
    ? t('space.ade.branchRequest.source.session.label', { step: it.stepName })
    : t('space.settings.agentUsage.session.label');
}

// The holder of an adopt item: its task and worktree, the main checkout, or the path alone.
function holderText(it: BranchRequestItem): string {
  const h = it.holder;
  if (!h) return '';
  if (h.kind !== 'task') return h.worktree; // main has its own label
  const data = board.data.value;
  const task = data?.tasks.find((x) => x.id === h.taskId);
  const title = task
    ? taskTitle(
        task,
        data?.branches.find((b) => b.taskId === task.id),
        '',
      )
    : t('space.ade.board.newTask.label');
  return `${title}, ${h.worktree}`;
}

function itemText(it: BranchRequestItem): string {
  if (it.kind === 'checkout') {
    return t('space.ade.branchRequest.item.checkout.label', { branch: it.branch });
  }
  if (it.kind === 'adopt') {
    return it.holder?.kind === 'main'
      ? t('space.ade.branchRequest.item.adoptMain.label', { branch: it.branch })
      : t('space.ade.branchRequest.item.adopt.label', {
          branch: it.branch,
          holder: holderText(it),
        });
  }
  if (it.kind === 'rename') {
    return t('space.ade.branchRequest.item.rename.label', { from: it.from, branch: it.branch });
  }
  if (it.kind === 'worktree') {
    return t('space.ade.branchRequest.item.worktree.label', { branch: it.branch });
  }
  return it.base
    ? t('space.ade.branchRequest.item.branch.label', { branch: it.branch, base: it.base })
    : t('space.ade.branchRequest.item.branchMain.label', { branch: it.branch });
}

const stacked = computed(() =>
  payload.value.items
    .filter((i) => i.kind === 'rename' && selected.value.includes(i.id))
    .flatMap((i) => i.stacked),
);
const hasCreate = computed(() => payload.value.items.some((i) => i.kind !== 'rename'));
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="ade-branch-request-body">
    <section
      v-for="group in groups"
      :key="group.call"
      class="flex flex-col gap-1"
      :data-testid="`ade-branch-request-call-${group.call}`"
    >
      <span class="text-muted-foreground" data-testid="ade-branch-request-source">
        {{ group.items[0] ? source(group.items[0]) : '' }}
      </span>
      <ul class="m-0 flex list-none flex-col gap-1 p-0">
        <li
          v-for="it in group.items"
          :key="it.id"
          class="flex flex-col gap-0.5"
          :data-testid="`ade-branch-request-item-${it.id}`"
        >
          <div class="flex items-center gap-2">
            <Checkbox
              :id="`ade-branch-request-${item.id}-${it.id}`"
              :model-value="selected.includes(it.id)"
              @update:model-value="toggle(it.id)"
            />
            <Label :for="`ade-branch-request-${item.id}-${it.id}`" class="font-medium">{{
              it.repo
            }}</Label>
            <span class="text-muted-foreground">{{ itemText(it) }}</span>
          </div>
          <div
            v-if="it.kind === 'adopt'"
            class="flex flex-col gap-0.5 pl-6"
            :data-testid="`ade-branch-request-holder-${it.id}`"
          >
            <FieldDescription v-if="it.dirty" :data-testid="`ade-branch-request-dirty-${it.id}`">{{
              t('space.ade.branchRequest.dirty.hint', { count: it.dirty })
            }}</FieldDescription>
            <FieldDescription v-if="it.holder?.kind === 'main'">{{
              t('space.ade.branchRequest.mainCheckout.hint')
            }}</FieldDescription>
          </div>
          <div v-if="it.kind === 'rename' && it.upstream" class="flex items-center gap-2 pl-6">
            <Checkbox
              :id="`ade-branch-request-remote-${item.id}-${it.id}`"
              :model-value="remote.includes(it.id)"
              :disabled="!selected.includes(it.id)"
              :data-testid="`ade-branch-request-remote-${it.id}`"
              @update:model-value="toggleRemote(it.id)"
            />
            <Label :for="`ade-branch-request-remote-${item.id}-${it.id}`">{{
              t('space.ade.branchRequest.renameRemote.label', { upstream: it.upstream })
            }}</Label>
          </div>
        </li>
      </ul>
    </section>
    <FieldDescription v-if="stacked.length > 0" data-testid="ade-branch-request-stacked">{{
      t('space.ade.branchRequest.stacked.hint', { branches: stacked.join(', ') })
    }}</FieldDescription>
    <FieldDescription v-if="hasCreate">{{
      t('space.ade.branchRequest.naming.hint')
    }}</FieldDescription>
  </div>
</template>
