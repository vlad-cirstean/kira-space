type Monarch = import('monaco-editor').languages.IMonarchLanguage;

import { grammar, queryLanguageConfig } from './kqlLanguage';

const luceneMonarch: Monarch = {
  tokenizer: {
    root: [
      [/\b(?:AND|OR|NOT|TO)\b/, 'keyword'],
      [/"(?:[^"\\]|\\.)*"?/, 'string'],
      [/\/(?:[^/\\]|\\.)+\/?/, 'string.escape'],
      [/[^\s()[\]{}:"+\-~^/]+(?=:)/, 'variable.name'],
      [/\d+(?:\.\d+)?(?![^\s()[\]{}:"~^])/, 'number'],
      [/[+\-!]/, 'operator'],
      [/[~^]\d*(?:\.\d+)?/, 'operator'],
      [/:/, 'operator'],
      [/[()[\]{}]/, '@brackets'],
      [/[^\s()[\]{}:"+\-~^/]+/, ''],
      [/\s+/, ''],
    ],
  },
};

export const luceneMonacoLanguage = grammar('kira-lucene', luceneMonarch, {
  ...queryLanguageConfig,
  brackets: [
    ['(', ')'],
    ['[', ']'],
    ['{', '}'],
  ],
});
