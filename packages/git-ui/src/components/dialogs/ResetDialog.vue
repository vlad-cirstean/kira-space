<script setup lang="ts">
/**
 * The confirm step — and, unlike every other dialog in this directory,
 * the confirm step for EVERY reset, not only a hazardous one (the row menu's
 * "Reset to this commit…" is the one entry point, and choosing a mode IS the confirmation here —
 * there is no "clean, skip the dialog" fast path to mirror `CheckoutDialog`/
 * `RevertDialog`'s own).
 *
 * Switching the mode radio never re-requests `preflight.reset` — `OpsState.previewResetMode`
 * recomputes `destroys`/`requiresTypedConfirmation`/`routes`/`verdict` client-side via `core`'s own
 * `classifyReset`, holding `leaving`/`gaining`/`dirty` fixed (they do not depend on `mode` at all).
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now, the mode
 * fieldset's radios are `RadioGroup`/`RadioGroupCard` (still an imperative `selectMode` call via
 * `@update:model-value`, not a plain `v-model`), the stash-first checkbox is `Checkbox`+`Label`,
 * and the confirm-token field is `Input`.
 */
import { shortSha } from '@kira/git-core';
import type { ResetMode } from '@kira/git-ipc';
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
import {
  Field,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { RadioGroup, RadioGroupCard } from '@theme/components/ui/radio-group';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, ref, useId, watch } from 'vue';
import { shaLabel } from '../../lib/text.ts';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{ ops: OpsState }>();

const preflight = computed(() => props.ops.pendingReset.value);
const active = computed(() => preflight.value !== undefined);

const typedToken = ref('');
const stashFirst = ref(false);
const tokenId = useId();
const stashFirstId = useId();

// A genuinely new target resets the typed confirmation and the stash-first choice; a mode-radio
// change (`previewResetMode`'s own re-classify, same target) must not, or picking a mode would
// immediately blank out what was already typed.
watch(
  () => preflight.value?.target,
  () => {
    typedToken.value = '';
    stashFirst.value = false;
  },
);

const mode = computed<ResetMode>(() => preflight.value?.mode ?? 'mixed');

function selectMode(next: ResetMode): void {
  props.ops.previewResetMode(next);
}

/** `HEAD` is a Git ref name, not prose. */
const refName = computed(() => preflight.value?.branch ?? 'HEAD');
const shortTarget = computed(() => shaLabel(preflight.value?.target ?? ''));
const destroys = computed(() => preflight.value?.destroys ?? []);
const requiresToken = computed(() => preflight.value?.requiresTypedConfirmation ?? false);
const canStashFirst = computed(() => (preflight.value?.routes ?? []).includes('stashFirst'));
const tokenMatches = computed(
  () => !requiresToken.value || typedToken.value.trim() === shortTarget.value,
);
const canConfirm = computed(() => stashFirst.value || tokenMatches.value);
const isHardDestructive = computed(() => mode.value === 'hard' && destroys.value.length > 0);

function cancel(): void {
  props.ops.resolveResetDialog(null);
}

function confirm(): void {
  if (!canConfirm.value) return;
  props.ops.resolveResetDialog({
    mode: mode.value,
    stashFirst: stashFirst.value,
    token: requiresToken.value && !stashFirst.value ? typedToken.value.trim() : undefined,
  });
}
</script>

<template>
  <Dialog :open="active" @update:open="(v) => !v && cancel()">
    <DialogContent size="md" destructive :dirty="typedToken !== ''">
      <DialogHeader>
        <DialogTitle>
          <CatalogText k="git.dialog.reset.title">
            <template #ref><Identifier :value="refName" /></template>
            <template #sha><Identifier :value="shortTarget" /></template>
          </CatalogText>
          <span v-if="preflight?.targetSubject" class="ml-1 font-normal text-muted-foreground">{{
            preflight.targetSubject
          }}</span>
        </DialogTitle>
      </DialogHeader>
      <DialogBody>
        <FieldDescription v-if="preflight && !preflight.branch">
          {{ t('git.dialog.reset.noBranch.hint') }}
        </FieldDescription>

        <DialogDescription v-if="preflight?.leaving === 0 && preflight.gaining === 0">
          {{ t('git.dialog.reset.alreadyHere.body') }}
        </DialogDescription>
        <template v-else-if="preflight?.gaining === 0">
          <DialogDescription>
            <CatalogText k="git.dialog.reset.leaving.body" :params="{ count: preflight.leaving }">
              <template #ref><Identifier :value="refName" /></template>
            </CatalogText>
          </DialogDescription>
          <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
            <li v-for="c in preflight.leavingCommits" :key="c.sha">
              <Identifier :value="c.sha" :short="shortSha" /> {{ c.subject }}
            </li>
          </ul>
          <p v-if="preflight.leavingTruncated" class="text-muted-foreground italic">{{
            t('git.dialog.reset.more.status')
          }}</p>
        </template>
        <DialogDescription v-else-if="preflight">
          {{
            t('git.dialog.reset.diverged.body', {
              leaving: preflight.leaving,
              gaining: preflight.gaining,
            })
          }}
        </DialogDescription>

        <FieldSet>
          <FieldLegend>{{ t('common.mode.label') }}</FieldLegend>
          <RadioGroup
            class="gap-1.5"
            :model-value="mode"
            @update:model-value="(v) => selectMode(v as ResetMode)"
          >
            <RadioGroupCard value="soft" data-testid="reset-mode-soft">
              <template #title>{{ t('git.dialog.reset.soft.label') }}</template>
              <template #description>
                {{ t('git.dialog.reset.soft.description') }}
              </template>
            </RadioGroupCard>
            <RadioGroupCard value="mixed" data-testid="reset-mode-mixed">
              <template #title>{{ t('git.dialog.reset.mixed.label') }}</template>
              <template #description>
                {{ t('git.dialog.reset.mixed.description') }}
              </template>
            </RadioGroupCard>
            <RadioGroupCard value="hard" data-testid="reset-mode-hard">
              <template #title>{{ t('git.dialog.reset.hard.label') }}</template>
              <template #description>
                {{ t('git.dialog.reset.hard.description') }}
                <strong class="font-medium">{{ t('git.dialog.reset.hard.hint') }}</strong>
              </template>
            </RadioGroupCard>
          </RadioGroup>
        </FieldSet>

        <template v-if="mode === 'hard' && destroys.length > 0">
          <Alert variant="warn">
            <AlertDescription>
              <p>{{ t('git.dialog.reset.discard.body') }}</p>
              <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                <li v-for="path in destroys" :key="path"><Identifier :value="path" /></li>
              </ul>
            </AlertDescription>
          </Alert>
          <FieldDescription class="italic">
            {{ t('git.dialog.reset.untracked.hint') }}
          </FieldDescription>

          <Field v-if="canStashFirst" orientation="horizontal">
            <Checkbox :id="stashFirstId" v-model="stashFirst" />
            <FieldLabel :for="stashFirstId">{{
              t('git.dialog.reset.stashFirst.label')
            }}</FieldLabel>
          </Field>

          <Field v-if="!stashFirst">
            <FieldLabel :for="tokenId">
              <CatalogText k="git.dialog.reset.token.label">
                <template #token><Identifier :value="shortTarget" /></template>
              </CatalogText>
            </FieldLabel>
            <Input
              :id="tokenId"
              v-model="typedToken"
              type="text"
              size="kira-lg"
              class="w-full"
              data-testid="reset-confirm-token"
            />
          </Field>
        </template>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-dialog-dismiss @click="cancel">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          v-if="isHardDestructive && canStashFirst"
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canConfirm"
          data-testid="reset-confirm"
          @click="confirm"
        >
          {{
            stashFirst
              ? t('git.dialog.reset.stashAndReset.confirm')
              : t('git.dialog.reset.discardAndReset.confirm')
          }}
        </Button>
        <Button
          v-else
          :variant="isHardDestructive ? 'dialog-danger' : 'dialog-primary'"
          size="kira-lg"
          :disabled="!canConfirm"
          data-testid="reset-confirm"
          @click="confirm"
        >
          {{ t('common.action.reset.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
