<script setup lang="ts">
/**
 * "Repository settings" — the dialog `AppToolbar.vue`'s own gear (`⚙`) opens.
 * Each field here is hand-written, not schema-driven (no loop over `repoSettingKeys()`) — a new
 * `source: 'repo'` leaf needs an explicit field/patch-diff line added here, same as every leaf
 * already present. `kiraSpace.worktree.prepareScript`/`.basePath` are the two `'repo'`
 * leaves this dialog deliberately does NOT surface. The prepare script is read-only in git-ui and
 * set in Kira Space; `WorktreeDialog.vue` only reads it. The dialog adds
 * `kiraSpace.checkout.autoStash`, its own new "Checkout" section.
 *
 * Mirrors `StashDialog.vue`'s own "one instance, `open` prop + `close` emit" convention — the
 * whole per-repo settings surface fits in one dialog the same way stash's create/branch/popConfirm
 * three modes do, so this is a second file in that shape, not a fourth mode grafted onto
 * `StashDialog.vue` itself (a settings dialog and a stash workflow share no state).
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now. Every
 * `KuiSelect` is `NativeSelect` (git-ui has no fancier dropdown primitive — same choice
 * `StackDialog.vue`'s own parent picker makes), each still driven by an explicit
 * `onXChange`-and-cast handler rather than a plain `v-model`, since `NativeSelect`'s own
 * `modelValue` type (`AcceptableValue`) is wider than any one of these settings' own narrow
 * union. The page-size field is `NumberStepperInput` with a manual `Number(...)` cast on
 * `update:model-value` — a component `v-model` has no `.number` auto-cast (Vue only applies it to
 * a native element).
 */
import { SETTINGS } from '@kira/git-core';
import type { RepoSettingsPatch, RepoSettingsSnapshot } from '@kira/git-ipc';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Textarea } from '@theme/components/ui/textarea';
import { type MessageKey, t } from '@theme/i18n';
import NumberStepperInput from '@theme/NumberStepperInput.vue';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, nextTick, reactive, useId, watch } from 'vue';
import type { RepoSettingsState } from '../../state/repoSettings.ts';

const props = defineProps<{
  open: boolean;
  repoSettingsState: RepoSettingsState;
}>();

const emit = defineEmits<(e: 'close') => void>();

type SelectOption = { readonly value: string; readonly label: MessageKey };

/** `RepoSettingsSnapshot`'s own leaves are `readonly` (a wire type is never a mutation target) —
 *  this dialog's draft needs a genuinely mutable copy of the same shape for `v-model` to write
 *  into. */
type RepoSettingsDraft = { -readonly [K in keyof RepoSettingsSnapshot]: RepoSettingsSnapshot[K] };

/** A local, editable draft — reset from the live snapshot every time the dialog opens (the same
 *  "re-read fresh every time it opens" shape `StashDialog.vue`'s own `includeUntrackedDefault`
 *  prop doc comment already follows), so an edit in progress here never fights a
 *  `repoSettings.changed` event arriving mid-edit from somewhere else (App.vue window resize,
 *  another window's own write) — the live snapshot keeps updating underneath; this draft only
 *  catches up with it at open time, not on every tick. */
const draft = reactive<RepoSettingsDraft>({ ...props.repoSettingsState.settings.value });
// The snapshot the draft was copied from: `save()` diffs against it, so a field the user did not
// edit is never written back over a newer value another surface set meanwhile.
let draftBase: RepoSettingsSnapshot = props.repoSettingsState.settings.value;
const saving = useAsyncAction((patch: Partial<RepoSettingsSnapshot>) =>
  props.repoSettingsState.set(patch),
);
const saveError = saving.error;

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    draftBase = props.repoSettingsState.settings.value;
    Object.assign(draft, draftBase);
    saving.reset();
    fireAndForget(props.repoSettingsState.loadProviders(), 'repoSettings.providers');
    void nextTick(form.reset);
  },
);

const baseCandidatesText = computed({
  get: () => draft['kiraSpace.review.baseCandidates'].join('\n'),
  set: (value: string) => {
    draft['kiraSpace.review.baseCandidates'] = value
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
  },
});

const graphScopeOptions: readonly SelectOption[] = [
  { value: 'all', label: 'git.dialog.repoSettings.scopeAll.option' },
  { value: 'head', label: 'git.dialog.repoSettings.scopeHead.option' },
];

const pullStrategyOptions: readonly SelectOption[] = [
  { value: 'auto', label: 'git.pullStrategy.auto.option' },
  { value: 'ff-only', label: 'git.pullStrategy.ffOnly.option' },
  { value: 'merge', label: 'common.merge.label' },
  { value: 'rebase', label: 'common.rebase.label' },
];

const hostProviderOptions = computed(() => [
  { value: 'auto', text: t('git.dialog.repoSettings.hostAuto.option') },
  { value: 'off', text: t('common.off.status') },
  ...props.repoSettingsState.providers.value.map((p) => ({
    value: p.id,
    text: p.label,
  })),
]);

function onGraphScopeChange(value: string): void {
  draft['kiraSpace.graph.scope'] = value as RepoSettingsSnapshot['kiraSpace.graph.scope'];
}

function onPullStrategyChange(value: string): void {
  draft['kiraSpace.pull.strategy'] = value as RepoSettingsSnapshot['kiraSpace.pull.strategy'];
}

function onPageSizeChange(value: string | number): void {
  // An emptied field is NaN, not 0: it fails `pageSizeValid` rather than saving a bogus value.
  draft['kiraSpace.graph.pageSize'] = value === '' ? Number.NaN : Number(value);
}

const pageSizeText = computed(() => {
  const size = draft['kiraSpace.graph.pageSize'];
  return Number.isNaN(size) ? '' : String(size);
});

const pageSizeValid = computed(() => {
  const size = draft['kiraSpace.graph.pageSize'];
  const { minimum, maximum } = SETTINGS['kiraSpace.graph.pageSize'];
  return Number.isInteger(size) && size >= minimum && size <= maximum;
});

const form = useFormErrors({
  pageSize: {
    value: () => draft['kiraSpace.graph.pageSize'],
    error: () =>
      pageSizeValid.value
        ? null
        : t('git.dialog.repoSettings.pageSize.error', {
            min: SETTINGS['kiraSpace.graph.pageSize'].minimum,
            max: SETTINGS['kiraSpace.graph.pageSize'].maximum,
          }),
  },
});

const isDirty = computed(() =>
  (Object.keys(draft) as (keyof RepoSettingsDraft)[]).some((key) => {
    const a = draft[key];
    const b = draftBase[key];
    return Array.isArray(a) ? a.join('\n') !== (b as readonly string[]).join('\n') : a !== b;
  }),
);

const pageSizeId = useId();
const autoStashId = useId();
const showInGraphId = useId();
const includeUntrackedId = useId();
const hostProviderId = useId();
const baseCandidatesId = useId();
const graphScopeItems = computed(() =>
  graphScopeOptions.map((o) => ({ value: o.value, label: t(o.label) })),
);
const pullStrategyItems = computed(() =>
  pullStrategyOptions.map((o) => ({ value: o.value, label: t(o.label) })),
);

function close(): void {
  emit('close');
}

async function save(): Promise<void> {
  if (!pageSizeValid.value) return;
  const current = draftBase;
  const patch: { -readonly [K in keyof RepoSettingsPatch]: RepoSettingsPatch[K] } = {};
  if (draft['kiraSpace.graph.pageSize'] !== current['kiraSpace.graph.pageSize']) {
    patch['kiraSpace.graph.pageSize'] = draft['kiraSpace.graph.pageSize'];
  }
  if (draft['kiraSpace.graph.scope'] !== current['kiraSpace.graph.scope']) {
    patch['kiraSpace.graph.scope'] = draft['kiraSpace.graph.scope'];
  }
  if (draft['kiraSpace.checkout.autoStash'] !== current['kiraSpace.checkout.autoStash']) {
    patch['kiraSpace.checkout.autoStash'] = draft['kiraSpace.checkout.autoStash'];
  }
  if (draft['kiraSpace.stash.showInGraph'] !== current['kiraSpace.stash.showInGraph']) {
    patch['kiraSpace.stash.showInGraph'] = draft['kiraSpace.stash.showInGraph'];
  }
  if (draft['kiraSpace.stash.includeUntracked'] !== current['kiraSpace.stash.includeUntracked']) {
    patch['kiraSpace.stash.includeUntracked'] = draft['kiraSpace.stash.includeUntracked'];
  }
  if (
    draft['kiraSpace.review.baseCandidates'].join('\n') !==
    current['kiraSpace.review.baseCandidates'].join('\n')
  ) {
    patch['kiraSpace.review.baseCandidates'] = [...draft['kiraSpace.review.baseCandidates']];
  }
  if (draft['kiraSpace.pull.strategy'] !== current['kiraSpace.pull.strategy']) {
    patch['kiraSpace.pull.strategy'] = draft['kiraSpace.pull.strategy'];
  }
  if (draft['kiraSpace.gitHost.provider'] !== current['kiraSpace.gitHost.provider']) {
    patch['kiraSpace.gitHost.provider'] = draft['kiraSpace.gitHost.provider'];
  }
  if (Object.keys(patch).length > 0 && !(await saving.run(patch))) return;
  close();
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && close()">
    <DialogContent :aria-describedby="undefined" size="md" :dirty="isDirty">
      <DialogHeader>
        <DialogTitle>{{ t('git.toolbar.repoSettings.tooltip') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <FieldSet>
          <FieldLegend>{{ t('common.graph.label') }}</FieldLegend>
          <Field>
            <FieldLabel :for="pageSizeId">{{
              t('git.dialog.repoSettings.pageSize.label')
            }}</FieldLabel>
            <NumberStepperInput
              :id="pageSizeId"
              :model-value="pageSizeText"
              group-class="w-24"
              :min="SETTINGS['kiraSpace.graph.pageSize'].minimum"
              :max="SETTINGS['kiraSpace.graph.pageSize'].maximum"
              v-bind="form.invalidAttrs('pageSize')"
              @update:model-value="onPageSizeChange"
            />
            <FieldError v-if="form.errorFor('pageSize')" :id="form.errorId('pageSize')">
              {{ form.errorFor('pageSize') }}
            </FieldError>
          </Field>
          <Field>
            <FieldLabel>{{ t('common.field.scope.label') }}</FieldLabel>
            <SecondaryTabs
              variant="choice"
              size="kira-lg"
              :aria-label="t('common.field.scope.label')"
              :model-value="draft['kiraSpace.graph.scope']"
              :items="graphScopeItems"
              @update:model-value="onGraphScopeChange"
            />
          </Field>
        </FieldSet>

        <FieldSet>
          <FieldLegend>{{ t('git.dialog.repoSettings.checkout.label') }}</FieldLegend>
          <Field orientation="horizontal">
            <Checkbox :id="autoStashId" v-model="draft['kiraSpace.checkout.autoStash']" />
            <FieldLabel :for="autoStashId">
              {{ t('git.dialog.repoSettings.autoStash.label') }}
            </FieldLabel>
          </Field>
          <FieldDescription>
            {{ t('git.dialog.repoSettings.autoStash.hint') }}
          </FieldDescription>
        </FieldSet>

        <FieldSet>
          <FieldLegend>{{ t('git.toolbar.stash.label') }}</FieldLegend>
          <Field orientation="horizontal">
            <Checkbox :id="showInGraphId" v-model="draft['kiraSpace.stash.showInGraph']" />
            <FieldLabel :for="showInGraphId">{{
              t('git.dialog.repoSettings.showInGraph.label')
            }}</FieldLabel>
          </Field>
          <Field orientation="horizontal">
            <Checkbox
              :id="includeUntrackedId"
              v-model="draft['kiraSpace.stash.includeUntracked']"
            />
            <FieldLabel :for="includeUntrackedId">
              {{ t('git.dialog.repoSettings.includeUntracked.label') }}
            </FieldLabel>
          </Field>
        </FieldSet>

        <FieldSet>
          <FieldLegend>{{ t('git.dialog.repoSettings.review.label') }}</FieldLegend>
          <Field>
            <FieldLabel :for="baseCandidatesId">
              {{ t('git.dialog.repoSettings.baseCandidates.label') }}
            </FieldLabel>
            <Textarea :id="baseCandidatesId" v-model="baseCandidatesText" class="font-data" />
          </Field>
        </FieldSet>

        <FieldSet>
          <FieldLegend>{{ t('common.gitHost.label') }}</FieldLegend>
          <Field>
            <FieldLabel :for="hostProviderId">{{
              t('git.dialog.repoSettings.hostProvider.label')
            }}</FieldLabel>
            <NativeSelect
              :id="hostProviderId"
              :model-value="draft['kiraSpace.gitHost.provider']"
              variant="bordered"
              class="w-full"
              @update:model-value="(v) => (draft['kiraSpace.gitHost.provider'] = v as string)"
            >
              <option v-for="opt in hostProviderOptions" :key="opt.value" :value="opt.value">
                {{ opt.text }}
              </option>
            </NativeSelect>
          </Field>
        </FieldSet>

        <FieldSet>
          <FieldLegend>{{ t('common.action.pull.label') }}</FieldLegend>
          <Field>
            <FieldLabel>{{ t('git.dialog.repoSettings.strategy.label') }}</FieldLabel>
            <SecondaryTabs
              variant="choice"
              size="kira-lg"
              :aria-label="t('git.dialog.repoSettings.strategy.label')"
              :model-value="draft['kiraSpace.pull.strategy']"
              :items="pullStrategyItems"
              @update:model-value="onPullStrategyChange"
            />
          </Field>
        </FieldSet>

        <Alert v-if="saveError" variant="error">
          <AlertDescription>{{
            t('git.dialog.repoSettings.saveError.error', { error: saveError })
          }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button variant="dialog" size="kira-lg" @click="close">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button variant="dialog-primary" size="kira-lg" :disabled="!pageSizeValid" @click="save">
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
