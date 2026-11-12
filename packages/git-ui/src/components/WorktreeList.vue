<script setup lang="ts">
/**
 * `RefsPopover.vue`'s fourth section — mirrors `StashList.vue`'s own "one row per
 * entry, row-level actions, no filter box" shape (a worktree list is rarely more than a handful of
 * entries). Switch reuses the existing `repo.open` flow (each worktree is already its own
 * `RepoEntry`/`RepoSummary` — "switch" needs no worktree-specific request at all), so this
 * component only EMITS the intent (`switch-worktree`) for `App.vue` to carry out, the same
 * "this component has nowhere of its own to do X" shape `StashList.vue`'s own `branchFromStash`
 * emit already uses. "Open in New Window" is the one action that needs the extension
 * (`worktree.openWindow`) — also emitted up rather than called directly, since this component
 * has no `BridgeClient` of its own (only `ops`/`worktrees`, matching every other row component in
 * this file's own family). Remove is handled entirely locally: a plain `op.run`, no host
 * involvement, with its own small confirm dialog for the dirty route.
 *
 * Renders the already filtered/ordered/capped `section` prop `pickerModel.ts` hands it —
 * the same `TagList.vue` contract. This is also the first cap this list has ever had (it
 * used to render `worktrees.entries.value` straight through, uncapped) — part of the fix.
 */
import type { WorktreeEntry, WorktreeRemovePreflight } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { rowMeta } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, ref } from 'vue';
import { type PickerList, worktreeLabel } from '../lib/pickerModel.ts';
import type { OpsState } from '../state/ops.ts';
import type { WorktreeState } from '../state/worktrees.ts';

const props = defineProps<{
  section: PickerList<WorktreeEntry>;
  worktrees: WorktreeState;
  ops: OpsState;
  /** P77 §7.3 — see `TagList.vue`'s own doc comment on this prop. */
  focusedRowId?: string;
}>();

const emit = defineEmits<{
  (e: 'switchWorktree', path: string): void;
  (e: 'createWorktree'): void;
}>();

function switchTo(entry: WorktreeEntry): void {
  if (entry.isCurrent) return;
  emit('switchWorktree', entry.path);
}

// ---------------------------------------------------------------------------------------
// Remove — a small, self-contained confirm dialog: blocked reasons are shown with no route
// forward; a dirty worktree requires typing the exact confirmation token the server's own fresh
// preflight names (never a value this component invents), mirroring the typed-
// confirmation pattern (`ResetDialog.vue`).
// ---------------------------------------------------------------------------------------

const pendingRemove = ref<{ entry: WorktreeEntry; preflight: WorktreeRemovePreflight } | undefined>(
  undefined,
);
const typedToken = ref('');

async function requestRemove(entry: WorktreeEntry): Promise<void> {
  const preflight = await props.worktrees.previewRemove(entry.path);
  if (!preflight) return;
  if (preflight.verdict === 'clean') {
    await props.ops.runWorktreeRemove(entry.path);
    return;
  }
  typedToken.value = '';
  pendingRemove.value = { entry, preflight };
}

function blockerText(preflight: WorktreeRemovePreflight): string {
  const b = preflight.blockers[0];
  if (!b) return '';
  switch (b.kind) {
    case 'notAWorktree':
      return t('git.worktree.blocker.notAWorktree.error', { path: b.path });
    case 'mainWorktree':
      return t('git.worktree.blocker.main.error');
    case 'currentWorktree':
      return t('git.worktree.blocker.current.error');
    case 'openInAnotherWindow':
      return t('git.worktree.blocker.openElsewhere.error');
    case 'locked':
      return t('git.worktree.blocker.locked.error', { reason: b.reason });
  }
}

const canConfirmRemove = computed(
  () =>
    pendingRemove.value !== undefined &&
    pendingRemove.value.preflight.verdict === 'dirty' &&
    typedToken.value === pendingRemove.value.preflight.confirmToken,
);

function cancelRemove(): void {
  pendingRemove.value = undefined;
}

async function confirmRemove(): Promise<void> {
  const pending = pendingRemove.value;
  if (!pending || !canConfirmRemove.value) return;
  pendingRemove.value = undefined;
  await props.ops.runWorktreeRemove(pending.entry.path, pending.preflight.confirmToken);
}
</script>

<template>
  <section :aria-label="t('common.module.worktrees.label')">
    <SectionHeading :label="t('common.module.worktrees.label')">
      <Button variant="toolbar" size="kira" class="ml-auto" @click="emit('createWorktree')">
        {{ t('git.worktree.create.label') }}
      </Button>
    </SectionHeading>
    <div
      data-testid="branch-row"
      v-for="entry in section.visible"
      :key="entry.path"
      class="flex items-center gap-0.5 px-1"
      :data-row-id="`worktree:${entry.path}`"
      :tabindex="focusedRowId === `worktree:${entry.path}` ? 0 : -1"
    >
      <div data-testid="branch-row-main" class="flex items-center gap-0.5 flex-1 min-w-0 text-left">
        <span
          v-if="entry.isCurrent"
          class="text-kira-sm text-subtle"
          :data-kira-tip="t('git.worktree.current.tooltip')"
          >●</span
        >
        <span
          v-if="entry.isMain"
          class="text-kira-sm text-subtle"
          :data-kira-tip="t('common.mainWorktree.tooltip')"
          >{{
            t('git.worktree.mainBadge.label')
          }}</span
        >
        <span
          v-if="entry.locked"
          class="text-kira-sm text-subtle"
          :data-kira-tip="entry.locked.reason"
        >
          <CodiconIcon name="lock" :size="13" />
        </span>
        <span
          v-if="entry.openElsewhere"
          class="text-kira-sm text-subtle"
          :data-kira-tip="t('common.openElsewhere.tooltip')"
        >
          <CodiconIcon name="window" :size="13" />
        </span>
        <span class="truncate" :data-kira-tip="entry.path">{{ worktreeLabel(entry) }}</span>
        <span :class="cn('flex-1 min-w-0', rowMeta)">
          <Identifier split="path" :value="entry.path" />
        </span>
      </div>
      <TooltipIconButton
        v-if="!entry.isCurrent"
        icon="arrow-swap"
        :label="t('git.worktree.switch.tooltip')"
        @click="switchTo(entry)"
      />
      <TooltipIconButton
        v-if="!entry.isMain && !entry.isCurrent"
        icon="trash"
        :label="t('git.worktree.remove.tooltip')"
        @click="requestRemove(entry)"
      />
    </div>
    <InlineState v-if="section.visible.length === 0" kind="empty">{{
      t('git.worktree.list.empty')
    }}</InlineState>

    <!-- P131 Part 2 §5: opens inside RefsPopover's own modal Popover panel (§5.2) -- reka's
         modal content there prevents the panel dismissing under this nested Dialog. -->
    <Dialog :open="pendingRemove !== undefined" @update:open="(v) => !v && cancelRemove()">
      <DialogContent :aria-describedby="undefined" size="md" :dirty="typedToken !== ''">
        <DialogHeader>
          <DialogTitle>{{
            pendingRemove
              ? t('git.worktree.removeConfirm.title', { path: pendingRemove.entry.path })
              : ''
          }}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <template v-if="pendingRemove?.preflight.verdict === 'blocked'">
            <FieldError>{{ blockerText(pendingRemove.preflight) }}</FieldError>
          </template>
          <template v-else-if="pendingRemove?.preflight.verdict === 'dirty'">
            <p>
              <CatalogText k="git.worktree.removeConfirm.body">
                <template #token
                  ><Identifier :value="pendingRemove.preflight.confirmToken ?? ''" /></template
                >
              </CatalogText>
            </p>
            <Input
              v-model="typedToken"
              size="kira-lg"
              class="w-full"
              :aria-label="t('git.common.confirmToken.aria')"
            />
          </template>
        </DialogBody>
        <DialogFooter>
          <Button variant="dialog" size="kira-lg" @click="cancelRemove">{{
            t('common.action.cancel.label')
          }}</Button>
          <Button
            v-if="pendingRemove?.preflight.verdict === 'dirty'"
            variant="dialog-danger"
            size="kira-lg"
            :disabled="!canConfirmRemove"
            @click="confirmRemove"
          >
            {{ t('common.action.remove.label') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
