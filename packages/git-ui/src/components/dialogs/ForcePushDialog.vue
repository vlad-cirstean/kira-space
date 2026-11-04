<script setup lang="ts">
/**
 * The confirm step for `OpsState.runForcePush` — mirrors
 * `CheckoutDialog.vue`/`RevertDialog.vue`'s own "a hazard pre-flight sets a pending ref, this
 * dialog renders while it is set, resolving it settles the promise the caller is awaiting"
 * shape, `pendingForcePush`/`resolveForcePushDialog` playing the same roles as
 * `pendingCheckout`/`resolveCheckoutDialog`.
 *
 * Two escalating confirmations:
 *  - The default path is the lease (`--force-with-lease --force-if-includes`) — safe against
 *    anything pushed since `remoteTip` was read, whether or not this session ever fetched it.
 *    On a protected branch (`preflight.protectedBy` non-null) it additionally requires typing
 *    the branch name back, the friction for the common protected case.
 *  - Plain `--force` bypasses the lease entirely and is behind its own, separately-worded
 *    confirmation (a `Disclosure`, collapsed by default) — needed on top of, not
 *    instead of, the typed branch name when the branch is also protected.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still
 * only supplies its own body/footer content. The plain-`--force` confirm button stays inside the
 * `Disclosure` body (in the default slot) rather than moving to `DialogFooter`, since it belongs
 * beside its own disclosure and acknowledgement checkbox, not beside the lease/Cancel pair.
 */
import { shortSha } from '@kira/git-core';
import Disclosure from '@theme/components/Disclosure.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, ref, useId, watch } from 'vue';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{ ops: OpsState }>();

const pending = computed(() => props.ops.pendingForcePush.value);
const active = computed(() => pending.value !== undefined);
// P131 Part 1 §6.2: biome's noLabelWithoutControl can't see through a nested shadcn `Input`
// component to the native `<input>` it renders, unlike the raw `<input>` this label used to wrap
// directly -- an explicit for/id pair keeps the same association, verifiably.
const branchNameId = useId();
const plainAckId = useId();

const typedBranch = ref('');
const understandPlain = ref(false);

watch(pending, () => {
  typedBranch.value = '';
  understandPlain.value = false;
});

const protectedBy = computed(() => pending.value?.preflight.protectedBy ?? null);
const isProtected = computed(() => protectedBy.value !== null);
// The backend's ConfirmToken check (gitsession.RunRemote) gates on the
// resolved UPSTREAM branch name, not pending.branch's LOCAL one — a stacked/renamed branch can
// track a differently-named protected upstream. Comparing/sending the local name here made the
// gate impossible to ever satisfy in that case.
const resolvedBranch = computed(
  () => pending.value?.preflight.resolvedBranch ?? pending.value?.branch ?? '',
);
const confirmToken = computed(() => (isProtected.value ? typedBranch.value : undefined));
const branchNameMatches = computed(
  () => !isProtected.value || typedBranch.value === resolvedBranch.value,
);
const canConfirmLease = computed(() => branchNameMatches.value);
const canConfirmPlain = computed(() => branchNameMatches.value && understandPlain.value);

function cancel(): void {
  props.ops.resolveForcePushDialog(null);
}

function confirmLease(): void {
  if (!canConfirmLease.value) return;
  props.ops.resolveForcePushDialog({ plain: false, confirmToken: confirmToken.value });
}

function confirmPlain(): void {
  if (!canConfirmPlain.value) return;
  props.ops.resolveForcePushDialog({ plain: true, confirmToken: confirmToken.value });
}
</script>

<template>
  <Dialog v-if="pending" :open="active" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" destructive :dirty="typedBranch !== '' || understandPlain">
      <DialogHeader>
        <DialogTitle>{{
          t('git.dialog.forcePushConfirm.title', { branch: pending.branch, remote: pending.remote })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <DialogDescription>
          <CatalogText
            :k="
              pending.preflight.remoteTip === null
                ? 'git.dialog.forcePushConfirm.bodyNew.body'
                : 'git.dialog.forcePushConfirm.body'
            "
          >
            <template #ref><Identifier :value="`${pending.remote}/${resolvedBranch}`" /></template>
            <template #sha
              ><Identifier :value="pending.preflight.remoteTip ?? ''" :short="shortSha" /></template
            >
          </CatalogText>
          <template v-if="pending.preflight.behind > 0">
            {{ t('git.dialog.forcePushConfirm.ahead.hint', { count: pending.preflight.behind }) }}
          </template>
        </DialogDescription>

        <Alert v-if="protectedBy" variant="warn">
          <AlertDescription>
            <CatalogText k="git.dialog.forcePushConfirm.protected.hint">
              <template #branch><Identifier :value="resolvedBranch" /></template>
              <template #pattern><Identifier :value="protectedBy" /></template>
            </CatalogText>
          </AlertDescription>
        </Alert>
        <Field v-if="protectedBy">
          <FieldLabel :for="branchNameId">{{ t('git.common.branchName.label') }}</FieldLabel>
          <Input
            :id="branchNameId"
            v-model="typedBranch"
            type="text"
            :placeholder="resolvedBranch"
            size="kira-lg"
            class="w-full"
            data-testid="force-push-confirm-branch"
          />
        </Field>

        <Disclosure class="pt-2" trigger-class="text-muted-foreground">
          <template #label
            ><CatalogText k="git.dialog.forcePushConfirm.plain.label">
              <template #flag><code class="font-data">--force</code></template>
            </CatalogText></template
          >
          <div class="mt-3">
            <Alert variant="warn">
              <AlertDescription>
                {{ t('git.dialog.forcePushConfirm.plain.hint') }}
              </AlertDescription>
            </Alert>
          </div>
          <Field orientation="horizontal" class="mt-3">
            <Checkbox
              :id="plainAckId"
              v-model="understandPlain"
              data-testid="force-push-plain-ack"
            />
            <FieldLabel :for="plainAckId">
              {{ t('git.dialog.forcePushConfirm.plainAck.label') }}
            </FieldLabel>
          </Field>
          <div class="mt-3 flex justify-end">
            <Button
              variant="dialog-danger"
              size="kira-lg"
              :disabled="!canConfirmPlain"
              data-testid="force-push-confirm-plain"
              @click="confirmPlain"
            >
              {{ t('git.dialog.forcePushConfirm.plainConfirm.confirm') }}
            </Button>
          </div>
        </Disclosure>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-dialog-dismiss @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canConfirmLease"
          data-testid="force-push-confirm-lease"
          @click="confirmLease"
        >
          {{ t('git.dialog.forcePushConfirm.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
