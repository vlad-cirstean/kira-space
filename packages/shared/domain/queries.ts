import { z } from 'zod';

export type SortDirection = 'asc' | 'desc';
export const sortDirectionSchema = /*#__PURE__*/ z.enum(['asc', 'desc']);

/**
 * The two sort editors, one state. Clicking a grid header produces the structured
 * form and mirrors its equivalent string into the `ORDER BY` box; typing in the box switches
 * to the text form and clears the header indicators. Only the structured form is machine
 * readable enough to drive keyset pagination — a `text` sort is always `offset`.
 */
export const sortSpecSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  /*#__PURE__*/ z.object({
    kind: z.literal('structured'),
    terms: /*#__PURE__*/ z.array(
      /*#__PURE__*/ z.object({ column: z.string(), direction: sortDirectionSchema }),
    ),
  }),
  /*#__PURE__*/ z.object({
    kind: z.literal('text'),
    text: z.string().max(4096),
  }),
]);
export type SortSpec = z.infer<typeof sortSpecSchema>;

export const filterBodySchema = /*#__PURE__*/ z.object({
  where: z.string().nullable(),
  orderBy: sortSpecSchema.nullable(),
});
export type FilterBody = z.infer<typeof filterBodySchema>;

// §8.14: "Console contents are saved to saved_queries" — just the script text, no result state.
export const consoleBodySchema = /*#__PURE__*/ z.object({
  text: z.string(),
});
export type ConsoleBody = z.infer<typeof consoleBodySchema>;

const savedQueryBase = {
  id: z.string(),
  connectionId: z.string(),
  path: z.string(),
  name: z.string().trim().min(1).max(120),
  pinned: z.boolean(),
  createdAt: z.string(),
  usedAt: z.string().nullable(),
};

export const savedQuerySchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  /*#__PURE__*/ z.object({ ...savedQueryBase, kind: z.literal('filter'), body: filterBodySchema }),
  /*#__PURE__*/ z.object({
    ...savedQueryBase,
    kind: z.literal('console'),
    body: consoleBodySchema,
  }),
]);
export type SavedQuery = z.infer<typeof savedQuerySchema>;
export type SavedFilterQuery = Extract<SavedQuery, { kind: 'filter' }>;
export type SavedConsoleQuery = Extract<SavedQuery, { kind: 'console' }>;

export const filterHistoryEntrySchema = /*#__PURE__*/ z.object({
  id: z.string(),
  connectionId: z.string(),
  path: z.string(),
  where: z.string().nullable(),
  orderBy: sortSpecSchema.nullable(),
  usedAt: z.string(),
});
export type FilterHistoryEntry = z.infer<typeof filterHistoryEntrySchema>;
