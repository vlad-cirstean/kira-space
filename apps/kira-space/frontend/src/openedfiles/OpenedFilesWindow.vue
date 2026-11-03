<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import MainView from '@workbench/components/MainView.vue';
import TabStrip from '@workbench/components/TabStrip.vue';
import TitleBarBase from '@workbench/components/TitleBar.vue';
</script>

<template>
  <div class="flex h-full flex-col" data-testid="opened-files-window">
    <TitleBarBase>
      <span class="min-w-0 truncate text-kira-md font-medium" data-testid="opened-files-title">{{
        t('space.openedFiles.window.title')
      }}</span>
    </TitleBarBase>
    <div class="flex min-h-0 flex-1 bg-chrome px-1.5 pb-0.5">
      <section
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-kira border border-border bg-bg"
        :aria-label="t('space.openedFiles.window.title')"
      >
        <div
          class="h-tabbar min-h-0 shrink-0 overflow-hidden border-b border-border bg-chrome"
          data-testid="opened-files-tabs"
        >
          <TabStrip />
        </div>
        <div class="min-h-0 flex-1">
          <MainView>
            <template #empty>
              <EmptyState
                fill
                icon="file"
                :title="t('space.openedFiles.window.empty.title')"
                :description="t('space.openedFiles.window.empty.body')"
                data-testid="opened-files-empty"
              />
            </template>
          </MainView>
        </div>
      </section>
    </div>
  </div>
</template>
