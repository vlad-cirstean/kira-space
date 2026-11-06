import type { HttpCookieWire } from '@shared/domain/http';
import { useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { refreshApiQuery } from '../../api/state/apiQueries';
import { control } from '../../bridge/control';

// What the shared jar would send for a URL right now (CookiesPane request mode and the
// Cookies segment badge), as TanStack Query server state. The jar is process-wide, so lists are
// keyed by URL, never by tab. the default staleTime (Infinity) — only a send, Remove or Clear
// changes the jar.
// The lazy rule is `enabled`: a hidden tab is unmounted (no observer), a jar-off pane never
// fetches, and invalidation refetches only a mounted, enabled observer.

const JAR_COOKIES_KEY = ['httpJarCookies'] as const;

function jarCookiesKey(url: string) {
  return [...JAR_COOKIES_KEY, url] as const;
}

/** A URL that fails to parse (e.g. still carries an unresolved `{{host}}` template) is not an
 *  error worth surfacing: the pane just shows no cookies until the URL resolves, like an empty
 *  jar. Every other failure throws and shows in the pane. */
async function fetchJarCookies(url: string): Promise<HttpCookieWire[]> {
  try {
    return await control.httpCookies(url);
  } catch (err) {
    if ((err as { code?: string }).code === 'E_BAD_REQUEST') return [];
    throw err;
  }
}

export function useJarCookies(url: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
  return useQuery(() => {
    const key = toValue(url);
    return {
      queryKey: jarCookiesKey(key),
      queryFn: () => fetchJarCookies(key),
      enabled: toValue(enabled) && key !== '',
    };
  }, queryClient);
}

/** Refetches every cached URL's list that has a mounted, enabled observer; the rest turn stale.
 *  `except` skips a URL whose list the caller just set. */
export async function invalidateJarCookies(except?: string): Promise<void> {
  const keys = queryClient.getQueryCache().findAll({ queryKey: JAR_COOKIES_KEY });
  await Promise.all(
    keys.filter((q) => q.queryKey[1] !== except).map((q) => refreshApiQuery(q.queryKey)),
  );
}

/** Cancels first so a list fetch already in flight cannot land after the delete and resurrect the
 *  cookie; the reply is the jar's truth for `url`, every other URL's list is refreshed. */
export async function deleteJarCookie(url: string, cookie: HttpCookieWire): Promise<void> {
  const cookies = await control.httpDeleteCookie(url, {
    name: cookie.name,
    domain: cookie.domain,
    path: cookie.path,
  });
  await queryClient.cancelQueries({ queryKey: JAR_COOKIES_KEY });
  queryClient.setQueryData(jarCookiesKey(url), cookies);
  await invalidateJarCookies(url);
}

/** The jar is process-wide, so after a clear `[]` is exact for every cached URL. */
export async function clearJarCookies(): Promise<void> {
  await control.httpClearCookies();
  await queryClient.cancelQueries({ queryKey: JAR_COOKIES_KEY });
  queryClient.setQueriesData({ queryKey: JAR_COOKIES_KEY }, []);
}
