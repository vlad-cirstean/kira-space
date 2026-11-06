import type { ColumnDescriptor, TypeClass } from '@shared/protocol/page';

// Shared by tests/perf/grid-scroll.spec.ts and the P165 grid prototype, so both render identical data.

export const ROWS = 10_000;

export const WIDE_TEMPLATE: {
  name: string;
  dataType: string;
  typeClass: TypeClass;
  cell: (i: number) => string | null;
}[] = [
  { name: 'id', dataType: 'integer', typeClass: 'number', cell: (i) => String(i) },
  {
    name: 'uuid',
    dataType: 'uuid',
    typeClass: 'text',
    cell: (i) =>
      `${i.toString(16).padStart(8, '0')}-0000-4000-8000-${(i * 7919).toString(16).padStart(12, '0')}`,
  },
  {
    name: 'name',
    dataType: 'varchar(80)',
    typeClass: 'text',
    cell: (i) => `Customer number ${i} Ltd`,
  },
  {
    name: 'email',
    dataType: 'text',
    typeClass: 'text',
    cell: (i) => (i % 13 === 0 ? null : `user${i}@example-mail.com`),
  },
  { name: 'qty', dataType: 'smallint', typeClass: 'number', cell: (i) => String(i % 900) },
  {
    name: 'big',
    dataType: 'bigint',
    typeClass: 'number',
    cell: (i) => String(9_000_000_000_000 + i * 31),
  },
  {
    name: 'price',
    dataType: 'numeric(20,6)',
    typeClass: 'number',
    cell: (i) => ((i * 37) / 1000).toFixed(6),
  },
  {
    name: 'ratio',
    dataType: 'double precision',
    typeClass: 'number',
    cell: (i) => String(Math.sin(i) * 1000),
  },
  {
    name: 'active',
    dataType: 'boolean',
    typeClass: 'boolean',
    cell: (i) => (i % 2 === 0 ? 'true' : 'false'),
  },
  {
    name: 'created_at',
    dataType: 'timestamptz',
    typeClass: 'temporal',
    cell: (i) => new Date(1_704_067_200_000 + i * 60_000).toISOString(),
  },
  {
    name: 'born',
    dataType: 'date',
    typeClass: 'temporal',
    cell: (i) => new Date(631_152_000_000 + i * 86_400_000).toISOString().slice(0, 10),
  },
  {
    name: 'payload',
    dataType: 'jsonb',
    typeClass: 'json',
    cell: (i) => JSON.stringify({ id: i, tags: ['a', 'b'], nested: { k: i % 5 } }),
  },
  {
    name: 'blob',
    dataType: 'bytea',
    typeClass: 'binary',
    cell: (i) => `0x${(i * 2654435761).toString(16).padStart(16, '0')}`,
  },
  {
    name: 'notes',
    dataType: 'text',
    typeClass: 'text',
    cell: (i) => `Note ${i}: ${'lorem ipsum dolor sit amet '.repeat(3 + (i % 4))}`,
  },
  {
    name: 'country',
    dataType: 'char(2)',
    typeClass: 'text',
    cell: (i) => ['US', 'DE', 'RO', 'JP', 'BR'][i % 5],
  },
  {
    name: 'ip',
    dataType: 'inet',
    typeClass: 'other',
    cell: (i) => `10.${(i >> 8) & 255}.${i & 255}.1`,
  },
  { name: 'score', dataType: 'real', typeClass: 'number', cell: (i) => String((i % 1000) / 7) },
  {
    name: 'status',
    dataType: 'text',
    typeClass: 'text',
    cell: (i) => ['new', 'open', 'closed', 'archived'][i % 4],
  },
  {
    name: 'updated_at',
    dataType: 'timestamp',
    typeClass: 'temporal',
    cell: (i) =>
      new Date(1_714_067_200_000 + i * 90_000).toISOString().replace('T', ' ').slice(0, 19),
  },
  {
    name: 'address',
    dataType: 'text',
    typeClass: 'text',
    cell: (i) => (i % 9 === 0 ? null : `${i} Long Street Name Avenue, Springfield ${i % 99}`),
  },
];

export function wideColumns(ncols: number): ColumnDescriptor[] {
  return WIDE_TEMPLATE.slice(0, ncols).map((c, idx) => ({
    name: c.name,
    dataType: c.dataType,
    typeClass: c.typeClass,
    nullable: idx !== 0,
    isPrimaryKey: idx === 0,
    generated: false,
  }));
}

export function wideRows(ncols: number, rows: number = ROWS): (string | null)[][] {
  const cols = WIDE_TEMPLATE.slice(0, ncols);
  return Array.from({ length: rows }, (_, i) => cols.map((c) => c.cell(i + 1)));
}
