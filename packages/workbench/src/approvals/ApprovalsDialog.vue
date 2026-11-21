<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { connColorVar } from '@theme/connColor';
import { t } from '@theme/i18n';
import { useResizeObserver, useScroll } from '@vueuse/core';
import ColorLine from '@workbench/components/ColorLine.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, ref, useTemplateRef } from 'vue';
import ApprovalItemActions from './ApprovalItemActions.vue';
import ApprovalItemCard from './ApprovalItemCard.vue';
import { useApprovalActions } from './approvalsQueries';
import { blankInput } from './blankInput';
import { useApprovalKinds } from './registry';

const props = defineProps<{ items: ApprovalItem[] }>();

// Divider over the footer while the body hides content below, so a cut-off last line reads as
// "scroll for more" instead of clipped by the buttons.
const bodyRef = useTemplateRef<{ $el: HTMLElement }>('body');
const contentRef = useTemplateRef<HTMLElement>('content');
const bodyEl = computed(() => bodyRef.value?.$el ?? null);
const { arrivedState, measure } = useScroll(bodyEl);
// The colour line spans the content: left inset = content padding, right inset adds the body's
// scrollbar gutter while it scrolls, so the line ends where the boxes end.
const gutter = ref(0);
useResizeObserver(contentRef, () => {
  measure();
  const el = bodyEl.value;
  gutter.value = el ? el.offsetWidth - el.clientWidth : 0;
});
const moreBelow = computed(() => !arrivedState.bottom);

const store = useApprovalsStore();
const { answer, pending } = useApprovalActions();

// One prompt at a time; answering it moves `current` to the next oldest.
const current = computed(
  () => props.items.find((i) => i.id === store.focusId) ?? (props.items[0] as ApprovalItem),
);
const lineColor = computed(() => connColorVar(current.value.target.color));
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
  if (
    item.kind in kinds &&
    !store.isBroken(item.id) &&
    !store.answering.includes(item.id) &&
    blankInput(kinds[item.kind], item, store.inputs[item.id]) === null
  ) {
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
      size="2xl"
      fixed-height
      prompt
      data-testid="approvals-dialog"
      @keydown.capture="onChord"
    >
      <DialogHeader class="relative flex-row items-center gap-2">
        <DialogTitle>{{ t('workbench.approvals.dialog.title') }}</DialogTitle>
        <DialogDescription class="flex items-center gap-1.5">
          <Badge v-if="items.length > 1" variant="count" data-testid="approvals-position">
            {{ t('common.ofTotal.label', { value: position, total: items.length }) }}
          </Badge>
        </DialogDescription>
        <ColorLine
          v-if="lineColor"
          variant="under"
          :color="lineColor"
          :inset="{ left: '0.75rem', right: `calc(0.75rem + ${gutter}px)` }"
          data-testid="approvals-conn-line"
        />
      </DialogHeader>

      <DialogBody ref="body" flush>
        <div ref="content" class="flex flex-1 flex-col px-3">
          <ApprovalItemCard :key="current.id" :item="current" />
        </div>
      </DialogBody>

      <DialogFooter
        class="border-t"
        :class="moreBelow ? 'border-border' : 'border-transparent'"
        data-testid="approvals-footer"
        :data-more-below="moreBelow"
      >
        <ApprovalItemActions :key="current.id" :item="current">
          <template v-if="items.length > 1" #start>
            <Button
              variant="link-meta"
              size="inline"
              :pending="pending"
              data-testid="approvals-deny-all"
              @click="denyAll"
            >
              {{ t('workbench.approvals.denyAll.label', { count: items.length }) }}
            </Button>
          </template>
        </ApprovalItemActions>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
