<script setup lang="ts">
/**
 * The normative tree names exactly one `StashDialog.vue`, unlike
 * `BranchDialog.vue`/`TagDialog.vue`/`RevertDialog.vue`'s one-dialog-per-file precedent (all three
 * of its "collect input" moments live in this one file rather than being split further to match
 * that precedent):
 *
 * - **create** — `runStashPush`'s own confirm step (no pre-flight endpoint exists for it).
 * - **branch** — name entry plus `previewStashBranch`'s live pre-flight, re-run on every keystroke
 *   exactly like `RevertDialog.vue`'s own mainline picker re-runs `previewRevertMainline`.
 * - **popConfirm** — the shared apply/pop confirmation, driven directly by
 *   `ops.pendingStashPop` with no props of its own, exactly like `RevertDialog.vue`/
 *   `CheckoutDialog.vue` are driven by their own `ops.pending*` fields.
 *
 * At most one mode is ever active — `mode` below picks in that order (create/branch never overlap
 * `popConfirm` in practice, since `previewStashBranch` never opens `ops.pendingStashPop`, but the
 * order still matters if a caller opened `createOpen`/`branchTarget` while a pop confirmation from
 * an unrelated row happened to already be pending).
 *
 * The modal shell is shadcn's `Dialog`/`DialogContent` now, `title` feeds
 * `DialogTitle`'s default slot, and the four modes' own controls are Input/Checkbox/SecondaryTabs —
 * `mode`/`onClose`'s own dispatch is otherwise unchanged.
 */
import { validateRefName } from '@kira/git-core';
import type { StashBranchPreflight, StashEntry } from '@kira/git-ipc';
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
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { useFormErrors } from '@workbench/behaviour/formErrors';
import Identifier from '@workbench/components/Identifier.vue';
import { computed, nextTick, ref, useId, watch } from 'vue';
import { stashLabel } from '../../lib/stashListModel.ts';
import { renderText } from '../../lib/text.ts';
import type { OpsState } from '../../state/ops.ts';

const props = defineProps<{
  ops: OpsState;
  /** Toggled by `AppToolbar.vue`'s "Stash" button, via `App.vue`. */
  createOpen: boolean;
  /** `kiraSpace.stash.includeUntracked`'s current value — the create form's own default,
   *  re-read fresh every time the dialog opens (a setting change mid-session should be seen the
   *  next time this opens, not only after a reload). */
  includeUntrackedDefault: boolean;
  /** Set by `StashList.vue`'s "Create branch from stash…" row action, via `RefsPopover.vue` →
   *  `App.vue`; `undefined` when branch mode is not open. */
  branchTarget: StashEntry | undefined;
  /** G28 D13: toggled by the global-stash section's own header button and the palette's
   *  `saveGlobalStash` action, via `App.vue` — opens save mode with NO pre-selected source
   *  (defaults to "this working tree"). */
  saveOpen: boolean;
  /** Set by a STACK row's own "Save to global stash…" action — opens save mode
   *  pre-selected to promote THIS entry; `undefined` when that row action was not
   *  the trigger (the header button/palette route sets `saveOpen` instead, leaving this
   *  `undefined`, and the source radio then defaults to "this working tree"). */
  saveSourceEntry: StashEntry | undefined;
}>();

const emit = defineEmits<(e: 'close') => void>();

type Mode = 'create' | 'branch' | 'save' | 'popConfirm' | undefined;

const mode = computed<Mode>(() => {
  if (props.ops.pendingStashPop.value) return 'popConfirm';
  if (props.branchTarget !== undefined) return 'branch';
  if (props.saveOpen || props.saveSourceEntry !== undefined) return 'save';
  if (props.createOpen) return 'create';
  return undefined;
});

const active = computed(() => mode.value !== undefined);

// ---------------------------------------------------------------------------------------
// create mode
// ---------------------------------------------------------------------------------------

const message = ref('');
const includeUntracked = ref(props.includeUntrackedDefault);
const keepIndex = ref(false);
const messageId = useId();
const untrackedId = useId();
const keepIndexId = useId();
/** Wired from the caller's current changed-files selection when non-empty (`App.vue` passes
 *  it through `createOpen`'s own open call — see that wiring's own comment); empty means "whole
 *  worktree", never a half-wired guess. Shown here as a read-only summary, not an editable field —
 *  there is no staging UI in this app to pick a *different* subset from. */
const pathspec = ref<readonly string[]>([]);

watch(
  () => props.createOpen,
  (isOpen) => {
    if (!isOpen) return;
    message.value = '';
    includeUntracked.value = props.includeUntrackedDefault;
    keepIndex.value = false;
  },
);

function cancelCreate(): void {
  emit('close');
}

async function submitCreate(): Promise<void> {
  const result = await props.ops.runStashPush({
    message: message.value.trim() === '' ? undefined : message.value.trim(),
    includeUntracked: includeUntracked.value,
    keepIndex: keepIndex.value,
    // `[...pathspec.value]` (not `pathspec.value` itself): `ref<readonly string[]>`'s own value is
    // a reactive `Proxy`-wrapped array (Vue's `toReactive`), and `mockBridge.ts`'s in-memory pipe
    // genuinely `structuredClone`s every request to mimic real `postMessage` — Chromium's
    // structured-clone algorithm rejects a `Proxy` outright ("[object Object] could not be
    // cloned"), even an empty one, regardless of what it wraps. A plain array copy is the one this
    // request needs; nothing here relies on the reactive wrapper past this call.
    paths: [...pathspec.value],
  });
  if (!result.ok) return;
  emit('close');
}

// ---------------------------------------------------------------------------------------
// branch mode
// ---------------------------------------------------------------------------------------

const branchName = ref('');
const branchPreflight = ref<StashBranchPreflight | undefined>(undefined);
const branchNameId = useId();
let previewToken = 0;

watch(
  () => props.branchTarget,
  (entry) => {
    if (entry === undefined) return;
    branchName.value = '';
    branchPreflight.value = undefined;
    void nextTick(form.reset);
  },
);

watch(branchName, async (name) => {
  const entry = props.branchTarget;
  const trimmed = name.trim();
  if (entry === undefined || trimmed === '') {
    branchPreflight.value = undefined;
    return;
  }
  const token = ++previewToken;
  const result = await props.ops.previewStashBranch(entry, trimmed);
  // A later keystroke's own preview may have already resolved and been rendered by the time this
  // one comes back — `previewRevertMainline`'s call sites have no comparable race (a mainline
  // pick fires once, not once per keystroke), so this dialog needs its own guard against a slow,
  // stale response clobbering a newer one.
  if (token === previewToken) branchPreflight.value = result;
});

const branchNameLocalError = computed(() => {
  if (branchName.value.trim() === '') return undefined;
  const { valid, error } = validateRefName(branchName.value.trim());
  return valid || error === undefined ? undefined : renderText(error);
});

const form = useFormErrors({
  branch: {
    value: () => branchName.value,
    error: () => branchNameLocalError.value ?? branchPreflight.value?.name.error ?? null,
  },
});

/** Only a genuinely invalid/taken name (or nothing typed yet) blocks the button — a `blocked`
 *  checkout half still lets the user create the branch; the point is that the branch creation
 *  always succeeds, only the pop half can fail, and the recommendation is to say so plainly
 *  afterward rather than refuse the attempt up front. */
const canSubmitBranch = computed(
  () =>
    branchName.value.trim() !== '' &&
    branchNameLocalError.value === undefined &&
    branchPreflight.value !== undefined &&
    branchPreflight.value.verdict !== 'invalidName',
);

function cancelBranch(): void {
  emit('close');
}

async function submitBranch(): Promise<void> {
  const entry = props.branchTarget;
  if (entry === undefined || !canSubmitBranch.value) return;
  const result = await props.ops.runStashBranch(entry, branchName.value.trim());
  if (!result.ok) return;
  emit('close');
}

// ---------------------------------------------------------------------------------------
// save mode: two sources, the dialog is the confirm step (no pre-flight endpoint exists for
// globalStashSave, same reason create mode has none), and the one thing worth showing up front is
// the limitation — `git stash create` cannot include untracked files, so the working-tree source is
// spelled out as tracked-only rather than hiding the gap.
// ---------------------------------------------------------------------------------------

type SaveSource = 'workingTree' | 'entry';

const saveLabel = ref('');
const saveSource = ref<SaveSource>('workingTree');
const saveLabelId = useId();
const saveSourceItems = computed(() => [
  {
    value: 'workingTree',
    label: t('git.dialog.stash.workingTree.label'),
    testid: 'stash-save-source-workingTree',
  },
  ...(props.saveSourceEntry
    ? [
        {
          value: 'entry',
          label: t('git.dialog.stash.entry.label', { label: stashLabel(props.saveSourceEntry) }),
          testid: 'stash-save-source-entry',
          tooltip: t('git.dialog.stash.entry.label', { label: stashLabel(props.saveSourceEntry) }),
        },
      ]
    : []),
]);

watch(
  () => [props.saveOpen, props.saveSourceEntry] as const,
  ([isOpen, entry]) => {
    if (!isOpen && entry === undefined) return;
    saveLabel.value = '';
    // Pre-selected to "this stash entry" when opened from a stack row (D13); otherwise "this
    // working tree" — the radio itself stays changeable either way, this only sets the default.
    saveSource.value = entry !== undefined ? 'entry' : 'workingTree';
  },
);

/** F14: the working-tree source's own tracked-only limitation is spelled out whenever there is
 *  something it would actually miss, rather than unconditionally — a warning that is always true
 *  reads as noise. */
const saveHasUntracked = computed(() => (props.ops.statusSummary.value?.counts.untracked ?? 0) > 0);

const canSubmitSave = computed(
  () => saveLabel.value.trim() !== '' && !/[\r\n]/.test(saveLabel.value),
);

function cancelSave(): void {
  emit('close');
}

async function submitSave(): Promise<void> {
  if (!canSubmitSave.value) return;
  const sha = saveSource.value === 'entry' ? props.saveSourceEntry?.sha : undefined;
  const result = await props.ops.runGlobalStashSave(saveLabel.value.trim(), sha);
  if (!result.ok) return;
  emit('close');
}

// ---------------------------------------------------------------------------------------
// popConfirm mode (OQ7: one dialog, verb and one sentence differ)
// ---------------------------------------------------------------------------------------

const pending = computed(() => props.ops.pendingStashPop.value);

function cancelPop(): void {
  props.ops.resolveStashPopDialog(false);
}

const popConfirmLabel = computed(() => {
  const forced = pending.value?.preflight.verdict === 'blocked';
  if (pending.value?.verb === 'pop') {
    return t(forced ? 'git.dialog.stash.popAnyway.confirm' : 'git.dialog.stash.popConfirm.confirm');
  }
  return t(forced ? 'git.dialog.stash.applyAnyway.confirm' : 'common.action.apply.label');
});

function confirmPop(): void {
  props.ops.resolveStashPopDialog(true);
}

// ---------------------------------------------------------------------------------------
// mode-dispatched title / close, for the one shared dialog shell
// ---------------------------------------------------------------------------------------

const title = computed(() => {
  if (mode.value === 'create') return t('git.toolbar.stash.tooltip');
  if (mode.value === 'branch') return t('git.dialog.stash.branch.title');
  if (mode.value === 'save') return t('git.dialog.stash.save.title');
  if (mode.value === 'popConfirm' && pending.value) {
    return t(
      pending.value.verb === 'pop'
        ? 'git.dialog.stash.popConfirm.title'
        : 'git.dialog.stash.applyConfirm.title',
      { stash: stashRef(pending.value.preflight.stashIndex) },
    );
  }
  return '';
});

// A template literal would put two closing braces back to back, which the Vue parser reads as a
// mustache delimiter in templates; building it here keeps the template clean.
function stashRef(index: number | string): string {
  return `stash@{${index}}`;
}

function onClose(): void {
  if (mode.value === 'create') cancelCreate();
  else if (mode.value === 'branch') cancelBranch();
  else if (mode.value === 'save') cancelSave();
  else if (mode.value === 'popConfirm') cancelPop();
}
</script>

<template>
  <Dialog :open="active" @update:open="(v) => !v && onClose()">
    <DialogContent
      :aria-describedby="undefined"
      size="md"
      :dirty="(mode === 'create' && message !== '') || (mode === 'branch' && branchName !== '')"
    >
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <template v-if="mode === 'create'">
          <Field>
            <FieldLabel :for="messageId">{{ t('git.dialog.stash.message.label') }}</FieldLabel>
            <Input
              :id="messageId"
              v-model="message"
              type="text"
              class="w-full"
              :placeholder="t('git.dialog.stash.message.placeholder')"
            />
          </Field>
          <Field orientation="horizontal">
            <Checkbox :id="untrackedId" v-model="includeUntracked" />
            <FieldLabel :for="untrackedId">
              <CatalogText k="git.dialog.stash.untracked.label">
                <template #flag><code class="font-data">-u</code></template>
              </CatalogText>
            </FieldLabel>
          </Field>
          <Field orientation="horizontal">
            <Checkbox :id="keepIndexId" v-model="keepIndex" />
            <FieldLabel :for="keepIndexId">
              <CatalogText k="git.dialog.stash.keepIndex.label">
                <template #flag><code class="font-data">--keep-index</code></template>
              </CatalogText>
            </FieldLabel>
          </Field>
          <FieldDescription v-if="pathspec.length > 0">
            {{ t('git.dialog.stash.pathspec.hint', { count: pathspec.length }) }}
          </FieldDescription>
        </template>

        <template v-else-if="mode === 'branch'">
          <FieldDescription>
            <CatalogText
              k="git.dialog.stash.from.label"
              :params="{ message: branchTarget?.message ?? '' }"
            >
              <template #stash
                ><Identifier :value="stashRef(branchTarget?.index ?? '')" /></template
              >
            </CatalogText>
          </FieldDescription>
          <Field>
            <FieldLabel :for="branchNameId">{{ t('git.common.branchName.label') }}</FieldLabel>
            <Input
              :id="branchNameId"
              v-model="branchName"
              type="text"
              class="w-full"
              v-bind="form.invalidAttrs('branch')"
            />
            <FieldError v-if="form.errorFor('branch')" :id="form.errorId('branch')">{{
              form.errorFor('branch')
            }}</FieldError>
          </Field>
          <Alert v-if="branchPreflight?.verdict === 'blocked'" variant="warn">
            <AlertDescription>
              {{ t('git.dialog.stash.blocked.hint') }}
            </AlertDescription>
          </Alert>
        </template>

        <template v-else-if="mode === 'save'">
          <Field>
            <FieldLabel :for="saveLabelId">{{ t('git.dialog.stash.saveLabel.label') }}</FieldLabel>
            <Input :id="saveLabelId" v-model="saveLabel" type="text" class="w-full" />
          </Field>
          <FieldSet>
            <FieldLegend>{{ t('common.field.source.label') }}</FieldLegend>
            <SecondaryTabs
              variant="choice"
              size="kira-lg"
              wrap
              :aria-label="t('common.field.source.label')"
              :items="saveSourceItems"
              :model-value="saveSource"
              data-testid="stash-save-source"
              @update:model-value="(v) => (saveSource = v as SaveSource)"
            />
            <Alert v-if="saveSource === 'workingTree' && saveHasUntracked" variant="warn">
              <AlertDescription>
                <CatalogText k="git.dialog.stash.untrackedLimit.hint">
                  <template #command><code class="font-data">git stash create</code></template>
                </CatalogText>
              </AlertDescription>
            </Alert>
          </FieldSet>
          <FieldDescription>{{ t('git.dialog.stash.copied.hint') }}</FieldDescription>
        </template>

        <template v-else-if="mode === 'popConfirm' && pending">
          <template v-for="blocker in pending.preflight.blockers" :key="blocker.kind">
            <Alert v-if="blocker.kind === 'untrackedCollision'" variant="warn"
              ><AlertDescription>
                <p>{{ t('git.dialog.stash.untrackedCollision.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in blocker.paths" :key="path"><Identifier :value="path" /></li>
                </ul>
                <p>{{ t('git.dialog.stash.untrackedCollision.hint') }}</p>
              </AlertDescription></Alert
            >
            <Alert v-else-if="blocker.kind === 'localChangesWouldBeOverwritten'" variant="warn"
              ><AlertDescription>
                <p>{{ t('git.dialog.stash.localChanges.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in blocker.paths" :key="path"><Identifier :value="path" /></li>
                </ul>
                <p>{{ t('git.dialog.stash.localChanges.hint') }}</p>
              </AlertDescription></Alert
            >
            <Alert v-else variant="warn">
              <AlertDescription>{{ t('git.common.operationInProgress.body') }}</AlertDescription>
            </Alert>
          </template>

          <template v-if="pending.preflight.blockers.length === 0">
            <div v-if="pending.preflight.prediction.kind === 'clean'" class="text-ok">
              {{ t('git.dialog.preflight.clean.status') }}
            </div>
            <Alert v-else-if="pending.preflight.prediction.kind === 'conflicts'" variant="warn"
              ><AlertDescription>
                <p>{{ t('git.dialog.preflight.conflicts.body') }}</p>
                <ul class="max-h-40 overflow-y-auto pl-3 font-data text-kira-md">
                  <li v-for="path in pending.preflight.prediction.paths" :key="path">
                    <Identifier :value="path" />
                  </li>
                </ul>
                <p>{{
                  t(
                    pending.verb === 'pop'
                      ? 'git.dialog.stash.keptPop.hint'
                      : 'git.dialog.stash.keptApply.hint',
                  )
                }}</p>
              </AlertDescription></Alert
            >
            <FieldDescription v-else>
              {{
                t('git.dialog.preflight.unknown.error', {
                  reason: pending.preflight.prediction.reason,
                })
              }}
            </FieldDescription>
          </template>
        </template>
      </DialogBody>

      <DialogFooter>
        <template v-if="mode === 'create'">
          <Button variant="dialog-primary" size="kira-lg" @click="submitCreate">{{
            t('git.toolbar.stash.label')
          }}</Button>
        </template>
        <template v-else-if="mode === 'branch'">
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :disabled="!canSubmitBranch"
            @click="submitBranch"
          >
            {{ t('common.action.create.label') }}
          </Button>
        </template>
        <template v-else-if="mode === 'save'">
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :disabled="!canSubmitSave"
            @click="submitSave"
          >
            {{ t('common.action.save.label') }}
          </Button>
        </template>
        <template v-else-if="mode === 'popConfirm' && pending">
          <Button variant="dialog-primary" size="kira-lg" @click="confirmPop">
            {{ popConfirmLabel }}
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
