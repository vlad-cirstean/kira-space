import { shallowRef } from 'vue';
import { createI18n } from 'vue-i18n';
import en from './en.json';
import { formatCount } from './number';
import { pseudoMessages } from './pseudo';
import type { CopyLocale, MessageKey, MessageParams } from './types';

export type { CopyLocale, MessageKey, MessageParams, PlainKey } from './types';

export const i18n = createI18n({
  legacy: false,
  globalInjection: false,
  locale: 'en',
  fallbackLocale: 'en',
  missingWarn: false,
  fallbackWarn: false,
  messages: { en },
});

// Params every message can use without the call site passing them: the active agent's names
// ({agent}, {product}, {headless}, {command}), set once the bound AgentService answers. A ref, so a
// render that called `t` re-runs when the provider changes.
const defaults = shallowRef<MessageParams>({});

// Skips an equal set: the pending placeholder itself calls `t`, so a blind write would re-run it forever.
export function setMessageDefaults(params: MessageParams): void {
  const cur = defaults.value;
  const keys = Object.keys(params);
  if (keys.length === Object.keys(cur).length && keys.every((k) => cur[k] === params[k])) return;
  defaults.value = params;
}

export function messageDefaults(): MessageParams {
  return defaults.value;
}

// Every numeric param formats with locale digit grouping; `count` also selects the plural branch.
// An identifier (PR or issue number) is not a quantity: pass `String(n)` so it stays ungrouped.
export function t(key: MessageKey, params?: MessageParams): string {
  const g = i18n.global;
  const formatted: Record<string, string | number> = { ...defaults.value };
  for (const [name, value] of Object.entries(params ?? {})) {
    formatted[name] = typeof value === 'number' ? formatCount(value) : value;
  }
  return typeof params?.count === 'number'
    ? g.t(key, formatted, params.count)
    : g.t(key, formatted);
}

// The sentence separator is locale-dependent (none in CJK); it lives here, not at call sites.
export function joinSentences(...parts: readonly (string | false | null | undefined)[]): string {
  return parts.filter((p): p is string => typeof p === 'string' && p !== '').join(' ');
}

/** The display text a code host provider supplies (`GitHostInfo`). */
export interface HostWordsSource {
  readonly label: string;
  readonly changeRequest: string;
  readonly changeRequestNoun: string;
  readonly changeRequestShort: string;
}

/** The host's words for change requests, or neutral ones while no host is known. */
export function hostWords(host: HostWordsSource | null | undefined): HostWordsSource {
  if (host) return host;
  const changeRequest = t('common.gitHost.changeRequest.label');
  return {
    label: t('common.gitHost.label'),
    changeRequest,
    changeRequestNoun: changeRequest.toLowerCase(),
    changeRequestShort: t('common.gitHost.changeRequestShort.label'),
  };
}

export function installCopyTestHook(): void {
  let pseudoLoaded = false;
  window.__kiraSetLocale = (locale: CopyLocale) => {
    if (locale === 'pseudo' && !pseudoLoaded) {
      i18n.global.setLocaleMessage('pseudo', pseudoMessages(en));
      pseudoLoaded = true;
    }
    // createI18n infers the locale union from `messages`, which holds only en until pseudo loads.
    i18n.global.locale.value = locale as 'en';
  };
}
