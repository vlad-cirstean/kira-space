<script setup lang="ts">
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { computed, useTemplateRef } from 'vue';
import { control } from '../bridge/control';
import { renderMarkdown } from './markdown';

// Host-written Markdown, rendered without raw HTML (markdown.ts). A click never navigates the app's
// own window: an http(s) link opens in the OS browser, anything else does nothing.
const props = defineProps<{ source: string }>();
const html = computed(() => renderMarkdown(props.source));
const root = useTemplateRef('root');

useEventListener(root, 'click', (e: MouseEvent) => {
  const anchor = (e.target as Element | null)?.closest('a');
  if (!anchor) return;
  e.preventDefault();
  const href = anchor.getAttribute('href') ?? '';
  if (/^https?:\/\//i.test(href)) fireAndForget(control.openExternal(href), 'hostView.markdown');
});
</script>

<template>
  <div
    v-if="source.trim()"
    ref="root"
    class="kira-prose kira-prose-host"
    data-testid="host-view-markdown"
    v-html="html"
  />
  <FieldDescription v-else data-testid="host-view-no-description">{{
    t('space.hostView.description.empty.empty')
  }}</FieldDescription>
</template>
