<script setup lang="ts">
/**
 * The confirm step, "only when destructive or when pre-flight found a hazard".
 * `OpsState.runCheckout` only ever awaits this dialog for a `"blocked"` verdict (`clean` and
 * `cleanCarry` proceed with no prompt — see that method's own doc comment) — so this file has
 * nothing to render for either of those and never receives one.
 *
 * A bare `blockedByTracked` (no untracked block alongside it) also offers
 * `"stashAndCarry"` in `preflight.routes` — the "Stash changes and check out" button below,
 * calling straight back into `resolveCheckoutDialog` exactly like Discard/Cancel do, since
 * `runCheckout`'s own inline handling of that route (not a second dialog) is the rest of the
 * confirm step.
 *
 * `OpsState.runCheckout` now resolves a blocked verdict AUTOMATICALLY — no dialog at all
 * — whenever `preflight.routes` offers `"detachHere"` (always) or `"autoStash"` (when
 * `kiraSpace.checkout.autoStash` is on), re-issuing the same op with `mode: 'detach'` and/or
 * `autoStash: true` instead. This dialog now only ever opens for what is LEFT after that: an
 * `inProgressOperation` blocker (routing is never offered for one), or a blocked verdict with
 * neither route available (a tracked-only or untracked-only block when `autoStash` itself is off,
 * or `StashAvailable` were ever false). Its own Discard/Stash-and-carry/Cancel buttons, and the
 * `#stashAndCarry` route they call into, are completely UNCHANGED — this file gets called
 * less often than before.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now (reka-ui's own
 * `FocusScope`/Escape/outside-click replace `KuiDialog`'s hand-rolled equivalents) — this file
 * still only supplies its own body/footer content.
 */
import type { CheckoutPreflight } from '@kira/git-ipc';
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
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{ ops: OpsState }>();

const preflight = computed<CheckoutPreflight | undefined>(() => props.ops.pendingCheckout.value);
const active = computed(() => preflight.value !== undefined);

/** W2's own asserted order — inProgress first (it makes every other remedy moot), then worktree,
 *  then untracked, then tracked — so the *headline* blocker is always the one that actually
 *  explains why nothing else here would have helped either. */
const headline = computed(() => {
  const blockers = preflight.value?.blockers ?? [];
  return (
    blockers.find((b) => b.kind === 'inProgressOperation') ??
    blockers.find((b) => b.kind === 'worktreeConflict') ??
    blockers.find((b) => b.kind === 'blockedByUntracked') ??
    blockers.find((b) => b.kind === 'blockedByTracked')
  );
});

const trackedBlocker = computed(() =>
  preflight.value?.blockers.find((b) => b.kind === 'blockedByTracked'),
);
const canDiscard = computed(() => preflight.value?.routes.includes('discard') ?? false);
const canStashAndCarry = computed(() => preflight.value?.routes.includes('stashAndCarry') ?? false);

function cancel(): void {
  props.ops.resolveCheckoutDialog(null);
}

function discard(): void {
  props.ops.resolveCheckoutDialog({ kind: 'discard' });
}

function stashAndCarry(): void {
  props.ops.resolveCheckoutDialog({ kind: 'stashAndCarry' });
}
</script>

<template>
  <Dialog :open="active" @update:open="(v) => !v && cancel()">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{
          t('git.dialog.checkout.title', { name: preflight?.target.name ?? '' })
        }}</DialogTitle>
        <DialogDescription v-if="headline?.kind === 'inProgressOperation'">{{
          t('git.common.operationInProgress.body')
        }}</DialogDescription>
        <DialogDescription v-else-if="headline?.kind === 'worktreeConflict'">
          <CatalogText k="git.dialog.checkout.worktreeConflict.body">
            <template #branch><Identifier :value="headline.branch" /></template>
            <template #path><Identifier :value="headline.worktreePath" /></template>
          </CatalogText>
        </DialogDescription>
        <DialogDescription v-else-if="headline?.kind === 'blockedByUntracked'">{{
          t('git.dialog.checkout.untracked.body')
        }}</DialogDescription>
        <DialogDescription v-else-if="trackedBlocker">{{
          t('git.dialog.checkout.localChanges.body')
        }}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <template v-if="headline?.kind === 'blockedByUntracked'">
          <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
            <li v-for="path in headline.paths" :key="path"><Identifier :value="path" /></li>
          </ul>
          <p>{{ t('git.dialog.checkout.untracked.hint') }}</p>
        </template>

        <template v-else-if="headline?.kind === 'blockedByTracked' && trackedBlocker">
          <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
            <li v-for="path in trackedBlocker.paths" :key="path"><Identifier :value="path" /></li>
          </ul>
          <Alert v-if="canDiscard" variant="warn">
            <AlertDescription>
              {{ t('git.dialog.checkout.discard.hint') }}
              <template v-if="canStashAndCarry">{{
                t('git.dialog.checkout.stashInstead.hint')
              }}</template>
            </AlertDescription>
          </Alert>
        </template>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          v-if="canStashAndCarry"
          variant="dialog-primary"
          size="kira-lg"
          @click="stashAndCarry"
        >
          {{ t('git.dialog.checkout.stashAndCheckOut.confirm') }}
        </Button>
        <Button v-if="canDiscard" variant="dialog-danger" size="kira-lg" @click="discard">
          {{ t('git.dialog.checkout.discardAndCheckOut.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
