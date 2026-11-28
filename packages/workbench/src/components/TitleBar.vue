<script setup lang="ts">
// Kira Studio's own workbench/TitleBar.vue and Kira Space's, unified — the bar
// chrome only: height, insets, background, the `--wails-draggable: drag` region this root carries
// (drag.ts's own doc comment — dragging the bar moves the window, inert everywhere without a real
// Wails window). Every interactive child (mode tabs, action buttons) is per-app content, passed
// through the default slot. The old title bar actions wrapper class and title action
// button class (plus its labelled variant and its is-on toggle class) are gone from
// `workbench.css` — each app's own action row is now plain Tailwind utilities plus
// `<Button variant="title" size="title">` (packages/theme/src/components/ui/button), with
// `aria-pressed` replacing the old class toggle.
//
// The Settings mount: each app's own `<SettingsDialog v-if="settingsStore.settingsOpen"
// @close="..."> stays entirely in the app (the settings store's own shape is per-app) — this
// component owns only the `<Teleport to="body">` wrapper around whatever the `#settings` slot
// renders, so "Settings teleports to body, not the title bar's own stacking context" is one
// shared fact instead of two identical Teleport calls.
//
// Two roots, so `class` does not fall through: the `class` prop lands on the bar (a task band passes
// `bg-transparent`).
import { cn } from '@theme/lib/utils';

defineProps<{ class?: string }>();
</script>

<template>
  <!-- wails-drag carries --wails-draggable: drag so the bar behaves like a native title
       bar -- dragging it moves the window, double-clicking it zooms/minimises per System Settings,
       both for free. Inert (ordinary DOM) on Linux dev builds and under tests/ui (a static file
       server, no Wails window at all). -->
  <div
    :class="
      cn(
        'relative flex items-center shrink-0 wails-drag h-titlebar min-h-titlebar pl-titlebar-inset pr-1.5 bg-chrome',
        $props.class,
      )
    "
  >
    <slot />
  </div>
  <Teleport to="body">
    <slot name="settings" />
  </Teleport>
</template>
