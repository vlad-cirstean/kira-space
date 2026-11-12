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
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';
import ApprovalItemActions from './ApprovalItemActions.vue';
import ApprovalItemCard from './ApprovalItemCard.vue';
import { useApprovalActions } from './approvalsQueries';

const props = defineProps<{ items: ApprovalItem[] }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();

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
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && store.minimise()">
    <DialogContent size="full" fixed-height data-testid="approvals-dialog">
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
