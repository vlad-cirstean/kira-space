<script setup lang="ts">
import { proofDiagramPayloadSchema } from '@shared/domain/proof';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed } from 'vue';
import ProofDiagram from '../../../studio/proof/ProofDiagram.vue';
import { useProofStep } from '../../../studio/proof/queries';

// One diagram step drawn inline. `diagramRender.ts` lazy-imports mermaid, so it stays out of the main chunk.
const props = defineProps<{ proofId: string; rev: number; ordinal: number; description: string }>();
const step = useProofStep(
  () => props.proofId,
  () => props.rev,
  () => props.ordinal,
);
const parsed = computed(() => {
  if (!step.data.value) return null;
  const r = proofDiagramPayloadSchema.safeParse(step.data.value.data);
  return r.success ? r.data : null;
});
</script>

<template>
  <div
    class="h-64 overflow-hidden rounded-kira-sm border"
    :data-ordinal="ordinal"
    data-testid="ade-proof-diagram"
  >
    <ProofDiagram
      v-if="parsed"
      :payload="parsed"
      :description="description"
      :seed="{ top: 0, left: 0 }"
    />
    <FieldDescription v-else-if="step.error.value" class="p-2">
      {{ errorText(step.error.value) }}
    </FieldDescription>
    <FieldDescription v-else-if="step.data.value" class="p-2">
      {{ t('studio.proof.diagram.invalid.title') }}
    </FieldDescription>
  </div>
</template>
