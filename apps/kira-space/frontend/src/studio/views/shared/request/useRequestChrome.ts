import { computed } from 'vue';
import { useVariablesStore } from '../../../api/state/variables';
import { useConnectionsStore } from '../../../state/connections';
import { useRunState } from '../../../state/runState';
import { useTabIncognitoStore } from '../../../state/tabIncognito';

// P107 I2-16: HttpRequestView.vue's and GrpcRequestView.vue's own "P104 §3: ViewChrome/
// ViewHeader/RunState inlined at this call site" block, byte-identical apart from `connRecord`
// reading `tab.connectionId` (both do — HTTP's is on the tab itself, gRPC's the same).

export function useRequestChrome(tab: () => { id: string; connectionId: string | null }) {
  const variablesStore = useVariablesStore();
  const connectionsStore = useConnectionsStore();
  const tabIncognitoStore = useTabIncognitoStore();

  const envColor = computed(() => variablesStore.environmentColorForTab(tab().id));
  const connRecord = computed(() => connectionsStore.connectionRecord(tab().connectionId));
  const railColor = computed(() =>
    envColor.value !== undefined
      ? envColor.value
      : connRecord.value
        ? (connRecord.value.color ?? null)
        : undefined,
  );
  const runState = useRunState(() => tab().id);

  const incognito = computed(() => tabIncognitoStore.isIncognito(tab().id));
  function toggleIncognito(): void {
    tabIncognitoStore.setIncognito(tab().id, !incognito.value);
  }
  const envId = computed(() => variablesStore.environmentIdForTab(tab().id));

  return {
    envColor,
    connRecord,
    railColor,
    runState,
    incognito,
    toggleIncognito,
    envId,
  };
}
