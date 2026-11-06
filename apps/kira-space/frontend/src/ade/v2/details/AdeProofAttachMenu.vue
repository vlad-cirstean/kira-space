<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import { ref } from 'vue';
import { useLinkProof, useProofChoices } from '../queries';

// Attaches a closed, unattached proof session to the task.
const props = defineProps<{ taskId: string }>();
const open = ref(false);
const choices = useProofChoices(open);
const link = useLinkProof();
</script>

<template>
  <DropdownMenu v-model:open="open" :modal="false">
    <DropdownMenuTrigger as-child>
      <TooltipIconButton
        icon="beaker"
        :label="t('space.ade.proof.attach.tooltip')"
        data-testid="ade-proof-attach"
      />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="max-h-72 overflow-auto">
      <DropdownMenuItem
        v-if="!choices.data.value?.proofs.length"
        disabled
        data-testid="ade-proof-attach-empty"
      >
        {{ t('space.ade.proof.attach.empty.body') }}
      </DropdownMenuItem>
      <DropdownMenuItem
        v-for="p in choices.data.value?.proofs ?? []"
        :key="p.proofId"
        data-testid="ade-proof-attach-item"
        @select="link.mutate({ taskId: props.taskId, proofId: p.proofId })"
      >
        <span class="truncate">{{ p.name }}</span>
        <span class="ml-auto pl-3 text-kira-sm text-subtle">{{ formatRelative(p.closedAt) }}</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
