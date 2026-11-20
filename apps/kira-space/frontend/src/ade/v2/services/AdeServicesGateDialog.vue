<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeRepoTag from '../AdeRepoTag.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdeServicesGateStore } from '../state/adeServicesGate';
import type { RequiredService, RequiredServiceState } from '../wire';
import { needsStart, useRequiredServices } from './requiredServices';

// The ask before work starts on a task whose repos need services that are not running. There is no
// Cancel: the X and Esc cancel the start. Run hands the start over to the script's run dialog.
const gate = useAdeServicesGateStore();
const { model } = usePlanModel();
const repos = useProjectOptions();

const open = computed(() => gate.request !== null);
const taskId = computed(() => gate.request?.taskId ?? '');
const card = computed(() => (taskId.value ? (model.value?.cardFor(taskId.value) ?? null) : null));
const services = useRequiredServices(taskId);
const items = computed(() => services.data.value?.items ?? []);
const allRunning = computed(() => items.value.length > 0 && !items.value.some(needsStart));

const repoLabel = (id: string): string =>
  repos.options.value.find((o) => o.value === id)?.label ?? id;
const branchLabel = (s: RequiredService): string =>
  s.branchId ? (card.value?.rows.find((r) => r.id === s.branchId)?.name ?? '') : '';

type Tone = 'ok' | 'warn' | 'default' | 'error';
const TONE: Record<RequiredServiceState, Tone> = {
  running: 'ok',
  stopped: 'warn',
  noWorktree: 'default',
  invalid: 'error',
};
const STATUS: Record<RequiredServiceState, () => string> = {
  running: () => t('common.state.running.status'),
  stopped: () => t('space.ade.servicesGate.state.stopped.status'),
  noWorktree: () => t('space.ade.servicesGate.state.noWorktree.status'),
  invalid: () => t('space.ade.servicesGate.state.invalid.status'),
};
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && gate.cancel()">
    <DialogContent size="sm" data-testid="ade-services-gate">
      <DialogHeader>
        <DialogTitle>{{ t('common.requiredServices.title') }}</DialogTitle>
        <DialogDescription>{{
          t('space.ade.servicesGate.body', { task: card?.title ?? '' })
        }}</DialogDescription>
      </DialogHeader>
      <DialogBody>
        <div class="flex flex-col gap-3">
          <div
            v-for="s in items"
            :key="`${s.codeRepoId}:${s.branchId}:${s.script}`"
            class="flex flex-col gap-1"
            data-testid="ade-services-gate-row"
            :data-state="s.state"
          >
            <div class="flex items-center gap-1.5">
              <AdeRepoTag :code-repo-id="s.codeRepoId" :label="repoLabel(s.codeRepoId)" />
              <span class="min-w-0 flex-1 truncate font-data">{{ s.script }}</span>
              <span v-if="branchLabel(s)" class="truncate font-data text-muted-foreground">{{
                branchLabel(s)
              }}</span>
              <Badge :variant="TONE[s.state]">{{ STATUS[s.state]() }}</Badge>
              <Button
                v-if="s.state === 'stopped'"
                variant="dialog"
                size="kira"
                data-testid="ade-services-gate-run"
                @click="gate.run(s)"
              >
                {{ t('common.action.run.label') }}
              </Button>
            </div>
            <FieldError v-if="s.state === 'invalid'">
              {{
                t('space.ade.servicesGate.state.invalid.error', {
                  script: s.script,
                  reason: s.problem,
                })
              }}
            </FieldError>
          </div>
          <FieldDescription data-testid="ade-services-gate-hint">{{
            allRunning
              ? t('space.ade.servicesGate.allRunning.hint')
              : t('space.ade.servicesGate.only.hint')
          }}</FieldDescription>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          data-testid="ade-services-gate-continue"
          @click="gate.proceed()"
        >
          {{ t('common.continue.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
