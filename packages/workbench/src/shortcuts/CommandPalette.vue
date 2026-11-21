<script setup lang="ts">
import {
  CommandDialog,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from '@theme/components/ui/command';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { type PaletteCommand, usePaletteStore } from './palette';

// The shared command palette. CommandDialog (reka-ui underneath) supplies the dialog role, focus
// trap, restore, Escape and outside-click close. Commands come from the app via
// `providePaletteCommands`; the app passes its own placeholder and empty copy.
defineProps<{ placeholder: string; emptyLabel: string }>();

const paletteStore = usePaletteStore();

function runCommand(id: string): void {
  const command = paletteStore.commands.find((c) => c.id === id);
  if (!command) return;
  paletteStore.closePalette();
  if (command.movesFocus) pending = command;
  else command.run();
}

// A focus-moving command runs once the dialog has restored (or skipped restoring) focus.
let pending: PaletteCommand | null = null;
function onCloseAutoFocus(event: Event): void {
  if (!pending) return;
  event.preventDefault();
  const command = pending;
  pending = null;
  command.run();
}

function onOpenChange(open: boolean): void {
  if (!open) paletteStore.closePalette();
}
</script>

<template>
  <CommandDialog
    :open="paletteStore.open"
    class="w-105"
    @update:open="onOpenChange"
    @close-auto-focus="onCloseAutoFocus"
  >
    <CommandInput
      data-testid="command-palette-input"
      :placeholder="placeholder"
      @keydown="wrapSelectionOnType"
    />
    <CommandList class="max-h-72">
      <!-- CommandEmpty shows only for a search with no match; an app with no command at all (the
           palette lists only what has no other entry point) needs the copy without a search. -->
      <p
        v-if="paletteStore.commands.length === 0"
        class="py-4 text-center text-kira-md text-subtle"
        data-testid="command-palette-empty"
      >
        {{ emptyLabel }}
      </p>
      <CommandEmpty v-else class="text-subtle" data-testid="command-palette-empty">{{
        emptyLabel
      }}</CommandEmpty>
      <CommandItem
        v-for="command in paletteStore.commands"
        :key="command.id"
        :value="command.id"
        class="whitespace-nowrap"
        data-testid="command-palette-item"
        :data-command-id="command.id"
        @select="runCommand(command.id)"
      >
        {{ command.label }}
      </CommandItem>
    </CommandList>
  </CommandDialog>
</template>
