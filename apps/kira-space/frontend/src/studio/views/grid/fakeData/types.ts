import type { ColumnDescriptor } from '@shared/protocol/page';

// P15: the generator ids `recipes.ts`'s catalogue and name heuristics resolve to — an id, not a
// faker call, so this file (and recipeFor's whole decision table) never imports `@faker-js/faker`
// (D2 — only generate.ts does, behind the lazy chunk).
export type GeneratorId =
  | 'person.fullName'
  | 'person.firstName'
  | 'person.lastName'
  | 'internet.email'
  | 'internet.url'
  | 'phone.number'
  | 'location.city'
  | 'location.country'
  | 'location.state'
  | 'location.zipCode'
  | 'location.streetAddress'
  | 'company.name'
  | 'finance.amount'
  | 'date.recent'
  | 'date.birthdate'
  | 'date.past'
  | 'lorem.sentence'
  | 'lorem.words'
  | 'lorem.slug'
  | 'string.uuid'
  | 'datatype.boolean'
  | 'number.int'
  | 'json.object'
  | 'binary.hex';

// `skip` (never sent — a generated column, a defaulted PK, an unresolved FK, or an explicit user
// choice), `null` (an explicit SQL NULL), `constant` (fixed text, the deterministic recipe for
// tests/ui), `sequence` (an editable-start integer counter, the only client-side
// unique-by-construction number) and `faker` (a named generator, resolved to a call only in
// generate.ts).
export type Recipe =
  | { kind: 'skip' }
  | { kind: 'null' }
  | { kind: 'constant'; value: string }
  | { kind: 'sequence'; start: number }
  | { kind: 'faker'; generatorId: GeneratorId };

export interface ColumnPlan {
  column: ColumnDescriptor;
  recipe: Recipe;
  /** Set by `planWarnings` (D9), not by `recipeFor`'s own proposal — recomputed on every plan
   *  change since overriding one column's recipe can change another's warning (e.g. the FK case). */
  warning?: string;
}
