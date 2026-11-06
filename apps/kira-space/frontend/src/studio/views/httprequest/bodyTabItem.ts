import type { HttpRequestTabState } from '@shared/domain/http';
import { t } from '@theme/i18n';

const isNamed = (f: { enabled: boolean; name: string }): boolean =>
  f.enabled && f.name.trim() !== '';

/** The Body segment's label, with a row count for the two table modes. */
export function bodyTabItem(state: HttpRequestTabState): { label: string; count?: number } {
  const plain = { label: t('common.body.label') };
  switch (state.bodyMode) {
    case 'none':
      return plain;
    case 'raw':
      return { label: t('studio.http.pane.bodyRaw.label') };
    case 'code':
      return {
        label: t(
          state.codeLanguage === 'json'
            ? 'studio.http.pane.bodyJson.label'
            : 'studio.http.pane.bodyCode.label',
        ),
      };
    case 'urlencoded': {
      const count = state.urlEncoded.filter(isNamed).length;
      return count > 0 ? { ...plain, count } : plain;
    }
    case 'formdata': {
      const count = state.formData.filter(isNamed).length;
      return count > 0 ? { ...plain, count } : plain;
    }
    case 'file':
      return state.binaryFile ? { label: t('studio.http.pane.bodyFile.label') } : plain;
  }
}
