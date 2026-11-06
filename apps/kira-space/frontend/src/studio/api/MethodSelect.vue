<script setup lang="ts">
import { HTTP_METHODS, type HttpMethod, httpMethodToken } from '@shared/domain/http';
import CodiconIcon from '@theme/CodiconIcon.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { cn } from '@theme/lib/utils';
import { methodTextClass } from '@theme/methodColor';

// App-drawn trigger: a native option's per-row colour lands only under `appearance: base-select`.
// Controlled: HttpRequestView patches tab state from its own @update:model-value handler.
defineProps<{ testid?: string }>();
const method = defineModel<HttpMethod>({ required: true });

function select(value: unknown): void {
  method.value = String(value) as HttpMethod;
}
</script>

<template>
  <DropdownMenu :modal="false">
    <div class="relative flex">
      <DropdownMenuTrigger as-child>
        <PickerTrigger
          size="kira"
          :class="cn('font-medium font-data', methodTextClass(httpMethodToken(method)))"
          :data-testid="testid"
          :data-value="method"
          >{{
            method
          }}</PickerTrigger
        >
      </DropdownMenuTrigger>
    </div>
    <DropdownMenuContent align="start" data-testid="method-menu">
      <DropdownMenuRadioGroup :model-value="method" @update:model-value="select">
        <DropdownMenuRadioItem
          v-for="m in HTTP_METHODS"
          :key="m"
          :value="m"
          :class="cn('font-medium', methodTextClass(httpMethodToken(m)))"
          :data-testid="`method-menu-item-${m}`"
          :data-value="m"
        >
          {{ m }}
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
