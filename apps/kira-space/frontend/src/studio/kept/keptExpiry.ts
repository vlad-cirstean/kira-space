import { formatCompactDuration } from '@theme/format';
import { t } from '@theme/i18n';
import { useCountdown } from '@workbench/approvals/useCountdown';
import { computed } from 'vue';

/** "47m left" for a temporary variable's lifetime, "Expired" once over. Display only: the backend
 *  decides expiry. Lifetimes run minutes to hours, hence `formatCompactDuration`. */
export function useKeptExpiry(expiresAt: () => string) {
  const left = useCountdown(() => new Date(expiresAt()).getTime());
  return computed(() =>
    left.value === null || left.value <= 0
      ? t('common.expired.status')
      : t('common.left.status', {
          time: formatCompactDuration(left.value * 1000, { ceil: true }),
        }),
  );
}
