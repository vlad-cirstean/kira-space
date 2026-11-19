import { t } from '@theme/i18n';
import { computed, type MaybeRefOrGetter } from 'vue';
import { plainText } from '../lib/logTail';
import { useKeptState } from './viewState';

/** The one log find model: a kept filter that hides lines without the text. Trimmed,
 *  case-insensitive substring, ANSI ignored. `shown` feeds LogLines and CopyLogButton;
 *  `needle` marks matches in LogLines; `status` reads "N of M lines" while filtering, else ''. */
export function useLogFilter<T extends { text: string }>(
  lines: () => readonly T[],
  key: MaybeRefOrGetter<string>,
) {
  const filter = useKeptState(key, '');
  const needle = computed(() => filter.value.trim().toLowerCase());
  // Lower-cased plain text once per line object; lines are appended, never mutated.
  const lower = new WeakMap<T, string>();
  const textOf = (l: T): string => {
    let s = lower.get(l);
    if (s === undefined) {
      s = plainText(l.text).toLowerCase();
      lower.set(l, s);
    }
    return s;
  };
  const shown = computed(() => {
    const n = needle.value;
    return n === '' ? lines() : lines().filter((l) => textOf(l).includes(n));
  });
  const status = computed(() =>
    needle.value === ''
      ? ''
      : t('common.logs.matches.status', { shown: shown.value.length, total: lines().length }),
  );
  return { filter, needle, shown, status };
}
