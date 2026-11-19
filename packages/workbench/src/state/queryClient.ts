import { QueryClient } from '@tanstack/vue-query';

// Query key convention: a flat `[domain, ...ids]` tuple — `['schema', connectionId]`,
// `['maskRules', connectionId]`. Keep new keys to this shape.
//
// One factory, two profiles. Both treat cached data as fresh forever (`staleTime: Infinity`): reads
// are push-invalidated (Wails events on desktop, SSE on the phone), so a mount never refetches.
// A query that must refetch on mount (a poll, a preview of local state) says `staleTime: 0` itself.
//
// - desktop (`remote: false`): every query resolves over the Wails bridge to the local Go process,
//   so a failure is a real backend error, not a transient one worth repeating; window focus changes
//   nothing. Ignores the webview's network state (`networkMode: 'always'`, no reconnect refetch):
//   `onlineManager` flips on a window `offline` event that says nothing about the bridge.
// - remote (`remote: true`, the phone UI over HTTP): one retry, and a refocus or reconnect refetches.
//   Pauses while the browser is offline (`online`) and resumes on reconnect; writes carry
//   idempotency keys.
// `gcTime` for a key with bounded cardinality: a singleton, an app-wide list, or a key removed when
// its owner (tab, item, collection) closes or goes away. Every use names its removal. Input-keyed
// keys (completions, previews, search) keep the 5 min default: under this they grow without bound.
export const KEEP_WHILE_OPEN = Number.POSITIVE_INFINITY;

export function createQueryClient({ remote }: { remote: boolean }): QueryClient {
  const networkMode = remote ? 'online' : 'always';
  return new QueryClient({
    defaultOptions: {
      queries: remote
        ? {
            networkMode,
            retry: 1,
            refetchOnWindowFocus: true,
            refetchOnReconnect: true,
            staleTime: Number.POSITIVE_INFINITY,
          }
        : {
            networkMode,
            retry: false,
            refetchOnWindowFocus: false,
            refetchOnReconnect: false,
            staleTime: Number.POSITIVE_INFINITY,
          },
      mutations: { networkMode },
    },
  });
}

// The desktop app's client, exported so non-component code (Pinia store bodies, signal installers)
// can invalidate; main.ts hands it to VueQueryPlugin rather than letting the plugin construct its own.
export const queryClient = createQueryClient({ remote: false });
