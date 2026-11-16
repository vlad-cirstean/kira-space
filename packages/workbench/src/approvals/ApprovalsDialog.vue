<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';
import ApprovalItemActions from './ApprovalItemActions.vue';
import ApprovalItemCard from './ApprovalItemCard.vue';
import { useApprovalActions } from './approvalsQueries';
import { useApprovalKinds } from './registry';

const props = defineProps<{ items: ApprovalItem[] }>();

const store = useApprovalsStore();
const { answer, pending } = useApprovalActions();

// One prompt at a time; answering it moves `current` to the next oldest.
const current = computed(
  () => props.items.find((i) => i.id === store.focusId) ?? (props.items[0] as ApprovalItem),
);
const position = computed(() => props.items.findIndex((i) => i.id === current.value.id) + 1);
// Only the shown item can be approved, by its own click. Deny is safe in bulk. Ids are captured at
// click time: an item that joins mid-click stays pending.
function denyAll(): void {
  void answer(
    props.items.map((i) => i.id),
    false,
  );
}

// Two-key chords only: Mod+Enter approves and Mod+Backspace denies the shown item. Single keys do
// nothing here, so text typed elsewhere can never answer a request.
const kinds = useApprovalKinds();
const approveKey = shortcutKeydown(['form.submit'], () => {
  const item = current.value;
  if (item.kind in kinds && !store.isBroken(item.id) && !store.answering.includes(item.id)) {
    void answer([item.id], true);
  }
});
const denyKey = shortcutKeydown(
  ['prompt.deny'],
  () => {
    if (!store.answering.includes(current.value.id)) void answer([current.value.id], false);
  },
  { ignoreTextFields: true },
);
function onChord(event: KeyboardEvent): void {
  approveKey(event);
  denyKey(event);
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && store.minimise()">
    <DialogContent
      size="full"
      fixed-height
      prompt
      data-testid="approvals-dialog"
      @keydown.capture="onChord"
    >
      <DialogHeader>
        <DialogTitle>{{ t('workbench.approvals.dialog.title') }}</DialogTitle>
        <Badge variant="count" data-testid="approvals-count">{{ items.length }}</Badge>
        <Badge v-if="items.length > 1" variant="count" data-testid="approvals-position">
          {{ t('common.ofTotal.label', { value: position, total: items.length }) }}
        </Badge>
      </DialogHeader>

      <DialogBody class="gap-0 py-0">
        <ApprovalItemCard :key="current.id" :item="current" />
      </DialogBody>

      <DialogFooter class="flex-col items-stretch gap-2">
        <div
          class="flex items-center gap-1.5"
          :class="items.length === 1 && 'invisible'"
          data-testid="approvals-bulk"
        >
          <Button
            variant="dialog-deny"
            size="kira"
            :pending="pending"
            data-testid="approvals-deny-all"
            @click="denyAll"
          >
            {{ t('workbench.approvals.denyAll.label', { count: items.length }) }}
          </Button>
        </div>
        <ApprovalItemActions :key="current.id" :item="current" />
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
