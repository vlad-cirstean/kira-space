<script setup lang="ts">
import type { MaskKind } from '@shared/domain/mask';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { MASK_KIND_OPTIONS } from './maskKinds';

// "Mask as" dropdown: picking a kind adds the rule at once.
defineProps<{ testid: string }>();
const emit = defineEmits<{ select: [kind: MaskKind] }>();
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button size="kira" variant="dialog" :data-testid="testid">
        {{ t('studio.project.connectionDialog.privacy.maskAs.label') }}
        <CodiconIcon name="chevron-down" :size="12" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem
        v-for="o in MASK_KIND_OPTIONS"
        :key="o.value"
        :data-testid="`mask-kind-menu-${o.value}`"
        @select="emit('select', o.value)"
      >
        {{ t(o.label) }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
