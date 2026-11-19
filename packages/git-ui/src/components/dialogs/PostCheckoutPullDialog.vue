<script setup lang="ts">
/**
 * "when checking out a branch that advanced, ask if I want to pull it too" —
 * `OpsState.runCheckout`'s own post-switch confirm step (`#maybePromptPostCheckoutPull`), mirroring
 * `PullDialog.vue`'s "a pending ref set by the state class, this dialog renders while it is set,
 * resolving it settles the promise the caller is awaiting" shape, one route simpler: there is only
 * ever Pull now / Not now, never a hazard to describe.
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now.
 */
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

const pending = computed(() => props.ops.pendingPostCheckoutPull.value);
const active = computed(() => pending.value !== undefined);

function notNow(): void {
  props.ops.resolvePostCheckoutPullDialog(false);
}

function pullNow(): void {
  props.ops.resolvePostCheckoutPullDialog(true);
}
</script>

<template>
  <Dialog v-if="pending" :open="active" @update:open="(v) => !v && notNow()">
    <DialogContent size="sm">
      <DialogHeader>
        <DialogTitle>{{ t('git.dialog.postCheckoutPullConfirm.title') }}</DialogTitle>
        <DialogDescription>
          <CatalogText
            k="git.dialog.postCheckoutPullConfirm.body"
            :params="{ count: pending.behind }"
          >
            <template #branch><strong>{{ pending.branch }}</strong></template>
            <template #upstream><Identifier :value="pending.upstreamShortName" /></template>
          </CatalogText>
        </DialogDescription>
      </DialogHeader>
      <DialogBody> </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="post-checkout-pull-not-now"
          @click="notNow"
        >
          {{ t('common.action.notNow.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="post-checkout-pull-now"
          @click="pullNow"
        >
          {{ t('git.dialog.postCheckoutPullConfirm.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
