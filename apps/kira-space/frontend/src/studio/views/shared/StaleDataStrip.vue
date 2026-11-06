<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';

defineProps<{ reconnecting: boolean; testid: string; reconnectOnly?: boolean }>();
defineEmits<{ reconnect: [] }>();
</script>

<template>
  <Alert strip variant="warn" icon="plug" :data-testid="testid">
    <AlertDescription>{{ t('studio.common.connectionLost.description') }}</AlertDescription>
    <AlertAction>
      <Button
        variant="dialog-primary"
        size="kira"
        :data-testid="`${testid}-reconnect`"
        :pending="reconnecting"
        @click="$emit('reconnect')"
      >
        {{
          t(
            reconnectOnly
              ? 'studio.common.reconnect.label'
              : 'studio.common.reconnectAndLoad.label',
          )
        }}
      </Button>
    </AlertAction>
  </Alert>
</template>
