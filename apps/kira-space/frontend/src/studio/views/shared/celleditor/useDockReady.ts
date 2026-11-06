import { type Ref, ref, watch } from 'vue';

/** `ready` once every Monaco host the editor mounts has painted. The diff view and the timestamp
 *  pane have no host of their own to wait on. */
export function useDockReady(
  diffShown: Ref<boolean>,
  decodedShown: Ref<boolean>,
  onReady: () => void,
) {
  const mainReady = ref(false);
  const decodedReady = ref(false);
  watch(diffShown, () => {
    mainReady.value = false;
  });
  watch(decodedShown, (shown) => {
    if (!shown) decodedReady.value = false;
  });
  watch(
    () => diffShown.value || (mainReady.value && (!decodedShown.value || decodedReady.value)),
    (ok) => {
      if (ok) onReady();
    },
    { immediate: true },
  );
  return {
    markMainReady: () => {
      mainReady.value = true;
    },
    markDecodedReady: () => {
      decodedReady.value = true;
    },
  };
}
