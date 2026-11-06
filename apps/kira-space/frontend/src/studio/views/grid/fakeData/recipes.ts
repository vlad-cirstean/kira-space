import type { ObjectMeta } from '@shared/domain/tree';
import type { ColumnDescriptor, TypeClass } from '@shared/protocol/page';
import { type MessageKey, t } from '@theme/i18n';
import type { ColumnPlan, GeneratorId, Recipe } from './types';

export interface RecipeOption {
  id: GeneratorId;
  labelKey: MessageKey;
  /** Which `typeClass`es this generator is offerable for — both the recipe picker's own filtered
   *  option list and D4 rule 6's own "does the name heuristic agree with the real type" gate. */
  typeClasses: TypeClass[];
}

// D4 rule 6/7's whole generator surface, one row per id — the recipe picker (a <select>) lists
// only the options whose typeClasses include the column's own, and the name-heuristic table below
// only ever proposes an id whose typeClasses agree.
export const RECIPE_CATALOG: RecipeOption[] = [
  {
    id: 'person.fullName',
    labelKey: 'studio.generateData.recipe.personFullName.label',
    typeClasses: ['text'],
  },
  {
    id: 'person.firstName',
    labelKey: 'studio.generateData.recipe.personFirstName.label',
    typeClasses: ['text'],
  },
  {
    id: 'person.lastName',
    labelKey: 'studio.generateData.recipe.personLastName.label',
    typeClasses: ['text'],
  },
  { id: 'internet.email', labelKey: 'common.field.email.label', typeClasses: ['text'] },
  {
    id: 'internet.url',
    labelKey: 'studio.generateData.recipe.internetUrl.label',
    typeClasses: ['text'],
  },
  {
    id: 'phone.number',
    labelKey: 'studio.generateData.recipe.phoneNumber.label',
    typeClasses: ['text'],
  },
  {
    id: 'location.city',
    labelKey: 'studio.generateData.recipe.locationCity.label',
    typeClasses: ['text'],
  },
  {
    id: 'location.country',
    labelKey: 'studio.generateData.recipe.locationCountry.label',
    typeClasses: ['text'],
  },
  {
    id: 'location.state',
    labelKey: 'studio.generateData.recipe.locationState.label',
    typeClasses: ['text'],
  },
  {
    id: 'location.zipCode',
    labelKey: 'studio.generateData.recipe.locationZipCode.label',
    typeClasses: ['text'],
  },
  {
    id: 'location.streetAddress',
    labelKey: 'studio.generateData.recipe.locationStreetAddress.label',
    typeClasses: ['text'],
  },
  {
    id: 'company.name',
    labelKey: 'studio.generateData.recipe.companyName.label',
    typeClasses: ['text'],
  },
  {
    id: 'finance.amount',
    labelKey: 'studio.generateData.recipe.financeAmount.label',
    typeClasses: ['text', 'number'],
  },
  {
    id: 'date.recent',
    labelKey: 'studio.generateData.recipe.dateRecent.label',
    typeClasses: ['temporal'],
  },
  {
    id: 'date.birthdate',
    labelKey: 'studio.generateData.recipe.dateBirthdate.label',
    typeClasses: ['temporal'],
  },
  {
    id: 'date.past',
    labelKey: 'studio.generateData.recipe.datePast.label',
    typeClasses: ['temporal'],
  },
  {
    id: 'lorem.sentence',
    labelKey: 'studio.generateData.recipe.loremSentence.label',
    typeClasses: ['text'],
  },
  {
    id: 'lorem.words',
    labelKey: 'studio.generateData.recipe.loremWords.label',
    typeClasses: ['text'],
  },
  {
    id: 'lorem.slug',
    labelKey: 'studio.generateData.recipe.loremSlug.label',
    typeClasses: ['text'],
  },
  {
    id: 'string.uuid',
    labelKey: 'studio.generateData.recipe.stringUuid.label',
    typeClasses: ['text'],
  },
  {
    id: 'datatype.boolean',
    labelKey: 'studio.generateData.recipe.datatypeBoolean.label',
    typeClasses: ['boolean'],
  },
  {
    id: 'number.int',
    labelKey: 'studio.generateData.recipe.numberInt.label',
    typeClasses: ['number'],
  },
  {
    id: 'json.object',
    labelKey: 'studio.generateData.recipe.jsonObject.label',
    typeClasses: ['json'],
  },
  {
    id: 'binary.hex',
    labelKey: 'studio.generateData.recipe.binaryHex.label',
    typeClasses: ['binary'],
  },
];

const RECIPE_BY_ID = new Map(RECIPE_CATALOG.map((o) => [o.id, o]));

function normalizeName(name: string): string {
  return name.toLowerCase().replace(/[_-]/g, '');
}

interface NameHeuristic {
  test: (name: string) => boolean;
  generatorId: GeneratorId;
}

// The first match wins. Every entry's generator is applied only when `RecipeOption.typeClasses`
// (above) agrees with the column's own `typeClass` (recipeFor's own check), so a `varchar` column
// named `amount` still gets a number-shaped string and an `int` column named `name` does not get a
// person's name.
const NAME_HEURISTICS: NameHeuristic[] = [
  { test: (n) => n === 'email' || n.endsWith('email'), generatorId: 'internet.email' },
  { test: (n) => n === 'firstname', generatorId: 'person.firstName' },
  { test: (n) => n === 'lastname', generatorId: 'person.lastName' },
  { test: (n) => n === 'name', generatorId: 'person.fullName' },
  { test: (n) => n === 'phone' || n === 'tel' || n.endsWith('phone'), generatorId: 'phone.number' },
  { test: (n) => n === 'city', generatorId: 'location.city' },
  { test: (n) => n === 'country', generatorId: 'location.country' },
  { test: (n) => n === 'state', generatorId: 'location.state' },
  { test: (n) => n === 'zip' || n === 'postcode', generatorId: 'location.zipCode' },
  { test: (n) => n === 'address' || n === 'street', generatorId: 'location.streetAddress' },
  { test: (n) => n === 'company' || n === 'org', generatorId: 'company.name' },
  { test: (n) => n === 'url' || n === 'website' || n === 'link', generatorId: 'internet.url' },
  {
    test: (n) => n === 'price' || n === 'amount' || n === 'total' || n === 'cost' || n === 'salary',
    generatorId: 'finance.amount',
  },
  { test: (n) => n === 'createdat' || n === 'updatedat', generatorId: 'date.recent' },
  { test: (n) => n.startsWith('birth'), generatorId: 'date.birthdate' },
  {
    test: (n) =>
      n === 'description' || n === 'comment' || n === 'note' || n === 'bio' || n === 'summary',
    generatorId: 'lorem.sentence',
  },
  { test: (n) => n === 'uuid' || n === 'guid', generatorId: 'string.uuid' },
  { test: (n) => n === 'slug', generatorId: 'lorem.slug' },
  {
    test: (n) => n.startsWith('is') || n.startsWith('has') || n === 'active' || n === 'enabled',
    generatorId: 'datatype.boolean',
  },
];

function fallbackForTypeClass(typeClass: TypeClass): Recipe {
  switch (typeClass) {
    case 'number':
      return { kind: 'faker', generatorId: 'number.int' };
    case 'boolean':
      return { kind: 'faker', generatorId: 'datatype.boolean' };
    case 'temporal':
      return { kind: 'faker', generatorId: 'date.past' };
    case 'json':
      return { kind: 'faker', generatorId: 'json.object' };
    case 'binary':
      return { kind: 'faker', generatorId: 'binary.hex' };
    case 'text':
      return { kind: 'faker', generatorId: 'lorem.words' };
    case 'other':
      return { kind: 'skip' };
  }
}

// D4's seven-rule decision table, stopping at the first match — the "decision structure large
// enough that no one can hold it in their head" CLAUDE.md names outright, and the subject of C4's
// one recipe unit test.
export function recipeFor(descriptor: ColumnDescriptor, meta: ObjectMeta | null): ColumnPlan {
  // rule 1: a GENERATED/computed column — the server fills it in (P36 D28).
  if (descriptor.generated) return { column: descriptor, recipe: { kind: 'skip' } };

  const columnMeta = meta?.columns.find((c) => c.name === descriptor.name) ?? null;
  const hasDefault = columnMeta?.defaultExpr != null;

  // rule 2: a serial/identity/AUTO_INCREMENT primary key — supplying a value guarantees a
  // collision with whatever the sequence assigns next.
  if (descriptor.isPrimaryKey && hasDefault) {
    return { column: descriptor, recipe: { kind: 'skip' } };
  }

  // rules 3/4: a primary key with no default is the one column a generated value must not
  // collide on — `sequence`/`string.uuid` are the only recipes unique by construction (F11).
  if (descriptor.isPrimaryKey && !hasDefault) {
    return {
      column: descriptor,
      recipe:
        descriptor.typeClass === 'number'
          ? { kind: 'sequence', start: 1 }
          : { kind: 'faker', generatorId: 'string.uuid' },
    };
  }

  // rule 5: a foreign key column — its valid values live in another table (F12), out of reach
  // here. Skipped either way; a NOT-NULL one earns a warning (planWarnings, below).
  const isForeignKey = (meta?.foreignKeys ?? []).some((fk) => fk.columns.includes(descriptor.name));
  if (isForeignKey) {
    return { column: descriptor, recipe: { kind: 'skip' } };
  }

  // rule 6: a name heuristic, applied only when it agrees with the column's real type.
  const normalized = normalizeName(descriptor.name);
  for (const heuristic of NAME_HEURISTICS) {
    if (!heuristic.test(normalized)) continue;
    const option = RECIPE_BY_ID.get(heuristic.generatorId);
    if (option?.typeClasses.includes(descriptor.typeClass)) {
      return { column: descriptor, recipe: { kind: 'faker', generatorId: heuristic.generatorId } };
    }
    break; // the first matching name never falls through to a later, weaker heuristic
  }

  // rule 7: the typeClass fallback.
  return { column: descriptor, recipe: fallbackForTypeClass(descriptor.typeClass) };
}

// D9's three warning classes, recomputed from the *current* plan (including whatever the user has
// overridden) rather than baked into recipeFor's own one-time proposal.
export function planWarnings(plans: ColumnPlan[], meta: ObjectMeta | null): string[] {
  const fkColumns = new Set((meta?.foreignKeys ?? []).flatMap((fk) => fk.columns));
  const uniqueColumns = new Set(
    (meta?.indexes ?? []).filter((idx) => idx.unique).flatMap((idx) => idx.columns),
  );
  const warnings: string[] = [];

  for (const plan of plans) {
    const { column, recipe } = plan;
    const columnMeta = meta?.columns.find((c) => c.name === column.name);
    const isIntentionalSkip =
      column.generated ||
      (column.isPrimaryKey && columnMeta?.defaultExpr != null) ||
      fkColumns.has(column.name);

    if (fkColumns.has(column.name) && !column.nullable && recipe.kind === 'skip') {
      warnings.push(
        t('studio.generateData.warning.requiredForeignKey.hint', { column: column.name }),
      );
    }
    if (uniqueColumns.has(column.name)) {
      // F11: even a unique-by-construction recipe (sequence/string.uuid) is only unique within
      // this run — nothing client-side can know what the table already holds.
      warnings.push(t('studio.generateData.warning.uniqueIndex.hint', { column: column.name }));
    }
    if (!column.nullable && recipe.kind === 'skip' && !isIntentionalSkip) {
      warnings.push(t('studio.generateData.warning.requiredNoValue.hint', { column: column.name }));
    }
  }
  return warnings;
}
