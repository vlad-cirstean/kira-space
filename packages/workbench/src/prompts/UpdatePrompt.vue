<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { inject } from 'vue';
import UpdateDialog from '../components/UpdateDialog.vue';
import { appUpdateStoreKey } from '../state/createAppUpdateStore';

// The routed `update` popup. Later, Escape and the frame's close all go through store.later(),
// which also dismisses the router entry.
defineProps<{ entry: RoutedPrompt; more: number }>();
const store = inject(appUpdateStoreKey);
if (!store) throw new Error('appUpdateStoreKey is not provided');
</script>

<template>
  <UpdateDialog v-if="store.targetVersion !== ''" :store="store" />
</template>
