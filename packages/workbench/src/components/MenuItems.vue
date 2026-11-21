<script setup lang="ts">
// Renders workbench MenuItem[] as DropdownMenu entries: the one renderer behind the point-anchored
// context menu and every anchored action dropdown. The caller owns the surrounding
// DropdownMenu/DropdownMenuContent.
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import { colorMarkClass } from '@theme/connColor';
import { formatShortcut } from '@theme/shortcutText';
import type { TextPart } from '@theme/varText';
import { useId } from 'vue';
import type { MenuItem, MenuRadioOption } from '../state/contextMenu';
import MenuHint from './MenuHint.vue';
import MenuRowLabel from './MenuRowLabel.vue';

defineProps<{ items: readonly MenuItem[] }>();

// A disabled item takes no pointer events, so its hint needs them back to show on hover.
const HINT_WHEN_DISABLED = 'data-disabled:pointer-events-auto';
// A two-line (detail) item outgrows the fixed item height.
const DETAIL_ROW = 'h-auto py-1';
const uid = useId();

function hintParts(hint: string | readonly TextPart[] | undefined): readonly TextPart[] {
  return typeof hint === 'string' ? [hint] : (hint ?? []);
}

function keyOf(item: MenuItem, idx: number): string {
  return item.type === 'separator' || item.type === 'label' ? `${item.type}-${idx}` : item.id;
}

function tidOf(item: { id: string; testId?: string }): string {
  return item.testId ?? `menu-item-${item.id}`;
}

function swatchClass(swatch: string): string {
  return colorMarkClass('swatch', swatch === 'none' ? null : swatch);
}

// The radio group's model is a string; the "none" option (value null) maps to this.
const NONE = '__none';

function radioKey(value: string | null): string {
  return value ?? NONE;
}

async function onOptionSelect(opt: MenuRadioOption, current: string | null): Promise<void> {
  if (opt.disabled || (opt.value === current && !opt.rerun)) return;
  await opt.run();
}

async function onCheckboxSelect(
  ev: Event,
  item: Extract<MenuItem, { type: 'checkbox' }>,
): Promise<void> {
  if (item.keepOpen) ev.preventDefault();
  if (item.disabled) return;
  await item.run();
}

async function onItemClick(item: MenuItem): Promise<void> {
  if (item.type !== 'item' || item.disabled) return;
  await item.run();
}
</script>

<template>
  <template v-for="(item, idx) in items" :key="keyOf(item, idx)">
    <DropdownMenuSeparator v-if="item.type === 'separator'" />

    <DropdownMenuLabel v-else-if="item.type === 'label'">{{ item.label }}</DropdownMenuLabel>

    <DropdownMenuSub v-else-if="item.type === 'submenu'">
      <DropdownMenuSubTrigger :data-testid="tidOf(item)">
        <span class="flex items-center justify-center shrink-0 size-4">
          <CodiconIcon
            v-if="item.icon"
            :name="item.icon"
            :size="13"
            class="text-muted-foreground"
          />
        </span>
        <span class="flex-1 min-w-0 truncate">{{ item.label }}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuPortal>
        <DropdownMenuSubContent data-testid="context-submenu">
          <MenuItems :items="item.items" />
        </DropdownMenuSubContent>
      </DropdownMenuPortal>
    </DropdownMenuSub>

    <DropdownMenuRadioGroup
      v-else-if="item.type === 'radio'"
      :model-value="radioKey(item.value)"
      :data-testid="`menu-radio-${item.id}`"
    >
      <template
        v-for="(opt, oidx) in item.items"
        :key="opt.type === 'separator' ? `separator-${oidx}` : opt.id"
      >
        <DropdownMenuSeparator v-if="opt.type === 'separator'" />
        <Tooltip v-else :disabled="!opt.hint">
          <TooltipTrigger as-child>
            <DropdownMenuRadioItem
              :value="radioKey(opt.value)"
              :disabled="!!opt.disabled"
              :class="[opt.hint && HINT_WHEN_DISABLED, opt.detail && DETAIL_ROW]"
              :aria-describedby="opt.hint ? `${uid}-${opt.id}-hint` : undefined"
              :data-testid="tidOf(opt)"
              :data-value="opt.value ?? ''"
              @select="onOptionSelect(opt, item.value)"
            >
              <span class="flex items-center justify-center shrink-0 size-4">
                <span
                  v-if="opt.swatch"
                  data-testid="menu-swatch"
                  :class="swatchClass(opt.swatch)"
                />
              </span>
              <MenuRowLabel :label="opt.label" :parts="opt.parts" :detail="opt.detail" />
              <MenuHint
                v-if="opt.hint"
                :id="`${uid}-${opt.id}-hint`"
                :test-id="`${tidOf(opt)}-reason`"
                :hint="opt.hint"
              />
            </DropdownMenuRadioItem>
          </TooltipTrigger>
          <TooltipContent v-if="opt.hint"><VarText :parts="hintParts(opt.hint)" /></TooltipContent>
        </Tooltip>
      </template>
    </DropdownMenuRadioGroup>

    <Tooltip v-else-if="item.type === 'checkbox'" :disabled="!item.hint">
      <TooltipTrigger as-child>
        <DropdownMenuCheckboxItem
          :model-value="item.checked"
          :disabled="!!item.disabled"
          :class="item.hint && HINT_WHEN_DISABLED"
          :aria-describedby="item.hint ? `${uid}-${item.id}-hint` : undefined"
          :data-testid="tidOf(item)"
          @select="onCheckboxSelect($event, item)"
        >
          <span class="flex items-center justify-center shrink-0 size-4">
            <CodiconIcon
              v-if="item.icon"
              :name="item.icon"
              :size="13"
              class="text-muted-foreground"
            />
          </span>
          <MenuRowLabel :label="item.label" />
          <MenuHint
            v-if="item.hint"
            :id="`${uid}-${item.id}-hint`"
            :test-id="`${tidOf(item)}-reason`"
            :hint="item.hint"
          />
        </DropdownMenuCheckboxItem>
      </TooltipTrigger>
      <TooltipContent v-if="item.hint"><VarText :parts="hintParts(item.hint)" /></TooltipContent>
    </Tooltip>

    <Tooltip v-else :disabled="!item.hint">
      <TooltipTrigger as-child>
        <DropdownMenuItem
          :disabled="!!item.disabled"
          :variant="item.danger ? 'danger' : 'default'"
          :class="[item.hint && HINT_WHEN_DISABLED, item.detail && DETAIL_ROW]"
          :aria-describedby="item.hint ? `${uid}-${item.id}-hint` : undefined"
          :data-testid="tidOf(item)"
          @select="onItemClick(item)"
        >
          <span class="flex items-center justify-center shrink-0 size-4">
            <span v-if="item.swatch" data-testid="menu-swatch" :class="swatchClass(item.swatch)" />
            <CodiconIcon
              v-else-if="item.icon"
              :name="item.icon"
              :size="13"
              :class="item.danger ? undefined : 'text-muted-foreground'"
            />
          </span>
          <MenuRowLabel :label="item.label" :parts="item.parts" :detail="item.detail" />
          <MenuHint
            v-if="item.hint"
            :id="`${uid}-${item.id}-hint`"
            :test-id="`${tidOf(item)}-reason`"
            :hint="item.hint"
          />
          <DropdownMenuShortcut
            v-if="item.shortcut"
            :data-testid="`${tidOf(item)}-shortcut`"
            :data-shortcut="item.shortcut"
            >{{
              formatShortcut(item.shortcut)
            }}</DropdownMenuShortcut
          >
        </DropdownMenuItem>
      </TooltipTrigger>
      <TooltipContent v-if="item.hint"><VarText :parts="hintParts(item.hint)" /></TooltipContent>
    </Tooltip>
  </template>
</template>
