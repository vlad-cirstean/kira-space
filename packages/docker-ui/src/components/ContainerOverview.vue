<script setup lang="ts">
import DetailSection from '@theme/components/DetailSection.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import MaskedValue from '@theme/components/MaskedValue.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { displayValue, formatDateTime } from '@workbench/behaviour/format';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import { reactive } from 'vue';
import type { DockerContainerDetail } from '../wire';

const props = defineProps<{ detail: DockerContainerDetail }>();

const revealed = reactive(new Set<number>());

function splitEnv(entry: string): { key: string; value: string } {
  const i = entry.indexOf('=');
  return i === -1
    ? { key: entry, value: '' }
    : { key: entry.slice(0, i), value: entry.slice(i + 1) };
}

function toggle(i: number): void {
  if (revealed.has(i)) revealed.delete(i);
  else revealed.add(i);
}

const dateOrDash = (s: string): string =>
  s && !s.startsWith('0001-') ? formatDateTime(s) : displayValue(null);
const join = (parts: string[]): string => displayValue(parts.join(' '));

const rows = (): Array<[string, string]> => [
  [t('common.field.command.label'), join(props.detail.command)],
  [t('docker.field.entrypoint.label'), join(props.detail.entrypoint)],
  [t('docker.field.workingFolder.label'), displayValue(props.detail.workingDir)],
  [t('common.user.label'), displayValue(props.detail.user)],
  [t('docker.field.restartPolicy.label'), props.detail.restartPolicy || 'no'],
  [t('docker.field.health.label'), displayValue(props.detail.health)],
  [t('common.created.label'), formatDateTime(props.detail.container.created * 1000)],
  [t('docker.field.started.label'), dateOrDash(props.detail.startedAt)],
  [t('docker.field.finished.label'), dateOrDash(props.detail.finishedAt)],
  [
    t('docker.field.exitCode.label'),
    props.detail.container.state === 'running' ? displayValue(null) : String(props.detail.exitCode),
  ],
];
</script>

<template>
  <div class="h-full overflow-auto p-3" data-testid="docker-overview">
    <div class="@container">
      <div class="grid grid-cols-1 items-start gap-3 @3xl:grid-cols-2">
        <DetailSection :title="t('common.details.title')">
          <KeyValueList>
            <template v-for="[ k, v ] in rows()" :key="k">
              <dt>{{ k }}</dt>
              <dd class="break-all font-data">{{ v }}</dd>
            </template>
          </KeyValueList>
        </DetailSection>

        <div class="flex min-w-0 flex-col gap-3">
          <DetailSection
            v-if="detail.container.ports.length"
            :title="t('docker.field.ports.label')"
          >
            <div
              v-for="p in detail.container.ports"
              :key="`${p.ip}:${p.publicPort}:${p.privatePort}/${p.type}`"
              class="font-data"
              data-testid="docker-overview-port"
            >
              {{ p.publicPort ? `${p.ip || '0.0.0.0'}:${p.publicPort} -> ` : '' }}
              {{ p.privatePort }}/{{ p.type }}
            </div>
          </DetailSection>

          <DetailSection
            v-if="detail.networkAttachments.length"
            :title="t('docker.field.networks.label')"
          >
            <div
              v-for="n in detail.networkAttachments"
              :key="n.network"
              class="font-data"
              data-testid="docker-overview-network"
            >
              {{ n.network }}
              <span class="text-muted-foreground"
                >{{ n.ipAddress }}{{ n.aliases.length ? ` (${n.aliases.join(', ')})` : '' }}</span
              >
            </div>
          </DetailSection>

          <DetailSection v-if="detail.mounts.length" :title="t('docker.field.mounts.label')">
            <div
              v-for="m in detail.mounts"
              :key="m.destination"
              class="break-all font-data"
              data-testid="docker-overview-mount"
            >
              {{ m.name || m.source }}
              -> {{ m.destination }}
              <span class="text-muted-foreground">{{
                m.rw ? m.type : t('docker.mount.readOnly.label', { type: m.type })
              }}</span>
            </div>
          </DetailSection>

          <p
            v-if="
              !detail.container.ports.length &&
              !detail.networkAttachments.length &&
              !detail.mounts.length
            "
            class="px-1 text-muted-foreground"
            data-testid="docker-overview-bare"
            >{{
              t('docker.overview.bare.emptyHint')
            }}</p
          >
        </div>

        <DetailSection
          v-if="detail.env.length"
          :title="t('common.field.environment.label')"
          class="@3xl:col-span-2"
        >
          <div
            v-for="(entry, i) in detail.env"
            :key="i"
            :class="
              cn(rowVariants({ layout: 'tree' }), 'group/env -mx-1 rounded-kira-sm pl-1 font-data')
            "
            data-testid="docker-env-row"
          >
            <span class="shrink-0 text-muted-foreground">{{ splitEnv(entry).key }}=</span>
            <span
              v-if="revealed.has(i)"
              class="min-w-0 flex-1 break-all"
              data-testid="docker-env-value"
              >{{
                splitEnv(entry).value
              }}</span
            >
            <MaskedValue v-else class="min-w-0 flex-1 break-all" data-testid="docker-env-value" />
            <TooltipIconButton
              :icon="revealed.has(i) ? 'eye-closed' : 'eye'"
              :label="
                revealed.has(i) ? t('common.hideValue.label') : t('docker.env.revealValue.tooltip')
              "
              data-testid="docker-env-reveal"
              @click="toggle(i)"
            />
            <CopyIconButton
              :text="splitEnv(entry).value"
              :label="t('common.action.copyValue.label')"
            />
          </div>
        </DetailSection>

        <DetailSection
          v-if="Object.keys(detail.labels).length"
          :title="t('common.field.labels.label')"
          class="@3xl:col-span-2"
        >
          <div v-for="(v, k) in detail.labels" :key="k" class="break-all font-data">
            <span class="text-muted-foreground">{{ k }}=</span>{{ v }}
          </div>
        </DetailSection>
      </div>
    </div>
  </div>
</template>
