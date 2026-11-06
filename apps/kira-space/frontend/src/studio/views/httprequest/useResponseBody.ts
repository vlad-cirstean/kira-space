import type { HttpResponseView } from '@shared/domain/http';
import { computed, type Ref, ref, shallowRef, watch } from 'vue';
import { INLINE_CHARS, parseInline } from '../../workers/parse/client';
import type { PrettyFormat, PrettyResult } from '../../workers/parse/protocol';
import { useParseWorker } from '../../workers/parse/useParseWorker';

interface BodySource {
  body: string;
  bodyEncoding: string;
}

/**
 * One format pass per received body (worker for large ones). Pretty text is held only while the
 * Pretty view is selected (P21 finding 7): switching to Raw drops it, switching back re-requests it.
 */
export function useResponseBody(response: Ref<BodySource | null>, view: Ref<HttpResponseView>) {
  // `undefined` while the pass is pending.
  const format = ref<PrettyFormat | null | undefined>(undefined);
  const text = shallowRef<string | null>(null);
  const parse = useParseWorker();
  let seq = 0;

  function apply(r: PrettyResult): void {
    format.value = r.format;
    text.value = view.value === 'pretty' ? (r.text ?? null) : null;
  }

  function request(body: string, wantText: boolean): void {
    const id = ++seq;
    const input = { body, wantText };
    if (body.length <= INLINE_CHARS) {
      apply(parseInline('body.format', input));
      return;
    }
    parse
      .runLatest('body', 'body.format', input)
      .then((r) => {
        if (id === seq) apply(r);
      })
      .catch((e) => {
        if (id === seq && !(e instanceof DOMException && e.name === 'AbortError')) {
          apply({ format: null });
        }
      });
  }

  watch(
    response,
    (r) => {
      seq++;
      text.value = null;
      format.value = undefined;
      if (!r) return;
      if (r.bodyEncoding === 'base64') {
        format.value = null;
        return;
      }
      request(r.body, view.value === 'pretty');
    },
    { immediate: true },
  );

  watch(view, (v) => {
    const r = response.value;
    if (v === 'raw') {
      text.value = null;
      return;
    }
    if (!r || r.bodyEncoding === 'base64') return;
    if (format.value === undefined || (format.value !== null && text.value === null)) {
      request(r.body, true);
    }
  });

  /** Pretty view is waiting on its text. */
  const pending = computed(
    () =>
      response.value !== null &&
      response.value.bodyEncoding !== 'base64' &&
      view.value === 'pretty' &&
      (format.value === undefined || (format.value !== null && text.value === null)),
  );
  const bodyText = computed(() => {
    const r = response.value;
    if (!r) return '';
    return view.value === 'pretty' && text.value !== null ? text.value : r.body;
  });

  return { format, bodyText, pending };
}
