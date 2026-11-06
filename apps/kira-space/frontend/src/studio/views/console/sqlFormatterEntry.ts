// The sole point of contact with `sql-formatter`, reached only through a dynamic
// `import()` in format.ts — never a static import at the top of a module Vite includes in the
// boot bundle. A static re-export here still lets Rollup tree-shake this app's five reachable
// dialects out of the library's other sixteen; an inline `await import('sql-formatter')`
// instead would leave that tree-shake to Rollup's analysis of a dynamic namespace object, which
// measured twice the bundle cost (the 75 KB row vs. this file's 38 KB one).
export { clickhouse, formatDialect, mariadb, mysql, postgresql, sqlite } from 'sql-formatter';
