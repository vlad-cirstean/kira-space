<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import type { WorkflowStep } from '../../wire';
import { useStepScope } from '../stepScope';

// The results a step can end with, each with where it goes. The route selects are the keyboard path
// for everything a connection drag does on the canvas.
const props = defineProps<{ step: WorkflowStep; steps: WorkflowStep[]; fixed?: boolean }>();
const draft = useAdeWorkflowDraftStore();
const library = useStepScope() === 'library';
const id = (k: string, rid: string): string => `ade-wf-result-${k}-${props.step.id}-${rid}`;
// A return needs a step that runs: manual and skipped steps up to this one are not offered, the
// current pick excepted so the select keeps showing it.
const targets = (next: string): WorkflowStep[] => {
  if (library) return [];
  const at = props.steps.findIndex((s) => s.id === props.step.id);
  return props.steps.filter(
    (s, i) => s.id === next || !(i <= at && (s.kind === 'manual' || s.skip)),
  );
};
</script>

<template>
  <div class="flex flex-col gap-3" data-testid="ade-wf-results">
    <FieldDescription>{{ t('space.ade.workflows.results.intro.hint') }}</FieldDescription>
    <div
      v-for="r in step.results"
      :key="r.id"
      class="flex flex-col gap-1"
      data-testid="ade-wf-result"
      :data-result="r.id"
    >
      <div class="flex items-center gap-2">
        <span
          v-if="fixed"
          class="min-w-0 flex-1 font-data text-kira-sm"
          data-testid="ade-wf-result-fixed"
          >{{
            r.id
          }}</span
        >
        <template v-else>
          <Label :for="id('id', r.id)" class="sr-only">{{
            t('space.ade.workflows.results.id.aria')
          }}</Label>
          <Input
            :id="id('id', r.id)"
            :model-value="r.id"
            class="min-w-0 flex-1 font-data"
            data-testid="ade-wf-result-id"
            @update:model-value="
              (v: string | number) => draft.changeResult(step.id, r.id, { id: String(v) })
            "
          />
          <Checkbox
            :id="id('ok', r.id)"
            :model-value="r.ok"
            data-testid="ade-wf-result-ok"
            @update:model-value="(v) => draft.changeResult(step.id, r.id, { ok: v === true })"
          />
          <Label :for="id('ok', r.id)" class="text-kira-sm text-muted-foreground">{{
            t('space.ade.workflows.results.ok.label')
          }}</Label>
        </template>
        <Label :for="id('route', r.id)" class="sr-only">{{
          t('space.ade.workflows.results.route.label')
        }}</Label>
        <NativeSelect
          :id="id('route', r.id)"
          variant="bordered"
          class="w-36 shrink-0"
          :model-value="r.next"
          data-testid="ade-wf-result-route"
          @update:model-value="(v) => draft.route(step.id, r.id, String(v))"
        >
          <option value="next">{{ t('space.ade.stepMover.next.tooltip') }}</option>
          <option v-for="s in targets(r.next)" :key="s.id" :value="s.id">{{
            s.id === step.id ? t('space.ade.workflows.route.thisStep.option') : s.name
          }}</option>
          <option value="end">{{ t('space.ade.workflows.route.end.option') }}</option>
          <option v-if="!r.ok" value="stop">{{ t('common.action.stop.label') }}</option>
        </NativeSelect>
        <TooltipIconButton
          v-if="!fixed"
          icon="close"
          :label="t('space.ade.workflows.results.remove.tooltip')"
          class="text-error"
          :disabled="step.results.length < 2"
          data-testid="ade-wf-result-remove"
          @click="draft.deleteResult(step.id, r.id)"
        />
      </div>
      <template v-if="!fixed">
        <Label :for="id('desc', r.id)" class="sr-only">{{
          t('space.ade.workflows.results.description.aria')
        }}</Label>
        <Input
          :id="id('desc', r.id)"
          :model-value="r.description"
          :placeholder="t('space.ade.workflows.results.description.placeholder')"
          maxlength="200"
          data-testid="ade-wf-result-description"
          @update:model-value="
            (v: string | number) => draft.changeResult(step.id, r.id, { description: String(v) })
          "
        />
      </template>
    </div>
    <Button
      v-if="!fixed"
      variant="dialog"
      size="kira-lg"
      class="self-start"
      :disabled="step.results.length >= 12"
      data-testid="ade-wf-add-result"
      @click="draft.createResult(step.id)"
    >
      <CodiconIcon :name="ICON.add" :size="12" />
      {{ t('space.ade.workflows.results.add.label') }}
    </Button>
  </div>
</template>
