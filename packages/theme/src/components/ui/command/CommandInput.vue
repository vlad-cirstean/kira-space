<script setup lang="ts">
import { SearchIcon } from '@lucide/vue';
import { InputGroup, InputGroupAddon } from '@theme/components/ui/input-group';
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { ListboxFilterProps } from 'reka-ui';
import { ListboxFilter, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { useCommand } from '.';

defineOptions({
  inheritAttrs: false,
});

const props = defineProps<
  ListboxFilterProps & {
    class?: HTMLAttributes['class'];
  }
>();

const delegatedProps = reactiveOmit(props, 'class');

const forwardedProps = useForwardProps(delegatedProps);

const { filterState } = useCommand();
</script>

<template>
  <div data-slot="command-input-wrapper" class="p-1 pb-0">
    <InputGroup
      variant="default"
      class="bg-field border-transparent h-8! rounded-kira-sm! shadow-none! *:data-[slot=input-group-addon]:pl-2!"
    >
      <ListboxFilter
        v-bind="{ ...forwardedProps, ...$attrs }"
        v-model="filterState.search"
        data-slot="command-input"
        auto-focus
        :class="
          cn(
            'w-full text-kira-md outline-hidden disabled:cursor-default disabled:opacity-50',
            props.class,
          )
        "
      />
      <InputGroupAddon>
        <SearchIcon class="size-4 shrink-0 opacity-50" />
      </InputGroupAddon>
    </InputGroup>
  </div>
</template>
