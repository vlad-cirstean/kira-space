import type { FieldSpec } from '@workbench/config/configForm';

// Every leaf the repo form writes. A script write unsets its exclusive run partner in the same
// edit; keys not listed (graph, stash, review, ...) are never sent, so they stay in the file.
export const REPO_FIELDS: readonly FieldSpec[] = [
  { path: ['nickname'], unsetWhenEmpty: true },
  {
    path: ['worktree', 'prepare_script'],
    unsetWhenEmpty: true,
    clears: [['worktree', 'prepare_run']],
  },
  { path: ['worktree', 'prepare_timeout'], unsetWhenEmpty: true },
  { path: ['worktree', 'base_path'], unsetWhenEmpty: true },
  { path: ['integration_branches'], unsetWhenEmpty: true },
  { path: ['environments'], unsetWhenEmpty: true },
];

/** Pointers the form claims; a problem outside them shows on top. */
export const REPO_FORM_POINTERS = [
  '/nickname',
  '/worktree',
  '/integration_branches',
  '/environments',
] as const;
