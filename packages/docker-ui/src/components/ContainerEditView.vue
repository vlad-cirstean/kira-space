<script setup lang="ts">
import DetailSection from '@theme/components/DetailSection.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Skeleton } from '@theme/components/ui/skeleton';
import { Tabs, TabsContent } from '@theme/components/ui/tabs';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { useDocker } from '../context';
import { fieldErrors, hasNowChanges, inPlaceForApply } from '../lib/editDiff';
import {
  useContainerEditSpec,
  useDockerStatus,
  useNetworks,
  useRecreateContainer,
  useUpdateContainer,
} from '../queries';
import { type EditFailure, useDockerEditStore } from '../state/dockerEdit';
import { useDockerExecSessionsStore } from '../state/dockerExec';
import { useDockerUiStore } from '../state/dockerUi';
import EditInPlaceSection from './EditInPlaceSection.vue';
import EditPendingSummary from './EditPendingSummary.vue';
import EditRecreateSection from './EditRecreateSection.vue';
import OriginIcon from './OriginIcon.vue';
import RecreateConfirmDialog from './RecreateConfirmDialog.vue';

const props = defineProps<{ containerId: string }>();

const ctx = useDocker();
const ui = useDockerUiStore();
const store = useDockerEditStore();
const execSessions = useDockerExecSessionsStore();
const id = computed(() => props.containerId);
const specQuery = useContainerEditSpec(id);
const status = useDockerStatus();
const networks = useNetworks();
const update = useUpdateContainer();
const recreate = useRecreateContainer();
const confirming = ref(false);
const tab = ref<'inPlace' | 'recreate'>('inPlace');
const EDIT_TABS = computed(() => [
  {
    value: 'inPlace',
    label: t('docker.edit.tab.inPlace.label'),
    testid: 'docker-edit-tab-inplace',
  },
  {
    value: 'recreate',
    label: t('docker.edit.tab.recreate.label'),
    testid: 'docker-edit-tab-recreate',
  },
]);

watch(
  () => specQuery.data.value,
  (s) => {
    if (s) store.ensure(s);
  },
  { immediate: true },
);

const draft = computed(() => store.drafts[props.containerId]);
const base = computed(() => draft.value?.base);
const errors = computed(() => (draft.value ? fieldErrors(draft.value) : {}));
const inPlaceErrors = computed(() =>
  Object.keys(errors.value).some((k) => !k.startsWith('recreate.')),
);
const anyErrors = computed(() => Object.keys(errors.value).length > 0);
const changes = computed(() => store.pending(props.containerId));
const inPlaceCount = computed(() => changes.value.filter((c) => c.section === 'inPlace').length);
const recreateCount = computed(() => changes.value.filter((c) => c.section === 'recreate').length);
const applying = computed(() => store.applying[props.containerId]);
const readOnly = computed(() => (base.value?.managed ?? '') !== '');
const networkLocked = computed(() => /^(host|none|container:)/.test(base.value?.networkMode ?? ''));
const allNetworks = computed(() => (networks.data.value ?? []).map((n) => n.name));
const engineCpus = computed(() => status.data.value?.engine?.cpus);

const canApplyNow = computed(
  () =>
    !!draft.value &&
    !applying.value &&
    !inPlaceErrors.value &&
    hasNowChanges(draft.value.base, draft.value),
);
const canRecreate = computed(
  () =>
    !!base.value &&
    !applying.value &&
    !anyErrors.value &&
    !base.value.autoRemove &&
    changes.value.some((c) => c.mode === 'recreate'),
);

const outcome = computed(() => store.outcome[props.containerId]);
const failure = computed(() => store.failure[props.containerId]);

function toFailure(e: unknown): EditFailure {
  const err = e as Error & { code?: string; details?: { restored?: boolean } };
  return {
    message: err.message,
    code: err.code ?? 'E_INTERNAL',
    restored: err.details?.restored === true,
  };
}

function fail(e: unknown): void {
  const f = toFailure(e);
  store.failure[props.containerId] = f;
  if (f.code === 'E_CONFLICT' && draft.value) draft.value.stale = true;
}

async function applyNow(): Promise<void> {
  const d = draft.value;
  if (!d) return;
  const cid = props.containerId;
  store.begin(cid, 'inPlace');
  try {
    const r = await update.mutateAsync({
      id: cid,
      baseHash: d.base.baseHash,
      spec: inPlaceForApply(d.base.inPlace, d.inPlace),
    });
    store.outcome[cid] = { warnings: r.warnings };
    store.expectRebase(cid);
  } catch (e) {
    fail(e);
    if ((e as { code?: string }).code !== 'E_CONFLICT') store.expectRebase(cid);
  } finally {
    store.end(cid);
  }
}

async function confirmRecreate(): Promise<void> {
  const d = draft.value;
  if (!d) return;
  const cid = props.containerId;
  confirming.value = false;
  store.begin(cid, 'recreate');
  try {
    const r = await recreate.mutateAsync({
      id: cid,
      baseHash: d.base.baseHash,
      inPlace: d.inPlace,
      recreate: d.recreate,
    });
    execSessions.closeForContainer(ctx, cid);
    store.discard(cid);
    store.outcome[r.id] = { warnings: r.warnings };
    ui.select({ kind: 'container', id: r.id }, 'edit');
  } catch (e) {
    fail(e);
  } finally {
    store.end(cid);
  }
}

async function reload(): Promise<void> {
  store.discard(props.containerId);
  const fresh = (await specQuery.refetch()).data;
  if (fresh) store.ensure(fresh);
}

const orchestratorName = computed(() =>
  base.value?.managed === 'kubernetes' ? 'Kubernetes' : 'Swarm',
);

const originNotice = computed(() => {
  const b = base.value;
  if (!b) return '';
  switch (b.origin) {
    case 'compose':
      return t('docker.edit.origin.compose.body', { tool: 'Compose', name: b.originName });
    case 'devcontainer':
      return t('docker.edit.origin.devcontainer.body', { name: b.originName });
    case 'kind':
      return t('docker.edit.origin.kind.body', { name: b.originName });
    case 'testcontainers':
      return t('docker.edit.origin.testcontainers.body', { tool: 'Testcontainers' });
    case 'buildx':
      return t('docker.edit.origin.buildx.body', { tool: 'Buildx', name: b.originName });
    default:
      return '';
  }
});
</script>

<template>
  <div class="@container h-full overflow-auto p-3" data-testid="docker-edit">
    <div v-if="!draft || !base" class="flex flex-col gap-2" aria-busy="true">
      <Skeleton class="h-control-lg w-1/3" />
      <Skeleton class="h-4 w-2/3" />
    </div>
    <div v-else class="flex flex-col gap-3">
      <Alert v-if="base.managed" variant="warn" data-testid="docker-edit-managed">
        <AlertDescription>
          {{ t('docker.edit.managed.body', { orchestrator: orchestratorName }) }}
        </AlertDescription>
      </Alert>
      <Alert v-if="originNotice" variant="info" data-testid="docker-edit-origin">
        <AlertDescription>
          <div class="flex items-center gap-2">
            <OriginIcon :origin="base.origin" :name="base.originName" />{{ originNotice }}
          </div>
        </AlertDescription>
      </Alert>
      <Alert v-if="base.autoRemove" variant="info" data-testid="docker-edit-autoremove">
        <AlertDescription>{{ t('docker.edit.autoRemove.body') }}</AlertDescription>
      </Alert>
      <Alert v-if="draft.stale" variant="warn" data-testid="docker-edit-stale">
        <AlertDescription>
          <div class="flex items-center gap-2">
            {{ t('docker.edit.stale.body') }}
            <Button size="inline" variant="link" data-testid="docker-edit-reload" @click="reload">{{
              t('common.action.refresh.label')
            }}</Button>
          </div>
        </AlertDescription>
      </Alert>
      <Alert v-if="failure" variant="error" data-testid="docker-edit-error">
        <AlertDescription>
          {{ failure.message }}
          <template v-if="failure.code === 'E_RECREATE_FAILED' && failure.restored">
            {{ t('docker.edit.restored.body') }}</template
          >
        </AlertDescription>
      </Alert>
      <Alert v-if="outcome?.warnings.length" variant="warn" data-testid="docker-edit-warnings">
        <AlertDescription>
          <div v-for="w in outcome.warnings" :key="w">{{ w }}</div>
        </AlertDescription>
      </Alert>

      <DetailSection v-if="changes.length" :title="t('common.changes.pending.title')">
        <EditPendingSummary :changes="changes" />
      </DetailSection>

      <Tabs :model-value="tab" class="flex flex-col gap-3">
        <SecondaryTabs
          :model-value="tab"
          :items="EDIT_TABS"
          :aria-label="t('docker.edit.mode.aria')"
          @update:model-value="(v) => (tab = v as typeof tab)"
        >
          <template #item="{ item }">
            {{ item.label }}
            <StatusDot
              v-if="item.value === 'inPlace' ? inPlaceCount : recreateCount"
              tone="warn"
              data-testid="docker-edit-tab-dirty"
            />
          </template>
        </SecondaryTabs>

        <TabsContent
          value="inPlace"
          class="flex flex-col gap-3"
          data-testid="docker-edit-section-inplace"
        >
          <EditInPlaceSection
            :draft="draft"
            :errors="errors"
            :disabled="readOnly || !!applying"
            :network-locked="networkLocked"
            :engine-cpus="engineCpus"
            :all-networks="allNetworks"
          />
          <div class="flex items-center justify-end gap-1">
            <TooltipIconButton
              icon="discard"
              :label="t('common.action.reset.label')"
              :disabled="!inPlaceCount"
              data-testid="docker-edit-reset-inplace"
              @click="store.reset(containerId, 'inPlace')"
            />
            <Button
              v-if="!readOnly"
              size="kira-lg"
              variant="dialog-primary"
              :disabled="!canApplyNow"
              :pending="applying === 'inPlace'"
              data-testid="docker-edit-apply-now"
              @click="applyNow"
            >
              {{ t('common.action.apply.label') }}
            </Button>
          </div>
        </TabsContent>

        <TabsContent
          value="recreate"
          class="flex flex-col gap-3"
          data-testid="docker-edit-section-recreate"
        >
          <EditRecreateSection
            :draft="draft"
            :errors="errors"
            :disabled="readOnly || !!applying"
            :preserved="base.preserved"
          />
          <div class="flex items-center justify-end gap-1">
            <TooltipIconButton
              icon="discard"
              :label="t('common.action.reset.label')"
              :disabled="!recreateCount"
              data-testid="docker-edit-reset-recreate"
              @click="store.reset(containerId, 'recreate')"
            />
            <Button
              v-if="!readOnly"
              size="kira-lg"
              variant="dialog-primary"
              :disabled="!canRecreate"
              :pending="applying === 'recreate'"
              data-testid="docker-edit-apply-recreate"
              @click="confirming = true"
            >
              {{ t('docker.edit.recreate.label') }}
            </Button>
          </div>
        </TabsContent>
      </Tabs>
    </div>

    <RecreateConfirmDialog
      v-if="confirming && base"
      :spec="base"
      :changes="changes"
      :pending="!!applying"
      @close="confirming = false"
      @confirm="confirmRecreate"
    />
  </div>
</template>
