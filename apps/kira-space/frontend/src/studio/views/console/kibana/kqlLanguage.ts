import type { MonacoModule } from '@workbench/editor/monaco';

type Monarch = import('monaco-editor').languages.IMonarchLanguage;
type LanguageConfig = import('monaco-editor').languages.LanguageConfiguration;

export const queryLanguageConfig: LanguageConfig = {
  brackets: [
    ['(', ')'],
    ['{', '}'],
  ],
  autoClosingPairs: [
    { open: '(', close: ')' },
    { open: '{', close: '}' },
    { open: '"', close: '"', notIn: ['string'] },
  ],
  surroundingPairs: [
    { open: '(', close: ')' },
    { open: '"', close: '"' },
  ],
};

const kqlMonarch: Monarch = {
  ignoreCase: true,
  tokenizer: {
    root: [
      [/\b(?:and|or|not)\b/, 'keyword'],
      [/"(?:[^"\\]|\\.)*"?/, 'string'],
      // A field is the word right before `:` or a comparison.
      [/[^\s():<>"{}]+(?=\s*(?::|<=?|>=?))/, 'variable.name'],
      [/\d+(?:\.\d+)?(?![^\s():<>"{}])/, 'number'],
      [/<=|>=|<|>|:/, 'operator'],
      [/[(){}]/, '@brackets'],
      [/[^\s():<>"{}]+/, ''],
      [/\s+/, ''],
    ],
  },
};

export function grammar(id: string, monarch: Monarch, cfg: LanguageConfig) {
  return {
    id,
    register(m: MonacoModule): void {
      m.languages.register({ id });
      m.languages.setMonarchTokensProvider(id, monarch);
      m.languages.setLanguageConfiguration(id, cfg);
    },
  };
}

export const kqlMonacoLanguage = grammar('kira-kql', kqlMonarch, queryLanguageConfig);
