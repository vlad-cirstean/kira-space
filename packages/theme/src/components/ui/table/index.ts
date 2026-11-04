export { default as Table } from './Table.vue';
export { default as TableBody } from './TableBody.vue';
export { default as TableCell } from './TableCell.vue';
export { default as TableEmpty } from './TableEmpty.vue';
export { default as TableHead } from './TableHead.vue';
export { default as TableHeader } from './TableHeader.vue';
export { default as TableRow } from './TableRow.vue';

// P263 §2.13: the one table look, also for `grid-cols-[...]` CSS-grid tables that cannot be a
// `<table>` (virtualized lists): spread these strings onto the head row, head cell, row and cell.
export const tableHeadRowClass = 'bg-elevated';
export const tableHeadClass =
  'h-row px-2 text-left align-middle text-kira-sm font-normal whitespace-nowrap text-muted-foreground';
export const tableRowClass = 'h-row hover:bg-field data-[state=selected]:bg-select';
export const tableCellClass = 'px-2 align-middle';
/** Column dividers, only on data tables (`Table columns`). */
export const tableColumnDividerClass =
  'group-data-[columns]/table:border-r group-data-[columns]/table:border-border group-data-[columns]/table:last:border-r-0';
