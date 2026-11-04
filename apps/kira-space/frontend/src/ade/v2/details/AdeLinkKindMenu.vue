<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import AdeChip from '../AdeChip.vue';
import { artifactKindLabel } from '../artifacts/status';
import type { Artifact } from '../wire';

// The kind chip of a stored row; picking another kind re-files the row.
defineProps<{ kind: Artifact['kind'] }>();
const emit = defineEmits<{ pick: [kind: Artifact['kind']] }>();

const KINDS: Artifact['kind'][] = ['changeRequest', 'issue', 'branch', 'deployment', 'doc', 'link'];
</script>

<template>
  <DropdownMenu :modal="false">
    <DropdownMenuTrigger as-child>
      <button
        type="button"
        class="shrink-0 rounded-kira-sm"
        :aria-label="t('space.ade.details.kindMenu.label')"
        :data-kind="kind"
        data-testid="ade-details-link-kind"
      >
        <AdeChip :label="artifactKindLabel(kind)" tone="grey" wide />
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start">
      <DropdownMenuRadioGroup
        :model-value="kind"
        @update:model-value="emit('pick', $event as Artifact['kind'])"
      >
        <DropdownMenuRadioItem
          v-for="k in KINDS"
          :key="k"
          :value="k"
          :data-kind="k"
          data-testid="ade-details-link-kind-item"
        >
          {{ artifactKindLabel(k) }}
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
