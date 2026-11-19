<script setup lang="ts">
import { grpcCodeClass, grpcCodeHint } from '@shared/domain/grpc';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Badge } from '@theme/components/ui/badge';

// The one gRPC status chip: tone from the code, the code meaning as its tooltip, "NAME (code)". Attrs
// (testid included) land on the Badge.
defineOptions({ inheritAttrs: false });
defineProps<{ code: number; codeName: string }>();
</script>

<template>
  <TextTooltip :text="grpcCodeHint(code)">
    <Badge data :variant="grpcCodeClass(code)" v-bind="$attrs">{{ codeName }} ({{ code }})</Badge>
  </TextTooltip>
</template>
