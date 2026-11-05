import { z } from 'zod';

// internal/tableref kinds the column review applies to (maskreview.Applies).
export const MASK_REVIEW_KINDS: ReadonlySet<string> = /*#__PURE__*/ new Set([
  'postgres',
  'mysql',
  'mariadb',
  'clickhouse',
  'sqlite',
]);

// bridge/maskreview.go's MaskRulesService.ReviewColumns.
export const maskReviewListingSchema = /*#__PURE__*/ z.object({
  applicable: z.boolean(),
  columns: /*#__PURE__*/ z.array(
    /*#__PURE__*/ z.object({
      ref: z.string(),
      table: z.string(),
      column: z.string(),
      dataType: z.string(),
      relationKind: z.string(),
    }),
  ),
  truncated: z.boolean(),
  checkedAt: z.number(),
});
export type MaskReviewListing = z.infer<typeof maskReviewListingSchema>;
export type MaskReviewColumn = MaskReviewListing['columns'][number];

export const maskReviewPendingSchema = /*#__PURE__*/ z.object({ count: z.number() });
export type MaskReviewPending = z.infer<typeof maskReviewPendingSchema>;

export type MaskReviewState = 'masked' | 'reviewed' | 'pending';

interface RuleLike {
  table?: string;
  column: string;
  kind: string;
}

// Mirrors maskreview.Covered: a rule covers a column by name (ignoring case) and a table of `*` or the
// column's own table name.
export function maskRuleFor(
  col: MaskReviewColumn,
  rules: readonly RuleLike[],
): RuleLike | undefined {
  const name = col.column.toLowerCase();
  const table = col.table.toLowerCase();
  return rules.find(
    (r) =>
      r.column.toLowerCase() === name &&
      (!r.table || r.table === '*' || r.table.toLowerCase() === table),
  );
}

export function reviewStateOf(
  col: MaskReviewColumn,
  rules: readonly RuleLike[],
  reviewed: ReadonlySet<string>,
): MaskReviewState {
  if (maskRuleFor(col, rules)) return 'masked';
  return reviewed.has(col.ref) ? 'reviewed' : 'pending';
}
