// The diff tree's own per-language seti-ui icon rule, ported via git-ui's "./icons"
// subpath export — NOT a re-export from @kira/git-ui's own index.ts, which pulls in App.vue,
// main.ts, SlickGrid and the whole graph layer (gitUiModule.ts's own precedent for keeping
// that out of anything eager). setiFileIcon.ts imports `seti-icons` and nothing else, so this stays
// a two-line copy of FileTree.vue's own `fileIconStyle`, not a reimplementation of its rule.
//
// Shared by AdeFileTreeRow.vue and AdeFileSearchRow.vue so a file explorer and a search result list agree
// on one icon per language, rather than the tree gaining per-language icons while search keeps its
// one generic glyph.
import { setiIconFor } from '@kira/git-ui/icons';

export function fileIconStyle(path: string): Record<string, string> {
  const icon = setiIconFor(path);
  return { maskImage: icon.maskUrl, WebkitMaskImage: icon.maskUrl, backgroundColor: icon.color };
}
