<script setup lang="ts">
/**
 * The confirm step — a near-sibling of `RevertDialog.vue`, opened by
 * `OpsState.runCherryPick` whenever the pre-flight is not a clean, non-merge, not-already-applied
 * single pick (that method's own doc comment). One addition revert has no need of: the non-
 * blocking `alreadyApplied` advisory (probe 6) — it never sets a blocker or changes `verdict`, so
 * it is rendered as its own note rather than folded into the blocker list below.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still
 * only supplies its own body/footer content.
 */
import { shortSha } from '@kira/git-core';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { RadioGroup, RadioGroupCard } from '@theme/components/ui/radio-group';
import { t } from '@theme/i18n';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, ref, watch } from 'vue';
import type { OpsState } from '../../state/ops.ts';
import PreflightPrediction from './PreflightPrediction.vue';

const props = defineProps<{ ops: OpsState }>();

const preflight = computed(() => props.ops.pendingCherryPick.value);
const active = computed(() => preflight.value !== undefined);

const selectedMainline = ref<number | undefined>(undefined);
const noCommit = ref(false);

// A genuinely new cherry-pick request (a different sha) resets the choice; a re-preflight for
// the *same* sha — `previewCherryPickMainline`'s own refresh, triggered by the watch just below
// — must not, or picking a mainline would immediately un-pick itself.
watch(
  () => preflight.value?.sha,
  () => {
    selectedMainline.value = undefined;
    noCommit.value = false;
  },
);

watch(selectedMainline, (mainline) => {
  if (mainline !== undefined) void props.ops.previewCherryPickMainline(mainline);
});

const needsMainline = computed(() => (preflight.value?.mainlineRequired.length ?? 0) > 0);
// `classifyCherryPick` files `mainlineRequired` as a `CherryPickBlocker` alongside the real,
// nothing-else-to-do refusals (the set-intersection blockers, plus the in-progress gate) —
// but unlike those, it has its own dedicated, non-blocking resolution right here (the radio group
// below), the same way `RevertDialog.vue`'s own `needsMainline` never waits on a generic blocker
// list at all. Filtered out here so picking a mainline stays reachable on its own: left in,
// `hasBlocker` would be permanently true the moment `needsMainline` is, the radio group would
// never render (`v-if="!hasBlocker && needsMainline"` below), and a merge commit's cherry-pick
// could never be confirmed.
const blockers = computed(
  () => preflight.value?.blockers.filter((b) => b.kind !== 'mainlineRequired') ?? [],
);
// Every remaining blocker is a real git refusal — unlike `RevertPreflight`'s advisory
// `dirtyWorktree`, there is no route past any of these short of changing the working tree or
// resolving the other operation first, so Confirm stays disabled while any is present.
const hasBlocker = computed(() => blockers.value.length > 0);
const canConfirm = computed(
  () => !hasBlocker.value && (!needsMainline.value || selectedMainline.value !== undefined),
);

function cancel(): void {
  props.ops.resolveCherryPickDialog(null);
}

function confirm(): void {
  if (!canConfirm.value) return;
  props.ops.resolveCherryPickDialog({
    mainline: selectedMainline.value,
    noCommit: noCommit.value,
  });
}
</script>

<template>
  <Dialog :open="active" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" destructive>
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.cherryPick.title') }}</DialogTitle>
        <DialogDescription>
          {{ t('git.dialog.cherryPick.description') }}
        </DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Alert v-if="preflight?.detachedHead" variant="warn">
          <AlertDescription>
            {{ t('git.dialog.cherryPick.detachedHead.hint') }}
          </AlertDescription>
        </Alert>

        <Alert v-if="preflight?.alreadyApplied" variant="warn">
          <AlertDescription>
            {{ t('git.dialog.cherryPick.alreadyApplied.hint') }}
          </AlertDescription>
        </Alert>

        <template v-if="hasBlocker">
          <Alert v-for="(blocker, i) in blockers" :key="i" variant="warn"
            ><AlertDescription>
              <template v-if="blocker.kind === 'inProgressOperation'">
                <p>{{ t('git.common.operationInProgress.body') }}</p>
              </template>
              <template v-else-if="blocker.kind === 'stagedChanges'">
                <p>{{ t('git.dialog.cherryPick.stagedChanges.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in blocker.paths" :key="path"><Identifier :value="path" /></li>
                </ul>
              </template>
              <template v-else-if="blocker.kind === 'localChangesWouldBeOverwritten'">
                <p>{{ t('git.dialog.cherryPick.localChanges.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in blocker.paths" :key="path"><Identifier :value="path" /></li>
                </ul>
              </template>
              <template v-else-if="blocker.kind === 'untrackedWouldBeOverwritten'">
                <p>{{ t('git.dialog.cherryPick.untracked.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in blocker.paths" :key="path"><Identifier :value="path" /></li>
                </ul>
              </template>
            </AlertDescription></Alert
          >
        </template>

        <template v-if="!hasBlocker && needsMainline">
          <p>{{ t('git.dialog.cherryPick.mainline.body') }}</p>
          <RadioGroup v-model="selectedMainline" class="gap-1.5">
            <RadioGroupCard
              v-for="entry in preflight?.mainlineRequired"
              :key="entry.parentNumber"
              :value="entry.parentNumber"
              :data-testid="`cherry-pick-mainline-${entry.parentNumber}`"
            >
              <template #title>
                {{ t('git.common.mergeParent.option', { number: entry.parentNumber }) }}
              </template>
              <template #description>
                <Identifier :value="entry.sha" :short="shortSha" /> {{ entry.subject }}
              </template>
            </RadioGroupCard>
          </RadioGroup>
        </template>

        <template v-if="!hasBlocker && (!needsMainline || selectedMainline !== undefined)">
          <PreflightPrediction
            v-if="preflight"
            :prediction="preflight.prediction"
            v-model:no-commit="noCommit"
          />
        </template>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-dialog-dismiss @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canConfirm"
          data-testid="cherry-pick-confirm"
          @click="confirm"
        >
          {{ t('git.dialog.cherryPick.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
