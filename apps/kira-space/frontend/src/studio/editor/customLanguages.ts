import type { MonacoModule } from '@workbench/editor/monaco';

const registered = new Set<string>();

/** Register once per page: `register` drops its Monaco disposables, so repeats pile up. */
export function ensureCustomLanguage(
  m: MonacoModule,
  language: { id: string; register(m: MonacoModule): void },
): void {
  if (registered.has(language.id)) return;
  language.register(m);
  registered.add(language.id);
}
