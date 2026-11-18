<script setup lang="ts">
/**
 * `RefsPopover.vue`'s Stacks tab — one sub-list per `StackSummary`, rows
 * pre-order bottom-to-top with `depth`-driven indent, a PR badge and track text reused verbatim
 * from `ChangeRequestState.byBranch`/`RefRow`, a stale chip, and a header "Restack" button
 * per stack. Row-level "Set stack parent…"/"Remove from stack" mirror `WorktreeList.vue`'s own
 * "this component has nowhere of its own to do X" shape: both EMIT the intent for `App.vue` to
 * open `StackDialog.vue` with, rather than this file owning a second dialog. The Restack button
 * does the same — `StackDialog.vue` is the one place the plan/blockers/progress render.
 *
 * `stacks`/`orphans` are already filtered/capped by `pickerModel.ts` (each stack group
 * carries its own already-`StackSummary`, so the per-stack header's "Restack" target/`needsRestack`
 * gate reads `group.summary` directly rather than a second, unfiltered lookup) — this component no
 * longer reads `stack.stacks.value`/`stack.orphans.value` for rendering, and no longer needs the
 * `stack` prop at all (nothing else in this file ever read it).
 */
import type { StackBranch } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta } from '@theme/lib/rowVariants';
import { changeRequestBadgeClass, refBadgeClass } from '../lib/badgeClass.ts';
import type { PickerList, PickerStackGroup } from '../lib/pickerModel.ts';
import {
  buildOrphanRows,
  buildStackRows,
  changeRequestBadgeLabel,
  type StackRow,
} from '../lib/stackListModel.ts';
import type { ChangeRequestState } from '../state/changeRequests.ts';
import type { OpsState } from '../state/ops.ts';

const props = defineProps<{
  stacks: PickerList<PickerStackGroup>;
  orphans: PickerList<StackBranch>;
  ops: OpsState;
  /** G24 D9's own branch-tip badge — optional so a caller with nothing to show yet gets a
   *  plain, badge-free list (mirrors `RefsPopover.vue`'s own `pr` prop). */
  changeRequests?: ChangeRequestState;
  /** P74 §3.3 — see `RefsPopover.vue`'s own doc comment, threaded straight through from
   *  `AppToolbar.vue`. */
  openChangeRequest: (number: number) => void;
  /** P77 §7.3 — see `TagList.vue`'s own doc comment on this prop. */
  focusedRowId?: string;
}>();

const emit = defineEmits<{
  (e: 'openRestackDialog', branch: string): void;
  (e: 'openSetParentDialog', branch: string): void;
}>();

function byBranchMap() {
  return props.changeRequests?.byBranch.value ?? new Map();
}

function rowsFor(branches: readonly StackBranch[]): StackRow[] {
  // buildStackRows takes a whole StackSummary; this file only ever needs its own branches array,
  // so it builds a minimal summary-shaped wrapper rather than widening buildStackRows' own
  // signature for a caller nobody else has.
  return buildStackRows(
    { base: '', baseTip: undefined, branches, needsRestack: false },
    byBranchMap(),
  );
}

function orphanRows(): StackRow[] {
  return buildOrphanRows(props.orphans.visible, byBranchMap());
}

function requestRestack(branch: string): void {
  emit('openRestackDialog', branch);
}

function requestSetParent(branch: string): void {
  emit('openSetParentDialog', branch);
}

/** No confirmation dialog: this is a config-only, fully undoable write (D2/D10) — the same
 *  "reversible, one click" posture `stashDrop`'s own row action already takes. */
async function removeFromStack(branch: string): Promise<void> {
  await props.ops.runStackSet(branch, undefined);
}
</script>

<template>
  <section :aria-label="t('git.stack.list.label')">
    <SectionHeading :label="t('git.stack.list.label')" />

    <div v-for="group in stacks.visible" :key="group.summary.base" class="mb-1">
      <SectionHeading
        data
        :label="group.summary.base"
        :data-kira-tip="t('git.stack.base.tooltip', { base: group.summary.base })"
      >
        <Button
          variant="toolbar"
          size="kira"
          :disabled="!group.summary.needsRestack"
          @click="
            requestRestack(
              group.summary.branches[group.summary.branches.length - 1]?.name ?? group.summary.base,
            )
          "
          ><CodiconIcon name="layers" :size="13" />
          {{ t('git.common.restack.label') }}
        </Button>
      </SectionHeading>

      <div
        data-testid="branch-row"
        v-for="row in rowsFor(group.branches)"
        :key="row.name"
        class="flex items-center gap-0.5 pr-1"
        :style="rowIndent(row.depth)"
        :data-row-id="`stack:${row.name}`"
        :tabindex="focusedRowId === `stack:${row.name}` ? 0 : -1"
      >
        <div
          data-testid="branch-row-main"
          class="flex items-center gap-0.5 flex-1 min-w-0 text-left"
        >
          <StatusDot
            v-if="row.isHead"
            tone="muted"
            :data-kira-tip="t('git.common.currentBranch.tooltip')"
          />
          <span class="truncate">{{ row.name }}</span>
          <span v-if="row.stale" :class="refBadgeClass('warn')" :data-kira-tip="row.staleText">
            {{ t('common.stale.status') }}
          </span>
          <button
            v-if="row.pr"
            type="button"
            :class="changeRequestBadgeClass(row.pr.state)"
            :data-kira-tip="row.pr.title"
            @click="openChangeRequest(row.pr.number)"
          >
            {{ changeRequestBadgeLabel(row.pr) }}
          </button>
          <span v-if="row.trackText" :class="rowMeta">{{ row.trackText }}</span>
          <span
            v-if="row.checkedOutIn"
            class="text-muted-foreground"
            :data-kira-tip="row.checkedOutIn"
          >
            <CodiconIcon name="repo" :size="13" />
          </span>
        </div>
        <TooltipIconButton
          icon="list-tree"
          :label="t('git.stack.setParent.tooltip')"
          @click="requestSetParent(row.name)"
        />
        <TooltipIconButton
          icon="close"
          :label="t('git.stack.remove.tooltip')"
          @click="removeFromStack(row.name)"
        />
      </div>
    </div>

    <div v-if="orphans.visible.length > 0" class="mb-1">
      <SectionHeading :label="t('git.stack.needsAttention.label')" />
      <div
        data-testid="branch-row"
        v-for="row in orphanRows()"
        :key="row.name"
        class="flex items-center gap-0.5 px-1"
        :data-row-id="`orphan:${row.name}`"
        :tabindex="focusedRowId === `orphan:${row.name}` ? 0 : -1"
      >
        <div
          data-testid="branch-row-main"
          class="flex items-center gap-0.5 flex-1 min-w-0 text-left"
        >
          <span class="truncate">{{ row.name }}</span>
          <span class="truncate text-kira-sm text-error">{{ row.orphanReason }}</span>
        </div>
        <TooltipIconButton
          icon="list-tree"
          :label="t('git.stack.setParent.tooltip')"
          @click="requestSetParent(row.name)"
        />
      </div>
    </div>

    <InlineState v-if="stacks.visible.length === 0 && orphans.visible.length === 0" kind="empty">
      {{ t('git.stack.list.empty') }}
    </InlineState>
  </section>
</template>
