<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import type { WorkflowStep } from '../../wire';

// The results a step can end with, each with where it goes. The route selects are the keyboard path
// for everything a connection drag does on the canvas.
const props = defineProps<{ step: WorkflowStep; steps: WorkflowStep[]; fixed?: boolean }>();
const draft = useAdeWorkflowDraftStore();
const id = (k: string, rid: string): string => `ade-wf-result-${k}-${props.step.id}-${rid}`;
// A return needs a step that runs: manual and skipped steps up to this one are not offered, the
// current pick excepted so the select keeps showing it.
const targets = (next: string): WorkflowStep[] => {
  const at = props.steps.findIndex((s) => s.id === props.step.id);
  return props.steps.filter(
    (s, i) => s.id === next || !(i <= at && (s.kind === 'manual' || s.skip)),
  );
};
</script>

<template>
  <div class="flex flex-col gap-1.5" data-testid="ade-wf-results">
    <div class="text-kira-sm text-muted-foreground">
      {{ t('space.ade.workflows.results.intro.hint') }}
    </div>
    <Card
      v-for="r in step.results"
      :key="r.id"
      variant="section"
      class="gap-1 p-2"
      data-testid="ade-wf-result"
      :data-result="r.id"
    >
      <div
        v-if="fixed"
        class="flex items-center gap-2 font-data text-kira-sm"
        data-testid="ade-wf-result-fixed"
      >
        {{ r.id }}
      </div>
      <div v-else class="flex items-center gap-2">
        <Label :for="id('id', r.id)" class="sr-only">{{
          t('space.ade.workflows.results.id.aria')
        }}</Label>
        <Input
          :id="id('id', r.id)"
          :model-value="r.id"
          size="kira"
          class="min-w-0 flex-1 font-data"
          data-testid="ade-wf-result-id"
          @update:model-value="
            (v: string | number) => draft.changeResult(step.id, r.id, { id: String(v) })
          "
        />
        <Label :for="id('ok', r.id)" class="text-kira-sm text-muted-foreground">{{
          t('space.ade.workflows.results.ok.label')
        }}</Label>
        <Checkbox
          :id="id('ok', r.id)"
          :model-value="r.ok"
          data-testid="ade-wf-result-ok"
          @update:model-value="(v) => draft.changeResult(step.id, r.id, { ok: v === true })"
        />
        <TooltipIconButton
          icon="close"
          :label="t('space.ade.workflows.results.remove.tooltip')"
          class="text-error"
          :disabled="step.results.length < 2"
          data-testid="ade-wf-result-remove"
          @click="draft.deleteResult(step.id, r.id)"
        />
      </div>
      <Label v-if="!fixed" :for="id('desc', r.id)" class="sr-only">{{
        t('space.ade.workflows.results.description.aria')
      }}</Label>
      <Input
        v-if="!fixed"
        :id="id('desc', r.id)"
        :model-value="r.description"
        :placeholder="t('space.ade.workflows.results.description.placeholder')"
        size="kira"
        maxlength="200"
        data-testid="ade-wf-result-description"
        @update:model-value="
          (v: string | number) => draft.changeResult(step.id, r.id, { description: String(v) })
        "
      />
      <div class="flex flex-wrap items-center gap-2 text-kira-sm text-muted-foreground">
        <Label :for="id('route', r.id)">{{ t('space.ade.workflows.results.route.label') }}</Label>
        <NativeSelect
          :id="id('route', r.id)"
          variant="bordered"
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
      </div>
    </Card>
    <Button
      variant="dialog"
      size="kira-lg"
      v-if="!fixed"
      class="self-start bg-transparent"
      :disabled="step.results.length >= 12"
      data-testid="ade-wf-add-result"
      @click="draft.createResult(step.id)"
    >
      {{ t('space.ade.workflows.results.add.label') }}
    </Button>
  </div>
</template>
