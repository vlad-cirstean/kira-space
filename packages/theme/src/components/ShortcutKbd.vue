<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import { Kbd } from '@theme/components/ui/kbd';
import { formatShortcut } from '@theme/shortcutText';

// The one place a chord chip renders; the text always comes from the registry.
defineProps<{ id: ShortcutId }>();
</script>

<template>
  <Kbd :data-shortcut="id">{{ formatShortcut(id) }}</Kbd>
</template>
