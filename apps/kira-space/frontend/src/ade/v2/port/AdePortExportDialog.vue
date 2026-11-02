<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
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
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import MonacoDiff from '@workbench/editor/MonacoDiff.vue';
import { computed, ref, watch } from 'vue';
import AdePortResult from './AdePortResult.vue';
import { useCancelPort, useExportApply, useExportPreview } from './queries';
import type { ExportPreview, PortProvider, PortResult } from './types';

// Renders Kira's settings and context as the provider's files and shows each as a diff. Credential
// values are masked in the diff and never in the env lines: those carry names only.
const props = defineProps<{ provider: PortProvider }>();
const open = defineModel<boolean>('open', { required: true });

const previewMutation = useExportPreview();
const applyMutation = useExportApply();
const cancelMutation = useCancelPort();
const { copy } = useCopy();

const preview = ref<ExportPreview | null>(null);
const roles = ref<Set<string>>(new Set());
const servers = ref(true);
const error = ref('');
const result = ref<PortResult | null>(null);

const files = computed(() => preview.value?.files ?? []);
const serverList = computed(() => preview.value?.servers ?? []);
const envLines = computed(() =>
  (preview.value?.envNames ?? []).map((n) => `export ${n}=`).join('\n'),
);
const nothing = computed(
  () => roles.value.size === 0 && !(servers.value && serverList.value.length),
);

async function load(): Promise<void> {
  error.value = '';
  result.value = null;
  try {
    const p = await previewMutation.mutateAsync(props.provider.id);
    preview.value = p;
    roles.value = new Set((p.files ?? []).filter((f) => f.changed).map((f) => f.role));
    servers.value = true;
  } catch (err) {
    preview.value = null;
    error.value = errorText(err);
  }
}

function release(): void {
  const token = preview.value?.token;
  if (token && result.value === null) cancelMutation.mutate(token);
  preview.value = null;
}

watch(open, (isOpen) => {
  if (isOpen) void load();
  else release();
});

function toggle(role: string, on: boolean): void {
  const next = new Set(roles.value);
  if (on) next.add(role);
  else next.delete(role);
  roles.value = next;
}

async function submit(): Promise<void> {
  if (!preview.value) return;
  error.value = '';
  try {
    result.value = await applyMutation.mutateAsync({
      token: preview.value.token,
      files: files.value.filter((f) => roles.value.has(f.role)).map((f) => f.role),
      servers: servers.value,
    });
    preview.value = null;
  } catch (err) {
    error.value = errorText(err);
  }
}

const busy = computed(() => previewMutation.isPending.value || applyMutation.isPending.value);
const language = (l: string): EditorLanguageId => (l === 'json' ? 'json' : 'plain');
const fileName = (path: string): string => path.split(/[\\/]/).pop() ?? path;
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent size="2xl" fixed-height data-testid="ade-port-export-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('space.ade.port.exportDialog.title', { agent: provider.label })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <AdePortResult v-if="result" :result="result" />
        <template v-else>
          <FieldDescription>{{ t('space.ade.port.exportDialog.hint') }}</FieldDescription>
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
            <Alert v-if="preview.cliMissing" variant="warn" data-testid="ade-port-cli-missing">
              <AlertDescription>{{ t('space.ade.port.cliMissing.hint') }}</AlertDescription>
            </Alert>
            <section class="flex flex-col gap-2" data-testid="ade-port-files">
              <h3 class="m-0 text-kira-lg font-medium">{{ t('common.module.files.label') }}</h3>
              <div
                v-for="f in files"
                :key="f.role"
                class="flex flex-col gap-1"
                :data-testid="`ade-port-file-${f.role}`"
              >
                <div class="flex items-center gap-2">
                  <Checkbox
                    :model-value="roles.has(f.role)"
                    :disabled="!f.changed"
                    :aria-label="fileName(f.path)"
                    :data-testid="`ade-port-file-check-${f.role}`"
                    @update:model-value="(v) => toggle(f.role, v === true)"
                  />
                  <span class="min-w-0 flex-1 truncate font-data text-kira-md">{{ f.path }}</span>
                  <Badge :variant="f.changed ? 'warn' : 'default'">{{
                    !f.changed
                      ? t('space.ade.port.file.unchanged.status')
                      : f.exists
                        ? t('common.changed.status')
                        : t('space.ade.port.file.new.status')
                  }}</Badge>
                </div>
                <Disclosure v-if="f.changed" :label="fileName(f.path)" content-class="pt-1">
                  <div class="h-60">
                    <MonacoDiff
                      :original="f.before"
                      :modified="f.after"
                      :language="language(f.language)"
                    />
                  </div>
                </Disclosure>
              </div>
            </section>
            <section
              v-if="serverList.length"
              class="flex flex-col gap-1"
              data-testid="ade-port-server-list"
            >
              <Label class="flex items-center gap-2 text-kira-md font-medium">
                <Checkbox
                  v-model="servers"
                  :disabled="preview.cliMissing"
                  data-testid="ade-port-servers-check"
                />
                {{ t('space.ade.port.servers.title') }}
              </Label>
              <ul class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm">
                <li
                  v-for="s in serverList"
                  :key="s.name"
                  class="flex items-center gap-2"
                  :data-testid="`ade-port-server-${s.name}`"
                >
                  <span class="font-data">{{ s.name }}</span>
                  <span class="min-w-0 flex-1 truncate text-subtle"
                    >{{ s.transport }} {{ s.target }}</span
                  >
                  <Badge :variant="s.action === 'replace' ? 'warn' : 'ok'">{{
                    s.action === 'replace'
                      ? t('space.ade.port.replace.status')
                      : t('common.action.add.label')
                  }}</Badge>
                </li>
              </ul>
            </section>
            <section
              v-if="preview.envNames?.length"
              class="flex flex-col gap-1"
              data-testid="ade-port-env"
            >
              <h3 class="m-0 text-kira-lg font-medium">{{ t('space.ade.port.env.title') }}</h3>
              <FieldDescription>{{ t('space.ade.port.env.hint') }}</FieldDescription>
              <pre
                class="m-0 overflow-auto rounded-kira-sm bg-field p-2 font-data text-kira-sm"
                data-testid="ade-port-env-lines"
              >{{ envLines }}</pre>
              <div>
                <Button
                  variant="dialog"
                  size="kira-lg"
                  data-testid="ade-port-env-copy"
                  @click="copy(envLines)"
                >
                  {{ t('space.ade.port.env.copy.label') }}
                </Button>
              </div>
            </section>
            <section
              v-if="preview.skipped?.length"
              class="flex flex-col gap-1"
              data-testid="ade-port-skipped"
            >
              <h3 class="m-0 text-kira-lg font-medium">{{ t('space.ade.port.skipped.title') }}</h3>
              <ul class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm">
                <li v-for="s in preview.skipped" :key="s.what"
                  ><span class="font-data">{{ s.what }}</span> {{ s.reason }}</li
                >
              </ul>
            </section>
            <ul
              v-if="preview.notes?.length"
              class="m-0 flex list-none flex-col gap-0.5 p-0 text-kira-sm"
            >
              <li v-for="n in preview.notes" :key="n">{{ n }}</li>
            </ul>
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
          :disabled="!preview || nothing"
          :pending="busy"
          data-testid="ade-port-apply"
          @click="submit"
        >
          {{ t('space.ade.port.exportApply.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
