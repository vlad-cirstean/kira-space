<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { Handle, Position } from '@vue-flow/core';
import { useEventListener } from '@vueuse/core';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, ref, useTemplateRef } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import { promptParts } from '../automation/smartStepBody';
import { isDefaultSandbox } from '../board/stepSandbox';
import { KIND_TONE, kindLabel, runsOnLabel, STEP_KINDS } from '../board/workflowForm';
import { resultHandleId, type StepNodeData } from '../board/workflowGraph';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useStepRemoval } from './useStepRemoval';

// One step on the canvas: number, name, kind, what it runs, and one connector per result. Fixed size;
// full text lives in the inspector.
const props = defineProps<{ data: StepNodeData }>();
const draft = useAdeWorkflowDraftStore();
const { confirmRemoveStep } = useStepRemoval();
const menuOpen = ref(false);
const root = useTemplateRef<HTMLElement>('root');
useEventListener(root, 'contextmenu', (e: MouseEvent) => {
  e.preventDefault();
  e.stopPropagation();
  menuOpen.value = true;
});
const step = computed(() => props.data.step);
const SANDBOX_CHIP = {
  write: 'space.ade.sandbox.write.label',
  read: 'space.ade.sandbox.read.label',
  off: 'space.ade.sandbox.off.label',
} as const;
const manual = computed(() => step.value.kind === 'manual');
const showMeta = computed(
  () =>
    (!manual.value && !step.value.outsideRepo) ||
    !isDefaultSandbox(step.value.kind, step.value.sandbox),
);
const body = computed(() => {
  const s = step.value;
  if (s.kind === 'review' && s.review)
    return t(
      s.review.sendTo === 'new'
        ? 'space.ade.workflows.stepNode.sendsNew.label'
        : 'space.ade.workflows.stepNode.sendsImplementer.label',
    );
  if (s.kind === 'script') return s.command || t('space.ade.workflows.stepNode.noCommand.label');
  if (s.kind === 'manual')
    return t(
      s.session
        ? 'space.ade.workflows.stepNode.session.label'
        : 'space.ade.workflows.stepNode.manual.label',
    );
  return s.smartScript ? `✦ ${s.smartScript}` : s.prompt;
});
const parts = computed(() =>
  step.value.kind === 'agent' ? promptParts(body.value) : [body.value],
);
const loops = computed(() => step.value.results.some((r) => r.loop));
const LABEL_MAX = 4;
const labelled = computed(() => step.value.results.length <= LABEL_MAX);
const tip = (r: StepNodeData['step']['results'][number]): string => {
  const outcome = t(
    r.ok ? 'space.ade.workflows.results.ok.label' : 'space.ade.workflows.results.notOk.label',
  );
  return r.next === 'stop'
    ? t('space.ade.workflows.stepNode.resultStops.tooltip', { id: r.id, outcome })
    : t('space.ade.workflows.stepNode.resultGoes.tooltip', { id: r.id, outcome, next: r.next });
};
</script>

<template>
  <div
    class="flex flex-col rounded-kira border bg-elevated text-kira-sm"
    :class="[
      data.selected ? 'border-info' : 'border-border',
      step.skip ? 'text-muted-foreground' : '',
    ]"
    :style="{ width: `${data.w}px`, height: `${data.h}px` }"
    data-testid="ade-wf-node"
    :data-step-id="step.id"
    :data-start="data.start"
    :data-kind="step.kind"
    :data-skipped="step.skip"
    ref="root"
    :data-selected="data.selected"
  >
    <Handle
      id="in"
      type="target"
      :position="Position.Top"
      class="!size-2.5 !border-0 !bg-muted-foreground"
    />
    <Handle
      id="loop-in"
      type="target"
      :position="Position.Top"
      class="!size-2 !border-0 !bg-muted-foreground !left-5"
    />
    <div class="flex items-center gap-1.5 px-2 pt-2">
      <AdeTip v-if="data.start" :text="t('common.action.start.label')"
        ><span class="text-tone-green" data-testid="ade-wf-node-start">▶</span></AdeTip
      >
      <span class="shrink-0 font-medium" data-testid="ade-wf-node-number"
        >{{ data.index + 1 }}.</span
      >
      <span
        class="min-w-0 flex-1 truncate text-kira-md font-medium"
        :class="step.skip ? 'line-through' : ''"
        data-testid="ade-wf-node-name"
        >{{
          step.name
        }}</span
      >
      <AdeChip
        :label="kindLabel(step.kind)"
        :tone="KIND_TONE[step.kind]"
        data-testid="ade-wf-node-kind"
      />
      <span
        v-if="step.before === 'approval'"
        class="shrink-0 text-warn-text"
        data-testid="ade-wf-node-gate"
        >{{
          t('space.ade.needs.kind.approval.label')
        }}</span
      >
      <span v-if="step.smartScript" class="shrink-0 text-info">{{ t('common.smart.label') }}</span>
      <span
        v-if="step.outsideRepo"
        class="shrink-0 text-muted-foreground"
        data-testid="ade-wf-node-outside"
        >{{
          t('space.ade.workflows.stepNode.outside.label')
        }}</span
      >
      <span v-if="loops" class="shrink-0 text-muted-foreground" data-testid="ade-wf-node-returns">{{
        step.maxReturns > 0
          ? t('space.ade.workflows.stepNode.returns.label', { limit: step.maxReturns })
          : t('space.ade.workflows.stepNode.returnsUnlimited.label')
      }}</span>
      <DropdownMenu v-model:open="menuOpen">
        <DropdownMenuTrigger as-child>
          <Button
            variant="toolbar"
            size="kira-icon"
            class="nodrag nopan shrink-0"
            :aria-label="t('space.ade.workflows.graph.stepMenu.aria')"
            data-testid="ade-wf-step-menu"
            @click.stop
          >
            <CodiconIcon :name="ICON.moreActions" :size="13" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuSub>
            <DropdownMenuSubTrigger data-testid="ade-wf-step-add-after">
              {{ t('space.ade.workflows.graph.addStepAfter.label') }}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem
                v-for="k in STEP_KINDS"
                :key="k"
                :data-testid="`ade-wf-step-add-after-${k}`"
                @select="draft.createStep(k, step.id)"
              >
                {{ kindLabel(k) }}
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            :disabled="!data.canUp"
            data-testid="ade-wf-step-up"
            @select="draft.moveStepBy(step.id, -1)"
          >
            {{ t('common.moveUp.tooltip') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            :disabled="!data.canDown"
            data-testid="ade-wf-step-down"
            @select="draft.moveStepBy(step.id, 1)"
          >
            {{ t('common.moveDown.tooltip') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            :disabled="data.start"
            data-testid="ade-wf-step-start"
            @select="draft.makeStart(step.id)"
          >
            {{ t('space.ade.workflows.stepInspector.makeStart.label') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            :disabled="!step.skip && !data.canSkip"
            data-testid="ade-wf-step-skip"
            @select="draft.patchStep(step.id, { skip: !step.skip })"
          >
            {{
              step.skip
                ? t('space.ade.workflows.stepInspector.unskip.label')
                : t('space.ade.stepBlock.skipThis.label')
            }}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            class="text-error"
            :disabled="!data.canRemove"
            data-testid="ade-wf-step-remove"
            @select="fireAndForget(confirmRemoveStep(step.id), 'adeWorkflows')"
          >
            {{ t('common.action.remove.label') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <div v-if="showMeta" class="flex items-center gap-1.5 px-2 text-subtle">
      <span v-if="!manual && !step.outsideRepo" class="min-w-0 truncate">{{
        runsOnLabel(step.runsOn)
      }}</span>
      <span
        v-if="!isDefaultSandbox(step.kind, step.sandbox)"
        class="shrink-0"
        :class="step.sandbox.access === 'off' ? 'text-warn-text' : ''"
        data-testid="ade-wf-node-sandbox"
        >{{
          t(SANDBOX_CHIP[step.sandbox.access])
        }}</span
      >
    </div>
    <div
      class="truncate px-2 text-muted-foreground [&>span]:whitespace-nowrap"
      :class="step.kind === 'script' ? 'font-data' : ''"
      data-testid="ade-wf-node-prompt"
    >
      <VarText :parts="parts" />
    </div>
    <Handle
      v-if="manual"
      id="out"
      type="source"
      :position="Position.Bottom"
      class="!size-2 !border-0 !bg-muted-foreground"
    />
    <div v-else class="mt-auto flex">
      <AdeTip v-for="r in step.results" :key="r.id" :text="tip(r)">
        <span
          class="relative flex min-w-0 flex-1 items-center justify-center gap-1 px-1 py-1"
          data-testid="ade-wf-node-result"
          :data-result="r.id"
          :data-ok="r.ok"
        >
          <StatusDot
            :color="r.ok ? 'var(--color-tone-green-solid)' : 'var(--color-tone-red-solid)'"
          />
          <span v-if="labelled" class="truncate">{{ r.id }}</span>
          <Handle
            :id="resultHandleId(r.id)"
            type="source"
            :position="Position.Bottom"
            class="!size-3 !border-2 !border-bg"
            :class="r.ok ? '!bg-tone-green-solid' : '!bg-tone-red-solid'"
          />
        </span>
      </AdeTip>
    </div>
  </div>
</template>
