import { createSharedComposable } from '@vueuse/core';
import { useRepos } from '../../../repo/state/reposQueries';

/** One repos observer for every branch row, not one per row. */
export const useSharedRepos = createSharedComposable(useRepos);
