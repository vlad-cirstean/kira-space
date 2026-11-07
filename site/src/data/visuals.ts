// Mini UI fixtures for the bento tiles. Names and values match the screenshot fixtures
// (apps/kira-space/tests/screenshots), never invented.

export const AGENT_ROWS = [
  { name: 'Add order search', stage: 'Implement', color: 'sky', pct: 64, live: true },
  { name: 'Fix grid paging', stage: 'Review', color: 'amber', pct: 88, live: false },
  { name: 'Cron run summary', stage: 'Plan', color: 'violet', pct: 28, live: true },
  { name: 'Docker log tail', stage: 'Release', color: 'lime', pct: 100, live: false },
  { name: 'Phone board polish', stage: 'Implement', color: 'teal', pct: 46, live: true },
  { name: 'Keychain prompt copy', stage: 'Review', color: 'pink', pct: 72, live: false },
] as const;

export const SQL_ROWS = [
  { customer: 'Alice Morgan', orders: '14', revenue: '2318.50', hot: true },
  { customer: 'Ben Carter', orders: '11', revenue: '1904.25', hot: false },
  { customer: 'Chloe Hughes', orders: '9', revenue: '1650.00', hot: false },
] as const;

export const API_BODY = [
  ['id', '4217', 'number'],
  ['customer', '"Alice Morgan"', 'string'],
  ['status', '"shipped"', 'string'],
  ['total', '138.9', 'number'],
] as const;

export const FILES = [
  { name: 'src', dir: true, depth: 0 },
  { name: 'orders.ts', dir: false, depth: 1, open: true },
  { name: 'search.ts', dir: false, depth: 1 },
  { name: 'Grid.vue', dir: false, depth: 1 },
  { name: 'README.md', dir: false, depth: 0 },
] as const;

export const CONTAINERS = [
  { name: 'postgres', tag: '16', up: true },
  { name: 'clickhouse', tag: '24', up: true },
  { name: 'redis', tag: '7', up: false },
] as const;

export const MCP_SWITCHES = [
  { name: 'acme-postgres', on: true },
  { name: 'analytics-clickhouse', on: false },
  { name: 'cache-redis', on: false },
] as const;
