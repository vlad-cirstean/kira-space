<script setup lang="ts">
/**
 * Two modes, at most one active at a time (`props.target?.mode`), mirroring
 * `WorktreeDialog.vue`'s own "App.vue owns the state, this component owns nothing of its own"
 * shape:
 * - **setParent** — `op.run`'s `stackSet` kind directly: pick an existing local branch as the new
 *   parent (or "None" to remove `target.branch` from its stack). No preflight endpoint of
 *   its own (the cycle check runs host-side, at write time) — this dialog is the confirm step,
 *   the same posture `BranchDialog.vue`'s rename mode already takes.
 * - **restack** — `preflight.restack`'s own live plan/blockers (re-fetched on open), a live
 *   progress list driven by `StackState.progress` while `StackState.restacking` is true, and a
 *   Cancel button that calls `ops.cancelRestack()`. A paused (conflicted) restack is surfaced
 *   here in words — resolving it is the `ConflictBanner.vue`, not a second UI here.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now, `title` feeds
 * `DialogTitle`'s default slot, and the parent picker is the shared `BranchPicker` (workbench).
 */
import type { RestackPreflight } from '@kira/git-ipc';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import Identifier from '@workbench/components/Identifier.vue';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import { computed, ref, useId, watch } from 'vue';
import { branchOptions } from '../../lib/refOptions.ts';
import type { OpsState } from '../../state/ops.ts';
import type { RefsState } from '../../state/refs.ts';
import type { StackState } from '../../state/stack.ts';

const props = defineProps<{
  stack: StackState;
  ops: OpsState;
  refs: RefsState;
  /** `App.vue` owns this — `undefined` means closed. Toggled by `StackList.vue`'s own row/header
   *  actions, the row menu's stack section, and the `restackStack` palette action. */
  target: { readonly mode: 'setParent' | 'restack'; readonly branch: string } | undefined;
}>();

const emit = defineEmits<(e: 'close') => void>();

const open = computed(() => props.target !== undefined);

// ---------------------------------------------------------------------------------------
// setParent
// ---------------------------------------------------------------------------------------

const selectedParent = ref('');
const parentId = useId();

watch(
  () => props.target,
  (target) => {
    if (target === undefined || target.mode !== 'setParent') return;
    selectedParent.value = '';
  },
);

const parentCandidates = computed(() =>
  branchOptions(props.refs, { remote: false, exclude: props.target?.branch }),
);

async function submitSetParent(): Promise<void> {
  const target = props.target;
  if (target === undefined) return;
  const result = await props.ops.runStackSet(target.branch, selectedParent.value || undefined);
  if (!result.ok) return;
  emit('close');
}

// ---------------------------------------------------------------------------------------
// restack
// ---------------------------------------------------------------------------------------

const preflight = ref<RestackPreflight | undefined>(undefined);
let previewToken = 0;

async function loadPreflight(branch: string): Promise<void> {
  const token = ++previewToken;
  const result = await props.stack.previewRestack(branch);
  if (token === previewToken) preflight.value = result;
}

watch(
  () => props.target,
  (target) => {
    if (target === undefined || target.mode !== 'restack') {
      preflight.value = undefined;
      return;
    }
    void loadPreflight(target.branch);
  },
  { immediate: true },
);

function blockerText(pf: RestackPreflight): string {
  const b = pf.blockers[0];
  if (!b) return '';
  switch (b.kind) {
    case 'inProgressOperation':
      return t('git.dialog.stack.blocker.inProgress.error');
    case 'notStacked':
      return t('git.dialog.stack.blocker.notStacked.error', { branch: b.branch });
    case 'cycle':
      return t('git.dialog.stack.blocker.cycle.error', { branches: b.branches.join(' → ') });
    case 'parentMissing':
      return t('git.dialog.stack.blocker.parentMissing.error', {
        branch: b.branch,
        parent: b.parent,
      });
    case 'checkedOutElsewhere':
      return t('git.dialog.stack.blocker.checkedOut.error', {
        branch: b.branch,
        path: b.worktreePath,
      });
    case 'dirtyWorktree':
      return t('git.dialog.stack.blocker.dirty.error');
  }
}

async function submitRestack(): Promise<void> {
  const target = props.target;
  if (target === undefined || props.stack.restacking.value) return;
  await props.ops.runRestack(target.branch);
  // A conflict/cancellation leaves real state to look at (G5's own ConflictBanner) — re-fetch the
  // preflight rather than closing, so the dialog's own plan reflects what is actually left.
  if (props.target !== undefined) await loadPreflight(target.branch);
}

function cancelRestack(): void {
  void props.ops.cancelRestack();
}

function closeDialog(): void {
  emit('close');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && closeDialog()">
    <DialogContent
      :aria-describedby="undefined"
      size="md"
      :dirty="target?.mode === 'setParent' && selectedParent !== ''"
    >
      <DialogHeader>
        <DialogTitle>
          {{
            target?.mode === 'setParent'
              ? t('git.dialog.stack.setParent.title', { branch: target.branch })
              : t('git.dialog.stack.restack.title', { branch: target?.branch ?? '' })
          }}
        </DialogTitle>
      </DialogHeader>
      <DialogBody>
        <template v-if="target?.mode === 'setParent'">
          <Field>
            <FieldLabel :for="parentId">{{ t('git.dialog.stack.parent.label') }}</FieldLabel>
            <div class="flex items-center gap-2">
              <BranchPicker
                :id="parentId"
                v-model="selectedParent"
                class="min-w-0 flex-1"
                testid="stack-parent"
                :branches="parentCandidates"
                :placeholder="t('git.dialog.stack.noParent.option')"
              />
              <Button
                v-if="selectedParent"
                variant="dialog"
                size="kira-lg"
                data-testid="stack-parent-clear"
                @click="selectedParent = ''"
              >
                {{ t('common.action.clear.label') }}
              </Button>
            </div>
          </Field>
        </template>

        <template v-else-if="target?.mode === 'restack' && preflight">
          <Alert v-if="preflight.verdict === 'blocked'" variant="warn">
            <AlertDescription>{{ blockerText(preflight) }}</AlertDescription>
          </Alert>
          <p v-else-if="preflight.verdict === 'noop'">{{ t('git.dialog.stack.upToDate.hint') }}</p>
          <template v-else>
            <p>
              <CatalogText k="git.dialog.stack.plan.body">
                <template #base><Identifier :value="preflight.base" /></template>
              </CatalogText>
            </p>
            <ul class="max-h-50 overflow-y-auto p-1 bg-bg text-kira-md">
              <li v-for="entry in preflight.plan" :key="entry.branch">
                <CatalogText
                  k="git.dialog.stack.planEntry.label"
                  :params="{
                    count: entry.commits,
                    reason:
                      entry.reason === 'stale'
                        ? t('common.stale.status')
                        : t('git.dialog.stack.reasonAncestor.status'),
                    source: entry.baseSource,
                  }"
                >
                  <template #branch><Identifier :value="entry.branch" /></template>
                  <template #parent><Identifier :value="entry.parent" /></template>
                </CatalogText>
              </li>
            </ul>
            <p v-if="preflight.needsForcePush.length > 0" class="text-muted-foreground">
              {{
                t('git.dialog.stack.forcePush.hint', {
                  branches: preflight.needsForcePush.join(', '),
                })
              }}
            </p>
          </template>

          <template v-if="stack.restacking.value">
            <p>{{ t('git.toolbar.restacking.loading') }}</p>
            <ul class="max-h-50 overflow-y-auto p-1 bg-bg text-kira-md">
              <li v-for="(p, i) in stack.progress.value" :key="i">
                {{
                  t('git.dialog.stack.progress.status', {
                    branch: p.branch,
                    index: p.index,
                    total: p.total,
                  })
                }}
              </li>
            </ul>
          </template>
        </template>
      </DialogBody>

      <DialogFooter>
        <template v-if="target?.mode === 'setParent'">
          <Button variant="dialog" size="kira-lg" @click="closeDialog">{{
            t('common.action.cancel.label')
          }}</Button>
          <Button variant="dialog-primary" size="kira-lg" @click="submitSetParent">{{
            t('common.action.save.label')
          }}</Button>
        </template>
        <template v-else-if="target?.mode === 'restack'">
          <template v-if="stack.restacking.value">
            <Button variant="dialog" size="kira-lg" @click="cancelRestack">{{
              t('git.dialog.stack.cancelRestack.label')
            }}</Button>
          </template>
          <template v-else>
            <Button variant="dialog" size="kira-lg" @click="closeDialog">{{
              t('common.action.close.label')
            }}</Button>
            <Button
              variant="dialog-primary"
              size="kira-lg"
              :disabled="
                !preflight || preflight.verdict === 'blocked' || preflight.verdict === 'noop'
              "
              @click="submitRestack"
            >
              {{ t('git.common.restack.label') }}
            </Button>
          </template>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
