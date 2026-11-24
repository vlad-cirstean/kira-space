<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import type { ProofStaleState } from './queries';

defineProps<{ state: ProofStaleState; disabled: boolean }>();
defineEmits<{ reload: [] }>();
</script>

<template>
  <Alert strip variant="warn" data-testid="proof-stale" :data-state="state">
    <AlertDescription>{{
      t(
        state === 'changed'
          ? 'studio.proof.stale.changed.status'
          : 'studio.proof.stale.removed.status',
      )
    }}</AlertDescription>
    <AlertAction v-if="state === 'changed'" data-shot-skip>
      <Button
        variant="dialog-primary"
        size="kira"
        data-testid="proof-stale-reload"
        :disabled="disabled"
        @click="$emit('reload')"
      >
        {{ t('common.action.refresh.label') }}
      </Button>
    </AlertAction>
  </Alert>
</template>
