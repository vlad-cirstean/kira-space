<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Collapsible, CollapsibleTrigger } from '@theme/components/ui/collapsible';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { Skeleton } from '@theme/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import QueryState from '@workbench/components/QueryState.vue';
import { isPrimaryModifier, primaryModifierName } from '@workbench/shortcuts/keys';
import { RovingFocusGroup, RovingFocusItem } from 'reka-ui';
import { nextTick, ref, useId, useTemplateRef, watch } from 'vue';
import { gapText } from '../artifacts/gapText';
import type { Task } from '../wire';
import AdeLinkAdd from './AdeLinkAdd.vue';
import AdeLinkCard from './AdeLinkCard.vue';
import AdeLinkPill from './AdeLinkPill.vue';
import { CATEGORY_ICON, CATEGORY_LABEL, type LinkItem } from './linkGroups';
import { useTaskLinks } from './useTaskLinks';

// The task's links as one strip: a line per non-empty category (Issues, Change requests, Misc) of 18px
// pills. A pill click opens the link in the app (host view) where it can, else in the browser;
// the primary modifier plus click opens it in the browser. Right-click, Space or Alt+Down open its card; the `+` opens the add popover. `edit` changes links (Details), `add` only
// adds (handoff dialogs), `view` is read-only. Pills share one tab stop; arrows move between them.
const props = defineProps<{
  task: Task;
  mode: 'edit' | 'view' | 'add';
  /** Include detected branches in Misc (a Done move). */
  branches?: boolean;
  /** Archived Details: the session button hands the id up instead of selecting. */
  archived?: boolean;
}>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();

const SHOWN = 7;
const OVER = 8;

const links = useTaskLinks(() => props.task, { branches: () => props.branches ?? false });
const root = useTemplateRef<HTMLElement>('root');
const adder = useTemplateRef<{ file: (raw: string) => void }>('adder');
const openKey = ref<string | null>(null);
const editingKey = ref<string | null>(null);
const addOpen = ref(false);
const hintId = useId();
const modifier = primaryModifierName();

const canAdd = () => props.mode !== 'view';
const canEdit = (item: LinkItem) => props.mode === 'edit' && item.origin !== 'detected';

function openCard(item: LinkItem): void {
  setOpen(item.key, true);
}

// Opens the link, else its card: nothing is left to do for a link with no target.
function openItem(e: MouseEvent | KeyboardEvent, item: LinkItem): void {
  const opened = links.openLink(item, {
    external: isPrimaryModifier(e),
    onRefused: () => openCard(item),
  });
  if (!opened) openCard(item);
}

// No trigger returns focus on close: put it back on the pill, unless a pointer outside closed it.
function onCardClose(e: Event, item: LinkItem): void {
  if (e.defaultPrevented) return;
  e.preventDefault();
  root.value?.querySelector<HTMLElement>(`[data-key="${CSS.escape(item.key)}"]`)?.focus();
}

function setOpen(key: string, value: boolean): void {
  if (value) {
    openKey.value = key;
    editingKey.value = null;
  } else if (openKey.value === key) {
    openKey.value = null;
    editingKey.value = null;
  }
}

const pills = (): HTMLElement[] =>
  Array.from(root.value?.querySelectorAll<HTMLElement>('[data-testid="ade-links-pill"]') ?? []);

// A removed link leaves a gap: focus moves to the pill that takes its place, else the `+`.
let focusAfter: number | null = null;
watch(
  () => links.groups.value,
  async (groups) => {
    const keys = new Set(groups.flatMap((g) => g.items.map((i) => i.key)));
    if (openKey.value !== null && !keys.has(openKey.value)) {
      openKey.value = null;
      editingKey.value = null;
    }
    if (focusAfter === null) return;
    const at = focusAfter;
    focusAfter = null;
    await nextTick();
    const all = pills();
    (
      all[Math.min(at, all.length - 1)] ??
      root.value?.querySelector<HTMLElement>('[data-testid="ade-links-add"]')
    )?.focus();
  },
);

async function removeFocused(item: LinkItem, el: HTMLElement): Promise<void> {
  focusAfter = pills().indexOf(el);
  await links.remove(item);
  if (links.error.value) focusAfter = null;
}

const itemOf = (el: HTMLElement): LinkItem | undefined =>
  links.groups.value.flatMap((g) => g.items).find((i) => i.key === el.dataset.key);

// One handler for the strip: pill keys (open, edit, remove, card), `+` and a line jump on Up/Down.
// Left/Right/Home/End are the roving group's.
function onKeydown(e: KeyboardEvent): void {
  if (e.isComposing || e.defaultPrevented) return;
  const el = (e.target as HTMLElement).closest<HTMLElement>('[data-testid="ade-links-pill"]');
  if (!el) return;
  const item = itemOf(el);
  if (!item) return;
  if (e.key === 'Enter') {
    e.preventDefault();
    openItem(e, item);
  } else if (e.key === ' ') {
    e.preventDefault();
    openCard(item);
  } else if (e.key === 'F2' && canEdit(item)) {
    e.preventDefault();
    openKey.value = item.key;
    editingKey.value = item.key;
  } else if ((e.key === 'Delete' || e.key === 'Backspace') && canEdit(item)) {
    e.preventDefault();
    void removeFocused(item, el);
  } else if (e.key === 'ArrowDown' && e.altKey) {
    e.preventDefault();
    openCard(item);
  } else if (e.key === '+' && canAdd()) {
    e.preventDefault();
    addOpen.value = true;
  } else if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !e.altKey) {
    const lines = Array.from(
      root.value?.querySelectorAll<HTMLElement>('[data-testid="ade-links-line"]') ?? [],
    );
    const at = lines.findIndex((l) => l.contains(el));
    const next = lines[at + (e.key === 'ArrowDown' ? 1 : -1)];
    if (!next) return;
    e.preventDefault();
    next.querySelector<HTMLElement>('[data-testid="ade-links-pill"]')?.focus();
  }
}

useEventListener(root, 'paste', (e: ClipboardEvent) => {
  const text = e.clipboardData?.getData('text') ?? '';
  const onPill = (e.target as HTMLElement).closest('[data-testid="ade-links-pill"]') !== null;
  if (!canAdd() || !onPill || text.trim() === '') return;
  e.preventDefault();
  adder.value?.file(text);
});
</script>

<template>
  <div ref="root" class="flex flex-col gap-0.5" data-testid="ade-links" :data-mode="mode">
    <div class="flex h-control items-center gap-1 text-kira-sm text-muted-foreground">
      <span class="flex-1" data-testid="ade-links-title">{{
        t('space.ade.details.links.title')
      }}</span>
      <AdeLinkAdd v-if="canAdd()" ref="adder" v-model:open="addOpen" :add="links.add">
        <TooltipIconButton
          icon="add"
          :label="t('space.ade.details.addLine.aria')"
          class="shrink-0"
          data-testid="ade-links-add"
        />
      </AdeLinkAdd>
    </div>
    <QueryState :query="links.query" variant="inline">
      <template #loading>
        <div class="flex min-h-6 items-center gap-1" data-testid="ade-links-loading">
          <Skeleton
            v-for="n in 3"
            :key="n"
            class="h-4.5 w-16 rounded-kira-sm"
            data-testid="ade-links-skeleton"
          />
        </div>
      </template>
      <template #error="{ message, retry }">
        <div class="flex min-h-6 items-center gap-1" data-testid="query-error">
          <FieldError class="min-w-0 flex-1 truncate">{{ message }}</FieldError>
          <Button size="kira" variant="toolbar" data-testid="query-retry" @click="retry">{{
            t('common.action.retry.label')
          }}</Button>
        </div>
      </template>
      <p
        v-if="links.gaps.value.length"
        class="m-0 text-kira-sm text-tone-amber"
        data-testid="ade-links-gaps"
      >
        <span
          v-for="(g, i) in links.gaps.value"
          :key="`${g.source}:${g.repo}:${g.reason ?? ''}`"
          data-testid="ade-links-gap"
          >{{ gapText(g) }}{{ i < links.gaps.value.length - 1 ? ' ' : '' }}</span
        >
      </p>
      <FieldError v-if="links.error.value" data-testid="ade-links-error">{{
        links.error.value
      }}</FieldError>
      <FieldDescription v-if="links.groups.value.length === 0" data-testid="ade-links-empty">
        {{ t('space.ade.details.empty.body')
        }}<template v-if="canAdd()"> {{ t('space.ade.links.empty.hint') }}</template>
      </FieldDescription>
      <span
        v-if="links.groups.value.length"
        :id="hintId"
        class="sr-only"
        data-testid="ade-links-hint"
        >{{
          t('space.ade.links.pill.hint', { modifier })
        }}</span
      >
      <RovingFocusGroup
        orientation="horizontal"
        loop
        class="flex flex-col gap-0.5"
        @keydown="onKeydown"
      >
        <div
          v-for="g in links.groups.value"
          :key="g.id"
          class="flex min-h-6 items-start gap-1"
          data-testid="ade-links-line"
          :data-category="g.id"
        >
          <Tooltip>
            <TooltipTrigger as-child>
              <span
                class="flex min-h-6 w-5 shrink-0 items-center justify-center text-muted-foreground"
                role="img"
                :aria-label="
                  t('space.ade.links.category.tooltip', {
                    category: t(CATEGORY_LABEL[g.id]),
                    count: g.items.length,
                  })
                "
                :data-category="g.id"
                data-testid="ade-links-category"
              >
                <CodiconIcon :name="CATEGORY_ICON[g.id]" :size="13" />
              </span>
            </TooltipTrigger>
            <TooltipContent>{{
              t('space.ade.links.category.tooltip', {
                category: t(CATEGORY_LABEL[g.id]),
                count: g.items.length,
              })
            }}</TooltipContent>
          </Tooltip>
          <Collapsible v-slot="{ open }" class="flex min-w-0 flex-1 flex-wrap gap-1 py-[3px]">
            <template v-for="(item, i) in g.items" :key="item.key">
              <Popover
                v-if="open || g.items.length <= OVER || i < SHOWN"
                :open="openKey === item.key"
                @update:open="(v: boolean) => setOpen(item.key, v)"
              >
                <RovingFocusItem as-child>
                  <PopoverAnchor as-child>
                    <AdeLinkPill
                      :item="item"
                      :show-kind="g.id === 'misc'"
                      aria-haspopup="dialog"
                      :aria-expanded="openKey === item.key"
                      :aria-describedby="hintId"
                      :data-state="openKey === item.key ? 'open' : 'closed'"
                      @click="openItem($event, item)"
                      @contextmenu.prevent.stop="openCard(item)"
                    />
                  </PopoverAnchor>
                </RovingFocusItem>
                <PopoverContent
                  align="start"
                  class="w-80"
                  data-testid="ade-links-card"
                  @close-auto-focus="onCardClose($event, item)"
                >
                  <AdeLinkCard
                    :task="task"
                    :item="item"
                    :mode="mode"
                    :archived="archived"
                    :editing="editingKey === item.key"
                    :links="links"
                    @edit="editingKey = item.key"
                    @done="editingKey = null"
                    @open-session="emit('openSession', $event)"
                  />
                </PopoverContent>
              </Popover>
            </template>
            <RovingFocusItem v-if="!open && g.items.length > OVER" as-child>
              <CollapsibleTrigger as-child>
                <button
                  type="button"
                  class="inline-flex h-control-sm items-center rounded-kira-sm bg-field px-1.5 text-kira-sm text-muted-foreground focus-visible:focus-ring"
                  data-testid="ade-links-more"
                >
                  {{ t('common.more.label', { count: g.items.length - SHOWN }) }}
                </button>
              </CollapsibleTrigger>
            </RovingFocusItem>
          </Collapsible>
        </div>
      </RovingFocusGroup>
    </QueryState>
  </div>
</template>
