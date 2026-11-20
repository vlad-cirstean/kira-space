// C5 §9.4: the extension-to-language map, shared by the tree row icon (AdeFileTreeRow.vue's own
// small five-bucket lookup is a different, coarser purpose — a glance icon, not a Monaco language
// id) and RepoFileView.vue's model creation — one file, one vocabulary, so the two can never
// disagree about what a given path colors as. Deliberately does NOT import codeindex's own Go-side
// extension table (§9.4's own reasoning: parse coverage and coloring coverage are different sets
// for different purposes; pretending otherwise would drag one to the other's shape).
//
// D3 (P67c §2.3): `monacoEntry.ts` now registers all 84 of monaco-editor's basic languages (up
// from 19), so this table was rebuilt against every `register.js`'s own `extensions: [...]` list
// (`node_modules/monaco-editor/esm/vs/languages/definitions/*/register.js`), not memory — a
// grammar not listed here still opens as 'plaintext' even though Monaco can color it. Two
// deliberate exceptions D9 already recorded stand: JSON colors via its own tokenizer (D1) rather
// than a `definitions/json` that doesn't exist in this Monaco version, and `.vue`/`.svelte` color
// via the HTML grammar (no Monaco grammar exists for either — a `<script>` block therefore colors
// as HTML text, not TypeScript; a hand-written SFC Monarch is declined per CLAUDE.md's
// library-first rule). `toml` keeps its own pre-existing override to `ini` for the same reason
// (no TOML grammar ships). `proto` now maps to Monaco's actual registered id `proto` — the
// previous `protobuf` was never a registered language id at all (`protobuf/register.js` calls
// `registerLanguage({ id: "proto", ... })`), so a `.proto` file silently fell back to plaintext.
//
// OQ-4: this app never asks Monaco to resolve an ambiguous extension (an explicit id is always
// looked up here first), so registration order can't affect coloring — the one place order
// matters is this table itself. Exactly one extension is claimed by two shipped grammars:
// `.pp` (`pascal/register.js` and `ruby/register.js` both list it) — kept on `pascal`, the far
// more common real-world use of that extension; Ruby's own convention is `.rb`.
const EXTENSION_LANGUAGE: Readonly<Record<string, string>> = {
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  js: 'javascript',
  es6: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  vue: 'html',
  svelte: 'html',
  json: 'json',
  jsonc: 'json',
  java: 'java',
  jav: 'java',
  py: 'python',
  rpy: 'python',
  pyw: 'python',
  cpy: 'python',
  gyp: 'python',
  gypi: 'python',
  go: 'go',
  rs: 'rust',
  rlib: 'rust',
  html: 'html',
  htm: 'html',
  shtml: 'html',
  xhtml: 'html',
  mdoc: 'html',
  jsp: 'html',
  asp: 'html',
  aspx: 'html',
  jshtm: 'html',
  css: 'css',
  scss: 'scss',
  less: 'less',
  md: 'markdown',
  markdown: 'markdown',
  mdown: 'markdown',
  mkdn: 'markdown',
  mkd: 'markdown',
  mdwn: 'markdown',
  mdtxt: 'markdown',
  mdtext: 'markdown',
  mdx: 'mdx',
  yaml: 'yaml',
  yml: 'yaml',
  xml: 'xml',
  xsd: 'xml',
  dtd: 'xml',
  xaml: 'xml',
  svg: 'xml',
  svgz: 'xml',
  xslt: 'xml',
  xsl: 'xml',
  ascx: 'xml',
  csproj: 'xml',
  config: 'xml',
  props: 'xml',
  targets: 'xml',
  wxi: 'xml',
  wxl: 'xml',
  wxs: 'xml',
  opf: 'xml',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  sql: 'sql',
  ini: 'ini',
  cfg: 'ini',
  toml: 'ini', // deliberate override — no Monaco TOML grammar exists (D9)
  properties: 'ini',
  gitconfig: 'ini',
  graphql: 'graphql',
  gql: 'graphql',
  proto: 'proto', // D3 fix — the registered id is `proto`, never `protobuf` (see comment above)

  // D3's own 65 newly-reachable grammars, one row per extension in that language's own register.js.
  abap: 'abap',
  cls: 'apex',
  azcli: 'azcli',
  bat: 'bat',
  cmd: 'bat',
  bicep: 'bicep',
  mligo: 'cameligo',
  clj: 'clojure',
  cljs: 'clojure',
  cljc: 'clojure',
  edn: 'clojure',
  coffee: 'coffeescript',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  hh: 'cpp',
  hxx: 'cpp',
  cs: 'csharp',
  csx: 'csharp',
  cake: 'csharp',
  csp: 'csp',
  cypher: 'cypher',
  cyp: 'cypher',
  dart: 'dart',
  dockerfile: 'dockerfile', // also matched by BASENAME_LANGUAGE below for the bare filename
  ecl: 'ecl',
  ex: 'elixir',
  exs: 'elixir',
  flow: 'flow9',
  ftl: 'freemarker2',
  ftlh: 'freemarker2',
  ftlx: 'freemarker2',
  fs: 'fsharp',
  fsi: 'fsharp',
  ml: 'fsharp', // Monaco's F# grammar also claims OCaml's own extensions — no separate OCaml ships
  mli: 'fsharp',
  fsx: 'fsharp',
  fsscript: 'fsharp',
  handlebars: 'handlebars',
  hbs: 'handlebars',
  tf: 'hcl',
  tfvars: 'hcl',
  hcl: 'hcl',
  jl: 'julia',
  kt: 'kotlin',
  kts: 'kotlin',
  lex: 'lexon',
  liquid: 'liquid',
  lua: 'lua',
  m3: 'm3',
  i3: 'm3',
  mg: 'm3',
  ig: 'm3',
  s: 'mips',
  dax: 'msdax',
  msdax: 'msdax',
  m: 'objective-c',
  pas: 'pascal',
  p: 'pascal',
  pp: 'pascal', // OQ-4 — also claimed by ruby's own register.js; see comment above
  ligo: 'pascaligo',
  pl: 'perl',
  pm: 'perl',
  php: 'php',
  php4: 'php',
  php5: 'php',
  phtml: 'php',
  ctp: 'php',
  pla: 'pla',
  dats: 'postiats',
  sats: 'postiats',
  hats: 'postiats',
  pq: 'powerquery',
  pqm: 'powerquery',
  ps1: 'powershell',
  psm1: 'powershell',
  psd1: 'powershell',
  jade: 'pug',
  pug: 'pug',
  qs: 'qsharp',
  r: 'r',
  rhistory: 'r',
  rmd: 'r',
  rprofile: 'r',
  rt: 'r',
  cshtml: 'razor',
  redis: 'redis', // upstream's own basic-language id — distinct from this app's `kira-redis`
  rst: 'restructuredtext',
  rb: 'ruby',
  rbx: 'ruby',
  rjs: 'ruby',
  gemspec: 'ruby',
  sb: 'sb',
  scala: 'scala',
  sc: 'scala',
  sbt: 'scala',
  scm: 'scheme',
  ss: 'scheme',
  sch: 'scheme',
  rkt: 'scheme',
  sol: 'sol',
  aes: 'aes',
  rq: 'sparql',
  st: 'st',
  iecst: 'st',
  iecplc: 'st',
  lc3lib: 'st',
  tcpou: 'st',
  tcdut: 'st',
  tcgvl: 'st',
  tcio: 'st',
  swift: 'swift',
  sv: 'systemverilog',
  svh: 'systemverilog',
  v: 'verilog',
  vh: 'verilog',
  tcl: 'tcl',
  twig: 'twig',
  tsp: 'typespec',
  vb: 'vb',
  wgsl: 'wgsl',
};

// Dockerfile has no extension — matched by basename, mirroring internal/codeworkspace/files.go's
// own languageFor.
const BASENAME_LANGUAGE: Readonly<Record<string, string>> = {
  dockerfile: 'dockerfile',
};

/** The Monaco language id for `path` — the extension only, never sniffed. */
export function monacoLanguageFor(path: string): string {
  const slash = path.lastIndexOf('/');
  const name = (slash < 0 ? path : path.slice(slash + 1)).toLowerCase();
  const byName = BASENAME_LANGUAGE[name];
  if (byName) return byName;
  const dot = name.lastIndexOf('.');
  if (dot < 0) return 'plaintext';
  const ext = name.slice(dot + 1);
  return EXTENSION_LANGUAGE[ext] ?? 'plaintext';
}
