<script setup lang="ts">
/**
 * `stashAndCarry` route, pull's side — the confirm step
 * for `OpsState.runPull`'s own `dirtyNonFastForward` blocker, mirroring `CheckoutDialog.vue`'s
 * "a hazard pre-flight sets a pending ref, this dialog renders while it is set, resolving it
 * settles the promise the caller is awaiting" shape exactly, one route simpler: pull's own
 * `PullPreflight.routes` only ever offers `"stashAndCarry"` (there is no pull analogue of
 * "discard" — a merge/rebase cannot be told to throw the local changes away), so
 * `resolvePullDialog` takes a plain `boolean` rather than a tagged route.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now — this file still
 * only supplies its own body/footer content.
 */
import type { PullPreflight } from '@kira/git-ipc';
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

const pending = computed<PullPreflight | undefined>(() => props.ops.pendingPull.value);
const active = computed(() => pending.value !== undefined);

function cancel(): void {
  props.ops.resolvePullDialog(false);
}

function stashAndCarry(): void {
  props.ops.resolvePullDialog(true);
}
</script>

<template>
  <Dialog v-if="pending" :open="active" @update:open="(v) => !v && cancel()">
    <DialogContent size="sm">
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.pull.title') }}</DialogTitle>
        <DialogDescription>
          <CatalogText k="git.dialog.pull.body">
            <template #strategy><Identifier :value="pending.strategy" /></template>
          </CatalogText>
        </DialogDescription>
      </DialogHeader>
      <DialogBody>
        <Alert variant="warn">
          <AlertDescription>
            {{ t('git.dialog.pull.stash.hint') }}
          </AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="pull-stash-and-carry"
          @click="stashAndCarry"
        >
          {{ t('git.dialog.pull.stashAndPull.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
