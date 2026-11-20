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
 * involvement. A refusal (blocked, or uncommitted changes) shows inline under the row; only the
 * dirty row offers a button that names the loss, no dialog.
 *
 * Renders the already filtered/ordered/capped `section` prop `pickerModel.ts` hands it —
 * the same `TagList.vue` contract. This is also the first cap this list has ever had (it
 * used to render `worktrees.entries.value` straight through, uncapped) — part of the fix.
 */
import type { WorktreeEntry, WorktreeRemovePreflight } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowMeta } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import Identifier from '@workbench/components/Identifier.vue';
import { ref, watch } from 'vue';
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
// Remove — a clean worktree goes at once. Blocked reasons and uncommitted changes show inline under
// the row; the dirty row's button passes the confirmation token the server's own fresh preflight
// names (never a value this component invents), and the server re-checks it before it removes.
// ---------------------------------------------------------------------------------------

const refusal = ref<{ path: string; preflight: WorktreeRemovePreflight } | undefined>(undefined);

// A list change drops a refusal that may no longer match its row.
watch(() => props.section.visible.map((e) => e.path).join('\0'), clearRefusal);

async function requestRemove(entry: WorktreeEntry): Promise<void> {
  refusal.value = undefined;
  const preflight = await props.worktrees.previewRemove(entry.path);
  if (!preflight) return;
  if (preflight.verdict === 'clean') {
    await props.ops.runWorktreeRemove(entry.path);
    return;
  }
  refusal.value = { path: entry.path, preflight };
}

function clearRefusal(): void {
  refusal.value = undefined;
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

async function discardRemove(): Promise<void> {
  const current = refusal.value;
  if (!current) return;
  refusal.value = undefined;
  await props.ops.runWorktreeRemove(current.path, current.preflight.confirmToken);
}
</script>

<template>
  <section :aria-label="t('common.module.worktrees.label')">
    <SectionHeading :label="t('common.module.worktrees.label')">
      <Button variant="toolbar" size="kira" @click="emit('createWorktree')"
        ><CodiconIcon :name="ICON.add" :size="13" />
        {{ t('git.worktree.create.label') }}
      </Button>
    </SectionHeading>
    <template v-for="entry in section.visible" :key="entry.path">
      <div
        data-testid="branch-row"
        class="flex items-center gap-0.5 px-1"
        :data-row-id="`worktree:${entry.path}`"
        :tabindex="focusedRowId === `worktree:${entry.path}` ? 0 : -1"
      >
        <div
          data-testid="branch-row-main"
          class="flex items-center gap-0.5 flex-1 min-w-0 text-left"
        >
          <StatusDot
            v-if="entry.isCurrent"
            tone="muted"
            :data-kira-tip="t('git.worktree.current.tooltip')"
          />
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
      <div
        v-if="refusal?.path === entry.path"
        class="flex flex-col gap-1 px-1 pb-1"
        data-testid="git-worktree-refusal"
      >
        <Alert v-if="refusal.preflight.verdict === 'blocked'" variant="error">
          <AlertDescription>{{ blockerText(refusal.preflight) }}</AlertDescription>
        </Alert>
        <template v-else>
          <Alert variant="warn">
            <AlertDescription>{{
              t('git.worktree.dirty.error', { path: entry.path })
            }}</AlertDescription>
          </Alert>
          <Button
            variant="dialog-danger"
            size="kira"
            data-testid="git-worktree-discard-remove"
            @click="discardRemove"
            >{{
              t('git.worktree.discardRemove.label')
            }}</Button
          >
        </template>
      </div>
    </template>
    <InlineState v-if="section.visible.length === 0" kind="empty">{{
      t('git.worktree.list.empty')
    }}</InlineState>
  </section>
</template>
