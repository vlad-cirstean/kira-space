<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { usePendingDecision } from '@workbench/behaviour/pendingDecision';
import { computed } from 'vue';
import { useAdeBranchRequestStore } from '../state/adeBranchRequest';
import type { BranchRequestItem } from '../wire';

// An agent asks for branches and worktrees; the user keeps the branches they allow. `routed` picks which
// mount shows it: the ADE shell for the Task tab row, the prompt host for the routed popup. Escape and
// the close button only hide it: an accidental key must not answer for the user.
const props = defineProps<{ routed: boolean }>();
const emit = defineEmits<{ hide: [] }>();
const store = useAdeBranchRequestStore();

const open = computed(() => store.isOpen && store.routed === props.routed);
const request = computed(() => store.request);
const { remainingSeconds } = usePendingDecision({
  pendingId: () => (open.value ? request.value?.id : undefined),
  expiresAtMs: () => request.value?.expiresAt,
});

const source = computed(() => {
  const r = request.value;
  if (!r) return '';
  if (r.source === 'automation') return t('space.ade.branchRequest.source.automation.label');
  if (r.source === 'run')
    return t('space.ade.branchRequest.source.run.label', { step: r.stepName });
  return r.stepName
    ? t('space.ade.branchRequest.source.session.label', { step: r.stepName })
    : t('space.settings.agentUsage.session.label');
});

function itemText(item: BranchRequestItem): string {
  if (item.kind === 'worktree') {
    return t('space.ade.branchRequest.item.worktree.label', { branch: item.branch });
  }
  return item.base
    ? t('space.ade.branchRequest.item.branch.label', { branch: item.branch, base: item.base })
    : t('space.ade.branchRequest.item.branchMain.label', { branch: item.branch });
}

function hide(): void {
  store.close();
  emit('hide');
}
async function approve(): Promise<void> {
  if (await store.approve()) emit('hide');
}
async function deny(): Promise<void> {
  if (await store.deny()) emit('hide');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && hide()">
    <DialogContent size="md" data-testid="ade-branch-request-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('space.ade.branchRequest.title.title', { task: request?.taskTitle ?? '' })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody class="flex flex-col gap-3 text-kira-md">
        <span class="text-muted-foreground" data-testid="ade-branch-request-source">{{
          source
        }}</span>
        <ul class="m-0 flex list-none flex-col gap-1 p-0">
          <li
            v-for="item in request?.items ?? []"
            :key="item.id"
            class="flex items-center gap-2"
            :data-testid="`ade-branch-request-item-${item.id}`"
          >
            <Checkbox
              :id="`ade-branch-request-${item.id}`"
              :model-value="store.selected.includes(item.id)"
              @update:model-value="store.toggle(item.id)"
            />
            <Label :for="`ade-branch-request-${item.id}`" class="font-medium">{{
              item.repo
            }}</Label>
            <span class="text-muted-foreground">{{ itemText(item) }}</span>
          </li>
        </ul>
        <FieldDescription>{{ t('space.ade.branchRequest.naming.hint') }}</FieldDescription>
        <span class="text-subtle" data-testid="ade-branch-request-expires">{{
          t('common.expiresIn.status', { seconds: remainingSeconds })
        }}</span>
        <Alert v-if="store.error" variant="error" data-testid="ade-branch-request-error">
          <AlertDescription>{{ store.error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          ref="denyButton"
          variant="dialog"
          size="kira-lg"
          data-testid="ade-branch-request-deny"
          @click="deny"
          >{{
            t('common.action.deny.label')
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="store.pending"
          :disabled="store.selected.length === 0"
          data-testid="ade-branch-request-approve"
          @click="approve"
          >{{
            t('space.ade.branchRequest.approve.label', { count: store.selected.length })
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
