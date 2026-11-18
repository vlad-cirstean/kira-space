<script setup lang="ts">
import Spinner from '@theme/components/Spinner.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { formatShortcut } from '../shortcuts/keys';
import { shortcutKeydown } from '../shortcuts/useShortcut';
import type { AppUpdateStore } from '../state/createAppUpdateStore';

// The in-app update flow, replacing Kira Studio's own release-page banner click.
// ConfirmDialog.vue/GitCredentialDialog.vue's own shadcn-vue Dialog
// structure and classes; the store is a prop (StackDialog.vue's own precedent, packages/git-ui) —
// App.vue owns the v-if (`v-if="store.dialogOpen"`, mounted beside <ConfirmDialog />, the same
// mount-order-decides-stacking reason ConfirmDialog.vue's own comment gives), so this component
// only ever exists while open.
const props = defineProps<{ store: AppUpdateStore }>();

type DialogState = 'idle' | 'installing' | 'checking' | 'checkFailed' | 'upToDate' | 'error';

const state = computed<DialogState>(() => {
  if (props.store.installing) return 'installing';
  if (props.store.checking) return 'checking';
  if (props.store.checkError) return 'checkFailed';
  if (props.store.checkRequested && !props.store.available) return 'upToDate';
  if (props.store.installError) return 'error';
  return 'idle';
});

// Esc/overlay click while idle or in error is Later (ConfirmDialog.vue's own @update:open
// precedent); while installing, both are ignored (below) so `open` never goes false to begin with.
function onOpenChange(open: boolean): void {
  if (!open && state.value !== 'installing') props.store.later();
}

function onEscapeOrOutside(e: Event): void {
  if (state.value === 'installing') e.preventDefault();
}

// Mod+Enter runs the primary button of the shown state; single keys do nothing.
const onChord = shortcutKeydown(['form.submit'], () => {
  if (state.value === 'checkFailed') void props.store.checkForUpdates();
  else if (state.value === 'upToDate') props.store.later();
  else if (state.value === 'idle' || state.value === 'error') void props.store.install();
});
</script>

<template>
  <Dialog :open="true" @update:open="onOpenChange">
    <DialogContent
      size="sm"
      prompt
      data-testid="update-dialog"
      @keydown="onChord"
      @escape-key-down="onEscapeOrOutside"
      @pointer-down-outside="onEscapeOrOutside"
    >
      <DialogHeader :closable="state !== 'installing'">
        <DialogTitle>{{ t('workbench.update.dialog.title', { app: store.appName }) }}</DialogTitle>
      </DialogHeader>

      <DialogBody>
        <template v-if="state === 'installing'">
          <p
            class="m-0 flex items-center gap-2"
            role="status"
            data-testid="update-dialog-installing"
          >
            <Spinner :size="13" label="" />
            {{
              t('workbench.update.installing.body', {
                app: store.appName,
                version: store.targetVersion,
              })
            }}
          </p>
        </template>
        <p
          v-else-if="state === 'checking'"
          class="m-0 flex items-center gap-2"
          role="status"
          data-testid="update-dialog-checking"
        >
          <Spinner :size="13" label="" />
          {{ t('workbench.update.checking.status') }}
        </p>
        <Alert
          v-else-if="state === 'checkFailed'"
          variant="error"
          data-testid="update-dialog-check-error"
        >
          <AlertDescription>{{ store.checkError }}</AlertDescription>
        </Alert>
        <p v-else-if="state === 'upToDate'" class="m-0" data-testid="update-dialog-up-to-date">
          {{
            t('workbench.update.upToDate.body', {
              app: store.appName,
              current: store.currentVersion,
            })
          }}
        </p>
        <template v-else>
          <p class="m-0" data-testid="update-dialog-versions">
            {{
              t('workbench.update.versions.body', {
                app: store.appName,
                target: store.targetVersion,
                current: store.currentVersion,
              })
            }}
          </p>
          <p v-if="store.stepped" class="m-0" data-testid="update-dialog-step-note">
            {{
              t('workbench.update.stepNote.body', {
                app: store.appName,
                newest: store.newestVersion,
              })
            }}
          </p>
          <p class="m-0 text-subtle">
            {{
              t('workbench.update.explain.body', {
                app: store.appName,
                folder: '/Applications',
                log: store.installLogPath,
              })
            }}
          </p>
          <Alert v-if="state === 'error'" variant="error" data-testid="update-dialog-error">
            <AlertDescription>
              {{ store.installError }}
              {{ t('workbench.update.installDetails.error', { log: store.installLogPath }) }}
            </AlertDescription>
          </Alert>
        </template>
      </DialogBody>

      <DialogFooter>
        <template v-if="state === 'installing'">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="update-dialog-cancel"
            @click="store.cancelInstall()"
          >
            {{ t('common.action.cancel.label') }}
          </Button>
        </template>
        <template v-else-if="state === 'checking'" />
        <template v-else-if="state === 'checkFailed'">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="update-dialog-close"
            @click="store.later()"
          >
            {{ t('common.action.close.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :kbd="formatShortcut('form.submit')"
            data-testid="update-dialog-retry"
            @click="store.checkForUpdates()"
          >
            {{ t('common.action.retry.label') }}
          </Button>
        </template>
        <template v-else-if="state === 'upToDate'">
          <Button
            variant="dialog"
            size="kira-lg"
            :kbd="formatShortcut('form.submit')"
            data-testid="update-dialog-close"
            @click="store.later()"
          >
            {{ t('common.action.close.label') }}
          </Button>
        </template>
        <template v-else>
          <Button
            variant="dialog"
            size="kira-lg"
            :kbd="formatShortcut('prompt.hide')"
            data-testid="update-dialog-later"
            @click="store.later()"
          >
            {{ t('common.later.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :kbd="formatShortcut('form.submit')"
            data-testid="update-dialog-install"
            @click="store.install()"
          >
            {{
              state === 'error'
                ? t('common.action.retry.label')
                : t('workbench.update.install.label')
            }}
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
