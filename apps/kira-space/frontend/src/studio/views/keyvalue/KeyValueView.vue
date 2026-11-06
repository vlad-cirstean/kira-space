<script setup lang="ts">
import type { KeyValueTabRecord } from '../../state/tabDomain';
import KeyValuePane from '../shared/keyvalue/KeyValuePane.vue';

// MainView.vue keys this component by tab.id — same discipline as every other view.
//
// This file's whole former body (badges/toolbar/strips/table/cell-editor, ~1000 lines)
// now lives in views/shared/keyvalue/KeyValuePane.vue, shared with the browse split's preview
// pane — under views/shared/, not views/keyvalue/, per that file's own module doc comment. Passing
// `tab` tells KeyValuePane to wrap itself in its own ViewChrome
// (icon/path/name/refresh/stop/reconnect) — see that file's own module doc comment for why the
// chrome lives there rather than here.
defineProps<{ tab: KeyValueTabRecord }>();
</script>

<template>
  <div class="h-full flex flex-col min-h-0" data-testid="keyvalue-view" :data-path="tab.path">
    <KeyValuePane :view-key="tab.id" :tab="tab" />
  </div>
</template>
