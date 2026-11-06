import type { RangeHighlight } from '@workbench/editor/ranges';
import { computed, ref, type ShallowRef } from 'vue';
import { DEFAULT_FIND_OPTIONS, type FindOptions, findRanges } from '../../editor/findRanges';
import type { FindBarHost, FindBarTarget } from '../shared/ResponseFindBar.vue';

// The request panel's half of the find bar. It searches the body editor only: params and headers
// tables have their own filter. Component-local: a lens over what is on screen, not tab state.
export interface RequestBodyHandle {
  findableDoc: string | null;
  findHost: FindBarHost | null;
}
export interface FindBarHandle {
  query: string;
  currentGlobal: number;
  options: FindOptions;
}

export function useRequestFind(
  requestBodyRef: Readonly<ShallowRef<RequestBodyHandle | null>>,
  requestFindBarRef: Readonly<ShallowRef<FindBarHandle | null>>,
) {
  const requestFindOpen = ref(false);

  function toggleRequestFind(): void {
    requestFindOpen.value = !requestFindOpen.value;
  }
  function closeRequestFind(): void {
    requestFindOpen.value = false;
  }

  /** Empty whenever the body pane is not showing a text editor (params/headers/urlencoded/form-data/
   *  binary), which is what makes the bar honestly report "0 of 0" there rather than searching a
   *  document that is not on screen. */
  const requestFindTargets = computed<readonly FindBarTarget[]>(() => {
    if (!requestFindOpen.value) return [];
    const doc = requestBodyRef.value?.findableDoc;
    if (doc === null || doc === undefined) return [];
    return [{ doc, host: requestBodyRef.value?.findHost ?? null }];
  });

  /** Paints exactly the matches the bar counts and steps through. Read synchronously here (not
   *  inside the returned closure) so this computed's own identity changes when the query, the
   *  options or the current match does — which is what makes CodeMirrorHost repaint. */
  const requestFindHighlights = computed<((doc: string) => readonly RangeHighlight[]) | undefined>(
    () => {
      if (!requestFindOpen.value) return undefined;
      const bar = requestFindBarRef.value;
      const query = bar?.query ?? '';
      if (!query) return undefined;
      const current = bar?.currentGlobal ?? -1;
      const options = bar?.options ?? DEFAULT_FIND_OPTIONS;
      return (doc: string) => findRanges(doc, query, current, options);
    },
  );

  return {
    requestFindOpen,
    requestFindTargets,
    requestFindHighlights,
    toggleRequestFind,
    closeRequestFind,
  };
}
