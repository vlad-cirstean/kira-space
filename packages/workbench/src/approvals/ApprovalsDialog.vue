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
import { revealElement } from '@workbench/behaviour/focus';
import { useApprovalsStore } from '@workbench/state/approvals';
import { nextTick, watch } from 'vue';
import ApprovalItemCard from './ApprovalItemCard.vue';
import { useApprovalActions } from './approvalsQueries';

const props = defineProps<{ items: ApprovalItem[] }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();

// Ids are captured at click time: an item that joins mid-click stays pending.
function answerAll(approve: boolean): void {
  void answer(
    props.items.map((i) => i.id),
    approve,
  );
}

watch(
  () => store.focusId,
  async (id) => {
    if (!id) return;
    await nextTick();
    const el = document.querySelector(`[data-testid="approval-item-${CSS.escape(id)}"]`);
    if (el) revealElement(el, { block: 'start' });
  },
  { immediate: true },
);
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && store.minimise()">
    <DialogContent size="2xl" fixed-height data-testid="approvals-dialog">
      <DialogHeader>
        <DialogTitle>{{ t('workbench.approvals.dialog.title') }}</DialogTitle>
        <Badge variant="count" data-testid="approvals-count">{{ items.length }}</Badge>
      </DialogHeader>

      <DialogBody>
        <ApprovalItemCard v-for="(item, i) in items" :key="item.id" :item="item" :first="i === 0" />
      </DialogBody>

      <DialogFooter v-if="items.length > 1">
        <Button
          variant="dialog-danger"
          size="kira-lg"
          data-testid="approvals-deny-all"
          @click="answerAll(false)"
        >
          {{ t('workbench.approvals.denyAll.label', { count: items.length }) }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="approvals-approve-all"
          @click="answerAll(true)"
        >
          {{ t('workbench.approvals.approveAll.label', { count: items.length }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
