<script setup lang="ts">
import { t } from '@theme/i18n';
import type { PortResult } from './types';

// What an Import or Export wrote. A backup path is one name beside its file.
defineProps<{ result: PortResult }>();

const base = (path: string): string => path.split(/[\\/]/).pop() ?? path;
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3 text-kira-md" data-testid="ade-port-result">
    <h3 class="m-0 text-kira-lg font-medium">{{ t('common.done.status') }}</h3>
    <section v-if="result.written?.length" class="flex flex-col gap-0.5">
      <div class="text-kira-sm text-subtle">{{ t('space.ade.port.written.label') }}</div>
      <ul class="m-0 list-none p-0 font-data text-kira-sm" data-testid="ade-port-written">
        <li v-for="p in result.written" :key="p">{{ p }}</li>
      </ul>
    </section>
    <section v-if="result.backups?.length" class="flex flex-col gap-0.5">
      <div class="text-kira-sm text-subtle">{{ t('space.ade.port.backups.label') }}</div>
      <ul class="m-0 list-none p-0 font-data text-kira-sm" data-testid="ade-port-backups">
        <li v-for="p in result.backups" :key="p">{{ base(p) }}</li>
      </ul>
    </section>
    <section v-if="result.servers?.length" class="flex flex-col gap-0.5">
      <div class="text-kira-sm text-subtle">{{ t('space.ade.port.servers.title') }}</div>
      <ul class="m-0 list-none p-0 text-kira-sm" data-testid="ade-port-servers">
        <li v-for="s in result.servers" :key="s.name">
          <span class="font-data">{{ s.name }}</span> {{ s.outcome
          }}<span v-if="s.detail">: {{ s.detail }}</span>
        </li>
      </ul>
    </section>
    <section v-if="result.secrets?.length" class="flex flex-col gap-0.5">
      <div class="text-kira-sm text-subtle">{{ t('space.ade.port.savedSecrets.label') }}</div>
      <ul class="m-0 list-none p-0 font-data text-kira-sm" data-testid="ade-port-secrets">
        <li v-for="s in result.secrets" :key="s">{{ s }}</li>
      </ul>
    </section>
    <ul v-if="result.notes?.length" class="m-0 list-none p-0 text-kira-sm">
      <li v-for="n in result.notes" :key="n">{{ n }}</li>
    </ul>
  </div>
</template>
