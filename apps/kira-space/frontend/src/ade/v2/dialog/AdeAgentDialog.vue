<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import ResetToDefaultButton from '@theme/components/ResetToDefaultButton.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
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
import { Field, FieldContent, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import { computed, ref } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import { baseOf } from '../board/rebaseActions';
import { baseChoiceOf, baseGroups, baseOptions, baseValue } from '../branchOptions';
import AdeSetupProgress from '../panel/AdeSetupProgress.vue';
import { useRebasePreview, useRepoBranches } from '../queries';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { composeDialog, isRebaseKind } from './compose';

// The one Claude dialog every opener shares (mockup `dlg`): kind-specific title, busy alert, targets,
// editable message, push switch and archive risk, then Cancel and the kind's send button.
const dialogs = useAdeDialogsStore();
// The dialogs store owns the failure text; these only track pending.
const sendAction = useAsyncAction(() => dialogs.send());
const discardAction = useAsyncAction(() => dialogs.discardAnyway());
const sending = computed(() => sendAction.pending.value || discardAction.pending.value);
// The rebase kinds show the server's own prompt; a pick or a switch asks for it again.
const previewArgs = computed(() => {
  const s = dialogs.spec;
  if (!s || !isRebaseKind(s) || s.draft) return null;
  return {
    branchId: s.branchId ?? '',
    onto: s.onto ?? null,
    queueWith: s.queueWith ?? '',
    push: dialogs.push,
    autostash: dialogs.autostash,
  };
});
const preview = useRebasePreview(previewArgs);

const view = computed(() => {
  const c = dialogs.ctx;
  const s = dialogs.spec;
  if (!c || !s) return null;
  return composeDialog(c, s, {
    msg: dialogs.msg,
    push: dialogs.push,
    override: dialogs.override,
    autostash: dialogs.autostash,
    preview: preview.data.value ?? null,
  });
});

const previewError = computed(() => {
  const e = preview.error.value;
  return e ? errorText(e) : '';
});

// Change base: the branch being rebased, for the picker's repo and the base it shows now.
const baseBranch = computed(() => {
  const s = dialogs.spec;
  return s?.kind === 'changeBase' ? dialogs.ctx?.graph.byBranch.get(s.branchId ?? '') : undefined;
});
const currentBase = computed(() => {
  const c = dialogs.ctx;
  const b = baseBranch.value;
  return c && b ? baseOf(b, c.graph, c.repoState(b.codeRepoId).mainName) : '';
});

const baseOpen = ref(false);
const baseChoice = computed(() => dialogs.spec?.onto ?? null);
const baseData = useRepoBranches(() =>
  baseBranch.value && (baseOpen.value || baseChoice.value !== null)
    ? { codeRepoId: baseBranch.value.codeRepoId, branchId: baseBranch.value.id }
    : null,
);
const baseOpts = computed(() => (baseData.data.value ? baseOptions(baseData.data.value) : []));
const baseShown = computed(() => {
  const c = baseChoice.value;
  if (c === null) return currentBase.value;
  const v = baseValue(c);
  return baseOpts.value.find((o) => o.value === v)?.label ?? (c.ref || c.branchId);
});

// The launch waits behind a prepare script: show each branch's, and why it is held or refused.
// A session launch covers the whole task; the other kinds cover their targets.
const setups = computed(() => {
  const c = dialogs.ctx;
  const v = view.value;
  const spec = dialogs.spec;
  if (!c || !v || !spec || v.isArchive) return [];
  const ids =
    spec.kind === 'session'
      ? [...c.graph.byBranch.values()].filter((b) => b.taskId === spec.taskId).map((b) => b.id)
      : v.targets.map((tg) => tg.branchId);
  return ids.flatMap((id) => {
    const b = c.graph.byBranch.get(id);
    const state = b?.setup?.state;
    return b && b.kind === 'mine' && b.name !== '' && (state === 'running' || state === 'failed')
      ? [{ branch: b, repo: c.repo(b.codeRepoId).nick }]
      : [];
  });
});

async function send(): Promise<void> {
  await sendAction.run();
}

async function discard(): Promise<void> {
  await discardAction.run();
}
</script>

<template>
  <Dialog v-if="view" :open="true" @update:open="(v) => !v && dialogs.close()">
    <DialogContent size="lg" :dirty="view.edited" data-testid="ade-dialog">
      <DialogHeader>
        <DialogTitle data-testid="ade-dialog-title">{{ view.title }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <Alert v-if="view.headless" variant="error" data-testid="ade-dialog-headless">
          <AlertDescription>{{ view.headless }}</AlertDescription>
        </Alert>
        <Alert v-else-if="view.busyShown" variant="error" data-testid="ade-dialog-busy">
          <AlertDescription>
            <div class="flex items-start gap-3">
              <div class="flex min-w-0 flex-1 flex-col gap-1">
                <span class="font-medium">{{ view.busyTitle }}</span>
                <div
                  v-for="row in view.busy"
                  :key="row.text"
                  class="flex items-center gap-1.5 text-kira-sm"
                  data-testid="ade-dialog-busy-row"
                >
                  <AdeActivityIcon :kind="row.kind" />
                  <span>{{ row.text }}</span>
                </div>
              </div>
              <Button
                variant="dialog"
                size="kira"
                class="shrink-0"
                data-testid="ade-dialog-override"
                @click="dialogs.override = !dialogs.override"
              >
                {{ view.overrideLabel }}
              </Button>
            </div>
          </AlertDescription>
        </Alert>

        <AdeSetupProgress
          v-for="s in setups"
          :key="s.branch.id"
          :branch="s.branch"
          :repo="s.repo"
          :compact="setups.length > 1"
          data-testid="ade-dialog-setup"
        />

        <Field v-if="baseBranch" orientation="responsive" data-testid="ade-dialog-base">
          <FieldLabel for="ade-dialog-base-picker">{{ t('space.common.base.label') }}</FieldLabel>
          <BranchPicker
            id="ade-dialog-base-picker"
            v-model:open="baseOpen"
            testid="ade-base-picker"
            :model-value="baseValue(baseChoice)"
            :branches="baseOpts"
            :groups="baseGroups()"
            :placeholder="currentBase"
            :trigger-label="baseShown"
            :loading="baseData.isPending.value"
            @select="(o) => dialogs.pickBase(baseChoiceOf(o.value))"
          />
        </Field>

        <Alert
          v-for="b in view.blockers"
          :key="b.branchId + b.kind"
          variant="error"
          data-testid="ade-dialog-blocker"
          :data-kind="b.kind"
        >
          <AlertDescription>{{ b.text }}</AlertDescription>
        </Alert>
        <Field v-if="view.canAutostash" orientation="horizontal">
          <Checkbox
            id="ade-dialog-autostash"
            v-model="dialogs.autostash"
            data-testid="ade-dialog-autostash"
          />
          <FieldLabel for="ade-dialog-autostash">{{
            t('space.ade.agentDialog.autostash.label')
          }}</FieldLabel>
        </Field>
        <FieldDescription v-if="view.noOp" data-testid="ade-dialog-noop">
          {{ t('space.ade.agentDialog.noOp.hint') }}
        </FieldDescription>

        <Alert v-if="view.isArchive" variant="warn" data-testid="ade-dialog-risk">
          <AlertDescription>{{
            t('space.ade.agentDialog.risk.body', { risk: view.riskText })
          }}</AlertDescription>
        </Alert>

        <Field v-for="(tg, i) in view.targets" :key="tg.branchId" data-testid="ade-dialog-target">
          <div class="flex items-center gap-2">
            <AdeRepoTag :code-repo-id="tg.repoId" :label="tg.repo" />
            <FieldLabel class="min-w-0 flex-1 truncate font-data">{{ tg.branch }}</FieldLabel>
          </div>
          <SecondaryTabs
            variant="choice"
            size="kira-lg"
            :aria-label="tg.branch"
            :model-value="tg.options.find((o) => o.on)?.value ?? ''"
            :items="tg.options"
            :data-testid="`ade-dialog-target-${i}`"
            @update:model-value="(v) => dialogs.pick(tg.branchId, v)"
          />
        </Field>

        <Field v-if="view.canPush" orientation="horizontal">
          <Checkbox id="ade-dialog-push" v-model="dialogs.push" data-testid="ade-dialog-push" />
          <FieldContent>
            <FieldLabel for="ade-dialog-push">
              <span data-testid="ade-dialog-push-label">{{ view.pushLabel }}</span>
            </FieldLabel>
            <FieldDescription v-if="!dialogs.push">{{
              t('space.ade.agentDialog.pushOff.hint')
            }}</FieldDescription>
          </FieldContent>
        </Field>

        <Field v-if="view.showMessage">
          <div class="flex items-center justify-between gap-1">
            <FieldLabel for="ade-dialog-message">{{
              t('space.ade.agentDialog.message.label')
            }}</FieldLabel>
            <ResetToDefaultButton
              v-if="view.edited"
              data-testid="ade-dialog-reset"
              @click="dialogs.msg = null"
            />
          </div>
          <Textarea
            class="min-h-24 max-h-90 resize-y"
            id="ade-dialog-message"
            :model-value="view.message"
            data-testid="ade-dialog-message"
            @update:model-value="(v) => (dialogs.msg = String(v))"
          />
        </Field>
        <Field v-if="view.showMessage && view.suffix">
          <FieldLabel>{{ t('common.run.addedByKira.label') }}</FieldLabel>
          <CodeBlock class="text-muted-foreground" data-testid="ade-dialog-suffix">{{
            view.suffix
          }}</CodeBlock>
        </Field>

        <Alert v-if="previewError" variant="error" data-testid="ade-dialog-preview-error">
          <AlertDescription>{{ previewError }}</AlertDescription>
        </Alert>

        <Alert v-if="dialogs.error" variant="error" data-testid="ade-dialog-error">
          <AlertDescription>{{ dialogs.error }}</AlertDescription>
        </Alert>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="ade-dialog-cancel"
          @click="dialogs.close()"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <TextTooltip v-if="view.isArchive" :text="t('space.ade.agentDialog.deleteAnyway.tooltip')">
          <!-- conventions-allow N17: second footer action; one emphasised button rightmost (RD.2) -->
          <Button
            variant="dialog"
            size="kira-lg"
            :pending="sending"
            data-testid="ade-dialog-delete"
            @click="discard"
          >
            {{ t('space.ade.agentDialog.deleteAnyway.label') }}
          </Button>
        </TextTooltip>
        <Button
          :variant="view.overridden ? 'dialog-danger' : 'dialog-primary'"
          size="kira-lg"
          :pending="sending"
          :disabled="view.blocked || view.sendDisabled || dialogs.waitingSetup"
          data-testid="ade-dialog-send"
          @click="send"
        >
          {{
            dialogs.waitingSetup
              ? t('space.ade.agentDialog.startsWhenReady.loading')
              : view.sendLabel
          }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
