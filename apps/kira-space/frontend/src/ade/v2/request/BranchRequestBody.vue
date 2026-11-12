<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { taskTitle } from '../board/labels';
import { useBoard } from '../queries';
import { type BranchRequestItem, branchRequestPayloadSchema } from './payload';
import { branchSource } from './source';

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

// The task holding an adopt item's branch; the main checkout has its own tag.
function holderTitle(it: BranchRequestItem): string {
  const h = it.holder;
  if (h?.kind !== 'task') return h?.worktree ?? '';
  const data = board.data.value;
  const task = data?.tasks.find((x) => x.id === h.taskId);
  return task
    ? taskTitle(
        task,
        data?.branches.find((b) => b.taskId === task.id),
        '',
      )
    : t('space.ade.board.newTask.label');
}

function holderTag(it: BranchRequestItem): string {
  return it.holder?.kind === 'main'
    ? t('space.ade.branchRequest.tag.heldByMain.label')
    : t('space.ade.branchRequest.tag.heldBy.label', { task: holderTitle(it) });
}

// The kind of change a row makes, as a short muted tag.
function tagText(it: BranchRequestItem): string {
  if (it.kind === 'checkout') return t('common.action.checkOut.label');
  if (it.kind === 'adopt') {
    return [t('common.action.checkOut.label'), holderTag(it)].filter(Boolean).join(' · ');
  }
  if (it.kind === 'rename') return t('space.ade.branchRequest.tag.rename.label', { from: it.from });
  if (it.kind === 'switch') return t('space.ade.branchRequest.tag.change.label', { from: it.from });
  if (it.kind === 'worktree') return t('common.worktree.label');
  return it.base
    ? t('space.ade.branchRequest.tag.new.label', { base: it.base })
    : t('space.ade.branchRequest.tag.newMain.label');
}

// Risks a row carries: uncommitted changes that stay in the holder, a detached main checkout.
function rowRisks(it: BranchRequestItem): { text: string; testid?: string }[] {
  const out: { text: string; testid?: string }[] = [];
  if (it.kind === 'adopt' && it.dirty) {
    out.push({
      text: t('space.ade.branchRequest.risk.dirty.status', { count: it.dirty }),
      testid: `ade-branch-request-dirty-${it.id}`,
    });
  }
  if (it.kind === 'adopt' && it.holder?.kind === 'main') {
    out.push({ text: t('space.ade.branchRequest.risk.detachesMain.status') });
  }
  return out;
}
</script>

<template>
  <div
    class="grid grid-cols-[auto_max-content_max-content_minmax(0,1fr)] items-center gap-x-3 gap-y-1.5"
    data-testid="ade-branch-request-body"
  >
    <div
      v-for="group in groups"
      :key="group.call"
      class="contents"
      :data-testid="`ade-branch-request-call-${group.call}`"
    >
      <span
        v-if="groups.length > 1"
        class="col-span-full text-kira-sm text-subtle"
        data-testid="ade-branch-request-source"
      >
        {{ group.items[0] ? branchSource(group.items[0]) : '' }}
      </span>
      <div
        v-for="it in group.items"
        :key="it.id"
        class="contents"
        :data-testid="`ade-branch-request-item-${it.id}`"
      >
        <Checkbox
          :id="`ade-branch-request-${item.id}-${it.id}`"
          :model-value="selected.includes(it.id)"
          @update:model-value="toggle(it.id)"
        />
        <Label
          :for="`ade-branch-request-${item.id}-${it.id}`"
          class="font-data text-kira-md font-medium"
          >{{
            it.repo
          }}</Label
        >
        <span class="font-data text-kira-md">{{ it.branch }}</span>
        <div class="flex min-w-0 flex-wrap items-baseline gap-x-3">
          <Tooltip :disabled="it.kind !== 'adopt' || !it.holder?.worktree">
            <TooltipTrigger as-child>
              <span
                class="text-kira-md text-muted-foreground"
                :data-testid="
                  it.kind === 'adopt' ? `ade-branch-request-holder-${it.id}` : undefined
                "
                >{{
                  tagText(it)
                }}</span
              >
            </TooltipTrigger>
            <TooltipContent>{{ it.holder?.worktree }}</TooltipContent>
          </Tooltip>
          <span
            v-if="it.kind === 'rename' && it.stacked.length > 0 && selected.includes(it.id)"
            class="text-kira-md text-muted-foreground"
            data-testid="ade-branch-request-stacked"
            >{{
              t('space.ade.branchRequest.tag.stacked.label', { branches: it.stacked.join(', ') })
            }}</span
          >
          <span
            v-for="risk in rowRisks(it)"
            :key="risk.text"
            class="inline-flex items-baseline gap-1 whitespace-nowrap text-kira-md text-warn-text"
            :data-testid="risk.testid"
          >
            <CodiconIcon name="warning" :size="13" class="self-center text-warn" />
            {{ risk.text }}
          </span>
        </div>
        <div
          v-if="it.kind === 'rename' && it.upstream"
          class="col-span-3 col-start-2 flex items-center gap-1.5"
        >
          <Checkbox
            :id="`ade-branch-request-remote-${item.id}-${it.id}`"
            :model-value="remote.includes(it.id)"
            :disabled="!selected.includes(it.id)"
            :data-testid="`ade-branch-request-remote-${it.id}`"
            @update:model-value="toggleRemote(it.id)"
          />
          <Label :for="`ade-branch-request-remote-${item.id}-${it.id}`" class="text-kira-md">{{
            t('space.ade.branchRequest.renameRemote.label', { upstream: it.upstream })
          }}</Label>
        </div>
      </div>
    </div>
  </div>
</template>
