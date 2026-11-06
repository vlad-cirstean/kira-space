// The sole point of contact with `@faker-js/faker`, reached only through a dynamic
// `import()` in generate.ts — never a static import at the top of a module Vite includes in the
// boot bundle. `en`'s locale data is one non-tree-shakeable object (444 KB raw / 152 KB gzip,
// four times sql-formatter's chunk), so this file exists purely to give that cost its own emitted
// chunk rather than folding it into index-*.js. The `/locale/en` subpath makes the single-locale
// intent explicit at the import (it costs the same as the bare package).
export { faker } from '@faker-js/faker/locale/en';
