<script setup lang="ts">
import Disclosure from '@theme/components/Disclosure.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { errorText } from '@workbench/bridge/codedError';
import OpLogCodeBlock from '@workbench/components/OpLogCodeBlock.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref, watch } from 'vue';
import AdePortResult from './AdePortResult.vue';
import { useCancelPort, useImportApply, useImportPreview } from './queries';
import type { ImportPreview, PortItem, PortProvider, PortResult } from './types';

// Reads the provider's own files on open and lists what an import would bring in. Secret values never
// reach this page: a slot shows its name and ••••.
const props = defineProps<{ provider: PortProvider }>();
const open = defineModel<boolean>('open', { required: true });

const previewMutation = useImportPreview();
const applyMutation = useImportApply();
const cancelMutation = useCancelPort();

const preview = ref<ImportPreview | null>(null);
const picked = ref<Set<string>>(new Set());
const modes = ref<Record<string, string>>({});
const error = ref('');
const result = ref<PortResult | null>(null);
const scrollEl = ref<HTMLElement | null>(null);

function snippetLanguage(id: string): 'yaml' | 'markdown' {
  return id === 'context' ? 'markdown' : 'yaml';
}

const items = computed(() => preview.value?.items ?? []);
const selectable = (item: PortItem): boolean => item.state === 'new' || item.state === 'conflict';
const defaults = computed(
  () => new Set(items.value.filter((i) => i.state === 'new').map((i) => i.id)),
);
const dirty = computed(
  () =>
    result.value === null &&
    (picked.value.size !== defaults.value.size ||
      [...picked.value].some((id) => !defaults.value.has(id))),
);

const { single: rowHeight } = useRowHeight();
const { virtualizer, virtualItems, totalSize, onScroll } = useVirtualRows({
  count: () => items.value.length,
  rowHeight: () => rowHeight.value * 3,
  scrollElement: scrollEl,
});

function measureRef(el: unknown): void {
  if (el instanceof HTMLElement) virtualizer.value.measureElement(el);
}

const rows = computed(() =>
  virtualItems.value.flatMap((vi) => {
    const item = items.value[vi.index];
    return item ? [{ vi, item }] : [];
  }),
);

async function load(): Promise<void> {
  error.value = '';
  result.value = null;
  try {
    const p = await previewMutation.mutateAsync(props.provider.id);
    preview.value = p;
    picked.value = new Set((p.items ?? []).filter((i) => i.state === 'new').map((i) => i.id));
    modes.value = Object.fromEntries(
      (p.items ?? []).filter((i) => i.modes?.length).map((i) => [i.id, i.modes?.[0] ?? '']),
    );
  } catch (err) {
    preview.value = null;
    error.value = errorText(err);
  }
}

// A preview is single use: dropping it frees the server's copy.
function release(): void {
  const token = preview.value?.token;
  if (token && result.value === null) cancelMutation.mutate(token);
  preview.value = null;
}

watch(open, (isOpen) => {
  if (isOpen) void load();
  else release();
});

function toggle(item: PortItem, on: boolean): void {
  const next = new Set(picked.value);
  if (on) next.add(item.id);
  else next.delete(item.id);
  picked.value = next;
}

async function submit(): Promise<void> {
  if (!preview.value || picked.value.size === 0) return;
  error.value = '';
  try {
    result.value = await applyMutation.mutateAsync({
      token: preview.value.token,
      items: items.value
        .filter((i) => picked.value.has(i.id))
        .map((i) => ({ id: i.id, mode: modes.value[i.id] ?? '' })),
    });
    preview.value = null;
  } catch (err) {
    error.value = errorText(err);
  }
}

const busy = computed(() => previewMutation.isPending.value || applyMutation.isPending.value);
const stateVariant = { new: 'ok', same: 'default', conflict: 'warn', skipped: 'default' } as const;
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="2xl" fixed-height :dirty="dirty" data-testid="ade-port-import-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('space.ade.port.importDialog.title', { agent: provider.label })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <AdePortResult v-if="result" :result="result" />
        <template v-else>
          <FieldDescription>{{ t('space.ade.port.importDialog.hint') }}</FieldDescription>
          <template v-if="error">
            <Alert variant="error" data-testid="ade-port-error">
              <AlertDescription>{{ error }}</AlertDescription>
            </Alert>
            <div>
              <Button
                variant="dialog"
                size="kira-lg"
                :pending="busy"
                data-testid="ade-port-retry"
                @click="load"
              >
                {{ t('space.ade.port.retry.label') }}
              </Button>
            </div>
          </template>
          <template v-if="preview">
            <ul
              v-if="preview.sources?.length"
              :aria-label="t('space.ade.port.sources.aria')"
              class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
              data-testid="ade-port-sources"
            >
              <li v-for="s in preview.sources" :key="s.role" class="flex items-center gap-2">
                <span class="min-w-0 flex-1 truncate font-data">{{ s.path }}</span>
                <Badge v-if="!s.exists">{{ t('space.ade.port.missing.status') }}</Badge>
                <span v-if="s.problem" class="text-error">{{ s.problem }}</span>
              </li>
            </ul>
            <ul
              v-if="preview.notes?.length"
              class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
            >
              <li v-for="n in preview.notes" :key="n">{{ n }}</li>
            </ul>
            <FieldDescription v-if="items.length === 0" data-testid="ade-port-none">
              {{ t('space.ade.port.items.emptyHint') }}
            </FieldDescription>
            <div
              v-else
              ref="scrollEl"
              class="min-h-0 flex-1 overflow-auto rounded-kira"
              data-testid="ade-port-items"
              @scroll="onScroll"
            >
              <ul
                :aria-label="t('space.ade.port.items.aria')"
                class="relative m-0 list-none p-0"
                :style="{ height: `${totalSize}px` }"
              >
                <li
                  v-for="{ vi, item } in rows"
                  :key="String(vi.key)"
                  :ref="measureRef"
                  :data-index="vi.index"
                  :class="[VIRTUAL_ROW_CLASS, 'flex flex-col gap-1 p-2']"
                  :style="{ transform: `translateY(${vi.start}px)` }"
                  :data-testid="`ade-port-item-${item.id}`"
                  :data-state="item.state"
                >
                  <div class="flex items-center gap-2">
                    <Checkbox
                      :model-value="picked.has(item.id)"
                      :disabled="!selectable(item)"
                      :aria-label="item.id"
                      :data-testid="`ade-port-check-${item.id}`"
                      @update:model-value="(v) => toggle(item, v === true)"
                    />
                    <span class="min-w-0 flex-1 truncate font-data text-kira-md">{{
                      item.id
                    }}</span>
                    <NativeSelect
                      v-if="item.modes?.length && picked.has(item.id)"
                      v-model="modes[item.id]"
                      variant="bordered"
                      size="kira-lg"
                      :aria-label="t('space.ade.port.mode.aria')"
                      :data-testid="`ade-port-mode-${item.id}`"
                    >
                      <option v-for="m in item.modes" :key="m" :value="m">{{
                        m === 'append'
                          ? t('space.ade.port.append.status')
                          : t('space.ade.port.replace.status')
                      }}</option>
                    </NativeSelect>
                    <Badge :variant="stateVariant[item.state]" data-testid="ade-port-state">{{
                      item.state === 'skipped'
                        ? t('common.skipped.status')
                        : t(`space.ade.port.state.${item.state}.status`)
                    }}</Badge>
                  </div>
                  <span v-if="item.reason" class="text-kira-sm text-subtle">{{ item.reason }}</span>
                  <OpLogCodeBlock
                    v-if="item.text && item.state !== 'skipped'"
                    :text="item.text"
                    :language="snippetLanguage(item.id)"
                  />
                  <ul
                    v-if="item.secrets?.length"
                    class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
                  >
                    <li
                      v-for="s in item.secrets"
                      :key="s.name"
                      class="font-data"
                      data-testid="ade-port-secret"
                    >
                      {{ t('space.ade.port.secret.label', { name: s.name }) }}
                      ••••
                    </li>
                  </ul>
                  <Disclosure
                    v-if="item.current"
                    :label="t('space.ade.port.current.label')"
                    content-class="pt-1"
                  >
                    <OpLogCodeBlock :text="item.current" :language="snippetLanguage(item.id)" />
                  </Disclosure>
                </li>
              </ul>
            </div>
          </template>
        </template>
      </DialogBody>
      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="ade-port-close" @click="open = false">
          {{ t('common.action.close.label') }}
        </Button>
        <Button
          v-if="!result"
          variant="dialog-primary"
          size="kira-lg"
          :disabled="picked.size === 0"
          :pending="busy"
          data-testid="ade-port-apply"
          @click="submit"
        >
          {{ t('space.ade.port.importApply.label', { count: picked.size }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
