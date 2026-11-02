<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { errorText } from '@workbench/bridge/codedError';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import { useAdeSettingsUiStore } from '../state/adeSettingsUi';
import AdeSettingsResult from './AdeSettingsResult.vue';
import AdeSettingsSecrets from './AdeSettingsSecrets.vue';
import { useAgentSettings, useSaveAgentSettings } from './queries';

// The agent settings: Markdown every agent Kira starts gets as extra system prompt. The draft is the
// source while it has unsaved edits, so a push never resets it. Save sends the hash of the text the
// draft was edited from; a file changed elsewhere is refused with Reload and Overwrite.
const setUi = useAdeSettingsUiStore();
const query = useAgentSettings();
const save = useSaveAgentSettings();

const draft = ref('');
const dirty = ref(false);
const baseHash = ref('');
const saveError = ref('');
const conflict = ref(false);

const view = computed(() => query.data.value);
const bytes = computed(() => new TextEncoder().encode(draft.value).length);
const actionable = computed(() => dirty.value && !save.isPending.value);
const over = computed(() => view.value !== undefined && bytes.value > view.value.maxBytes);

watch(
  view,
  (v) => {
    if (!v || dirty.value) return;
    draft.value = v.text;
    baseHash.value = v.hash;
  },
  { immediate: true },
);
watch(dirty, (v) => {
  setUi.dirty = v;
});

async function saveNow(): Promise<void> {
  if (!dirty.value) return;
  const text = draft.value;
  try {
    const next = await save.mutateAsync({ text, baseHash: baseHash.value });
    saveError.value = '';
    conflict.value = false;
    baseHash.value = next.hash;
    if (draft.value === text) dirty.value = false;
  } catch (err) {
    saveError.value = errorText(err);
    conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
  }
}

function resetTo(v: { text: string; hash: string } | undefined): void {
  draft.value = v?.text ?? '';
  baseHash.value = v?.hash ?? '';
  dirty.value = false;
  saveError.value = '';
  conflict.value = false;
}

function discard(): void {
  resetTo(view.value);
}

async function reload(): Promise<void> {
  resetTo((await query.refetch()).data);
}

async function overwrite(): Promise<void> {
  const fresh = (await query.refetch()).data;
  if (!fresh) return;
  baseHash.value = fresh.hash;
  conflict.value = false;
  await saveNow();
}

function onInput(v: string | number): void {
  draft.value = String(v);
  dirty.value = true;
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());

onBeforeUnmount(() => {
  setUi.dirty = false;
});
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-settings">
    <div :class="cn(paneFrame, 'flex flex-1 flex-col')">
      <PanelHeader>{{ t('common.module.settings.label') }}</PanelHeader>
      <div class="min-h-0 flex-1 overflow-auto p-4">
        <div v-if="view" ref="root" class="grid grid-cols-1 items-start gap-6 xl:grid-cols-2">
          <div class="flex min-w-0 flex-col gap-3">
            <ConfigFileBar
              v-if="view.exists"
              :path="view.path"
              :error="view.problem"
              :open="() => control.configFilesOpen({ folder: view!.folder, file: view!.file })"
              :reveal="() => control.configFilesReveal({ folder: view!.folder, file: view!.file })"
              :about="() => control.configFilesOpen({ folder: view!.folder, file: 'AGENTS.md' })"
            />
            <div v-else class="text-kira-sm text-subtle" data-testid="ade-settings-none">
              {{ t('space.ade.settings.none.hint', { path: view.path }) }}
            </div>
            <div
              v-if="view.lastGood"
              class="text-kira-sm text-subtle"
              data-testid="ade-settings-last-good"
            >
              {{ t('space.ade.context.lastGood.hint') }}
            </div>
            <div
              class="flex flex-col gap-1 text-kira-sm text-subtle"
              data-testid="ade-settings-note"
            >
              <span>{{ t('space.ade.settings.applies.hint') }}</span>
            </div>
            <div
              v-if="!view.editable"
              class="text-kira-sm text-subtle"
              data-testid="ade-settings-read-only"
            >
              {{ t('space.ade.context.readOnly.hint') }}
            </div>
            <template v-else>
              <Label for="ade-settings-text" class="sr-only">{{
                t('space.ade.settings.field.aria')
              }}</Label>
              <Textarea
                id="ade-settings-text"
                :model-value="draft"
                spellcheck="false"
                class="min-h-64 resize-y font-data leading-relaxed"
                data-testid="ade-settings-text"
                @update:model-value="onInput"
              />
              <div class="flex flex-wrap items-center gap-2">
                <span
                  class="min-w-0 flex-1 text-kira-sm"
                  :class="over ? 'text-tone-red' : 'text-subtle'"
                  data-testid="ade-settings-size"
                >
                  {{
                    over
                      ? t('space.ade.settings.overSize.error', { max: view.maxBytes })
                      : t('space.ade.context.size.status', { used: bytes, max: view.maxBytes })
                  }}
                </span>
                <Button
                  variant="dialog"
                  size="kira-lg"
                  :disabled="!actionable"
                  data-testid="ade-settings-discard"
                  @click="discard"
                >
                  {{ t('common.action.discard.label') }}
                </Button>
                <Button
                  variant="dialog-primary"
                  size="kira-lg"
                  :disabled="!dirty"
                  :pending="save.isPending.value"
                  data-testid="ade-settings-save"
                  @click="saveNow"
                >
                  {{ t('common.action.save.label') }}
                </Button>
              </div>
            </template>
            <Alert v-if="saveError" variant="error" data-testid="ade-settings-save-error">
              <AlertDescription>{{ saveError }}</AlertDescription>
              <AlertAction v-if="conflict">
                <Button
                  variant="link"
                  size="inline"
                  data-testid="ade-settings-reload"
                  @click="reload"
                >
                  {{ t('common.action.refresh.label') }}
                </Button>
                <Button
                  variant="link"
                  size="inline"
                  data-testid="ade-settings-overwrite"
                  @click="overwrite"
                >
                  {{ t('space.ade.workflows.conflict.overwrite.label') }}
                </Button>
              </AlertAction>
            </Alert>
            <AdeSettingsSecrets :view="view" />
          </div>
          <AdeSettingsResult :providers="view.providers" />
        </div>
      </div>
    </div>
  </div>
</template>
