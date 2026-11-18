<script setup lang="ts">
import type { MaskKind } from '@shared/domain/mask';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed } from 'vue';
import { MASK_KIND_OPTIONS } from './maskKinds';

// "Mask as" dropdown: picking a kind adds the rule at once.
defineProps<{ testid: string }>();
const emit = defineEmits<{ select: [kind: MaskKind] }>();

const items = computed<MenuItem[]>(() =>
  MASK_KIND_OPTIONS.map((o) => ({
    type: 'item',
    id: o.value,
    testId: `mask-kind-menu-${o.value}`,
    label: t(o.label),
    run: () => emit('select', o.value),
  })),
);
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
      <MenuItems :items="items" />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
