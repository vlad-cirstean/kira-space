import { t } from '@theme/i18n';
import { computed, type Ref } from 'vue';
import type { ReviewSessionState, ReviewWorkingSection } from '../state/review.ts';

/**
 * Zero commits on the branch but uncommitted files: the range stays `empty`, and the view shows the
 * Uncommitted section and Comments instead of the bare empty text.
 */
export function useReviewWorkingOnly(
  review: Ref<ReviewSessionState | undefined>,
  working: () => ReviewWorkingSection | null | undefined,
) {
  const workingOnly = computed(
    () => review.value?.phase.value === 'empty' && (working()?.files.length ?? 0) > 0,
  );
  /** The panes render: a listed range, or working-only. */
  const showsPanes = computed(() => review.value?.phase.value === 'listing' || workingOnly.value);
  /** The Commits pane has nothing to list when working-only; a stored 'commits' reads as Files. */
  const pane = computed(() =>
    workingOnly.value && review.value?.pane.value === 'commits'
      ? 'files'
      : review.value?.pane.value,
  );
  const emptyNote = computed(() =>
    t('git.review.empty.body', {
      branch: review.value?.branch.value ?? '',
      base: review.value?.resolution.value?.base ?? '',
    }),
  );
  return { workingOnly, showsPanes, pane, emptyNote };
}
