// EditorLanguageId lifted out of frontend/src/editor/languages.ts, whose module body
// imports four CodeMirror packages — a plain type-only import erases at build time
// (verbatimModuleSyntax), so there was no runtime coupling, but a *package* still cannot
// type-check against a file it cannot resolve (packages/api-core needs this type for
// views/httprequest/body.ts, which is not otherwise CodeMirror-coupled at all). Union,
// re-exported from editor/languages.ts so nothing else in the app has to change its import.
export type EditorLanguageId =
  | 'json'
  | 'xml'
  | 'sql'
  | 'mongo'
  | 'redis'
  | 'yaml'
  | 'markdown'
  | 'plain';
