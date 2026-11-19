# Third-Party Notices

This file lists third-party icon assets bundled with Kira Studio's UI, and any dependency whose
license terms call for a notice beyond the standard MIT/BSD/Apache-2.0 attribution `go.mod`/
`package.json` already carry.

## simple-icons

Kira Studio imports database/service engine marks (Postgres, MariaDB, MySQL, SQLite, MongoDB,
Redis, Apache Kafka, ClickHouse) and the Kubernetes mark (Docker module origin icons) from the [Simple Icons](https://simpleicons.org/) icon
set (`simple-icons` on npm), via `import { siX } from 'simple-icons'` and each icon's `path` and
`hex` fields.

Simple Icons' own code and icon data are dedicated to the public domain under CC0 1.0 Universal:

```
Creative Commons Legal Code

CC0 1.0 Universal
```

The full legal text is reproduced in `node_modules/simple-icons/LICENSE.md` and at
<https://creativecommons.org/publicdomain/zero/1.0/legalcode>; no attribution is legally required
under CC0, though it is credited here regardless.

One icon carries an additional, more specific license: Apache Kafka's mark is noted by Simple
Icons as licensed under Apache-2.0 (<https://spdx.org/licenses/Apache-2.0>).

The individual marks reproduced through Simple Icons (Postgres, MariaDB, MySQL, SQLite, MongoDB,
Redis, Apache Kafka, ClickHouse) remain trademarks of their respective owners. They are
used here solely to identify the corresponding database/service engine in the connection UI, not
to imply endorsement by, or affiliation with, those trademark holders.

Amazon SQS and Amazon S3 are not available in Simple Icons (Amazon does not publish per-service
icon marks under a redistributable license there); their icons in Kira Studio are original,
hand-drawn glyphs, not reproductions of Amazon's trademarks.

## seti-icons / seti-ui

Kira Space's file tree (`FileTree.vue`) renders per-language file icons from
[`seti-icons`](https://www.npmjs.com/package/seti-icons) (npm), via
`import { themeIcons } from 'seti-icons'`. `seti-icons` repackages the icon set and colour palette
from [jesseweed/seti-ui](https://github.com/jesseweed/seti-ui) — the same upstream VS Code's own
built-in Seti theme (`vs-seti`) derives from — as plain, tracked JSON/JS data, with no vendored
binary font.

Both `seti-icons` and `seti-ui` are licensed under the MIT License:

```
MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
associated documentation files (the "Software"), to deal in the Software without restriction,
including without limitation the rights to use, copy, modify, merge, publish, distribute,
sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or
substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT
OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

Copyright for `seti-icons` belongs to Elvis Wolcott; copyright for the underlying `seti-ui` icon
set and palette belongs to the Seti UI contributors (Jesse Weed and contributors).

## monaco-editor

Kira Studio's native code-viewing workspace (C5, `views/repo/`) renders opened repository files
read-only with [`monaco-editor`](https://www.npmjs.com/package/monaco-editor) (npm, pinned
0.56.0) — the editor component that also powers Visual Studio Code, reached only through
`views/repo/monacoEntry.ts`'s own dynamic `import()` boundary (studio/api sessions never download
this chunk). Its bundled icon font, `codicon.ttf` (pulled in transitively by
`monaco-editor/features/register.all.js`'s own `codicon.css`), ships and is credited here the same
way `seti-icons` is above — a second, independent copy of the codicon glyphs from this app's own
`@vscode/codicons` dependency (CodiconIcon.vue), since Monaco's internal widgets (the find toolbar,
inline suggestions) reference their own bundled copy rather than this app's.

`monaco-editor` is licensed under the MIT License:

```
The MIT License (MIT)

Copyright (c) 2016 - present Microsoft Corporation

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## fuzzysort

Kira Studio's quick open (C9, ⌘P) ranks repository files by fuzzy subsequence match with
[`fuzzysort`](https://www.npmjs.com/package/fuzzysort) (npm, pinned 4.0.2), a direct dependency
statically imported (`repo/state/quickOpen.ts`) — zero transitive dependencies of its own.

`fuzzysort` is licensed under the MIT License:

```
MIT License

Copyright (c) 2018 Stephen Kamenar

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## @xterm/xterm

Kira Studio's embedded terminal (P83, `views/repo/`) renders a worktree's own shell with
[`@xterm/xterm`](https://www.npmjs.com/package/@xterm/xterm) (npm, pinned 6.0.0) plus its
`@xterm/addon-fit` companion (pinned 0.11.0) — the terminal emulator that also powers Visual
Studio Code's integrated terminal, reached only through `views/repo/terminalRenderer.ts`'s own
dynamic `import()` boundary (`monaco-editor`'s own precedent, above: studio/api sessions never
download this chunk).

`@xterm/xterm` and `@xterm/addon-fit` are licensed under the MIT License:

```
Copyright (c) 2017-2019, The xterm.js authors (https://github.com/xtermjs/xterm.js)
Copyright (c) 2014-2016, SourceLair Private Company (https://www.sourcelair.com)
Copyright (c) 2012-2013, Christopher Jeffrey (https://github.com/chjj/)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## docker/cli

Docker module dials `ssh://` hosts through `github.com/docker/cli/cli/connhelper`
(Apache-2.0). Upstream `NOTICE`:

```
Docker
Copyright 2012-2017 Docker, Inc.

This product includes software developed at Docker, Inc. (https://www.docker.com).

This product contains software (https://github.com/creack/pty) developed
by Keith Rarick, licensed under the MIT License.

The following is courtesy of our legal counsel:

Use and transfer of Docker may be subject to certain restrictions by the
United States and other governments.
It is your responsibility to ensure that your use and/or transfer does not
violate applicable laws.

For more information, see https://www.bis.doc.gov

See also https://www.apache.org/dev/crypto.html and/or seek legal counsel.
```

## anser

Docker log view parses ANSI colour with `anser` (MIT).

```
The MIT License (MIT)

Copyright (c) 2012-25 Ionică Bizău <bizauionica@gmail.com> (https://ionicabizau.net)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## SQL parsers for the DB MCP table allow list

Kira Studio parses SQL in Go to find the tables a DB MCP statement touches (P532). The parsers are
pure Go or WebAssembly, with no cgo:

- `github.com/wasilibs/go-pgquery` (MIT) runs libpg_query as WebAssembly on
  `github.com/tetratelabs/wazero` (Apache-2.0). libpg_query is BSD-3-Clause and embeds PostgreSQL
  source (PostgreSQL License). The notices below ship with the binary.
- `github.com/pingcap/tidb/pkg/parser` (Apache-2.0): MySQL and MariaDB.
- `github.com/AfterShip/clickhouse-sql-parser` (MIT): ClickHouse.
- `github.com/rqlite/sql` (MIT): SQLite.

wazero notice (Apache-2.0 section 4(d)):

```
wazero
Copyright 2020-2023 wazero authors
```

libpg_query (pg_query), from go-pgquery's `NOTICE.txt`:

```
Copyright (c) 2015, Lukas Fittl <lukas@fittl.com>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.

* Neither the name of pg_query nor the names of its contributors may be used
to endorse or promote products derived from this software without specific
prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.
```

PostgreSQL source embedded in libpg_query (PostgreSQL License):

```
PostgreSQL Database Management System
(formerly known as Postgres, then as Postgres95)

Portions Copyright (c) 1996-2025, The PostgreSQL Global Development Group

Portions Copyright (c) 1994, The Regents of the University of California

Permission to use, copy, modify, and distribute this software and its
documentation for any purpose, without fee, and without a written agreement
is hereby granted, provided that the above copyright notice and this
paragraph and the following two paragraphs appear in all copies.

IN NO EVENT SHALL THE UNIVERSITY OF CALIFORNIA BE LIABLE TO ANY PARTY FOR
DIRECT, INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES, INCLUDING
LOST PROFITS, ARISING OUT OF THE USE OF THIS SOFTWARE AND ITS
DOCUMENTATION, EVEN IF THE UNIVERSITY OF CALIFORNIA HAS BEEN ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.

THE UNIVERSITY OF CALIFORNIA SPECIFICALLY DISCLAIMS ANY WARRANTIES,
INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS FOR A PARTICULAR PURPOSE. THE SOFTWARE PROVIDED HEREUNDER IS ON AN
"AS IS" BASIS, AND THE UNIVERSITY OF CALIFORNIA HAS NO OBLIGATIONS TO
PROVIDE MAINTENANCE, SUPPORT, UPDATES, ENHANCEMENTS, OR MODIFICATIONS.
```

## goja (JS engine for MCP scripts)

Kira Studio runs the JavaScript the API and DB MCP tools accept (P675) with goja, a pure-Go
ECMAScript engine, in a child process of the app binary. Modules linked in:

- `github.com/dop251/goja` (MIT), pinned at commit `8dfece3d1e1d717c1bfdc9be25186a7be06a5f12`.
- `github.com/dlclark/regexp2/v2` (MIT), goja's regular expression engine.
- `github.com/go-sourcemap/sourcemap` (BSD-2-Clause), linked by goja's parser; source maps are
  disabled at run time.
- `github.com/google/pprof/profile` (Apache-2.0, no NOTICE file), linked by goja's profiler; never
  started.
- `golang.org/x/text` (BSD-3-Clause), also used elsewhere in the app.

goja's `ftoa` package is a port of Rhino's `DToA.java` (David M. Gay's dtoa), and Rhino's file is
under the Mozilla Public License 2.0. The `ftoa` files are used unmodified. Their source is public
at <https://github.com/dop251/goja/tree/8dfece3d1e1d717c1bfdc9be25186a7be06a5f12/ftoa>; any patch to
them must be published under the MPL-2.0.

goja `LICENSE` (MIT):

```
Copyright (c) 2016 Dmitry Panov

Copyright (c) 2012 Robert Krimen

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
documentation files (the "Software"), to deal in the Software without restriction, including without limitation
the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

goja `ftoa/LICENSE_LUCENE` (Gay's dtoa, Lucent Technologies permission notice):

```
Copyright (C) 1998, 1999 by Lucent Technologies
All Rights Reserved

Permission to use, copy, modify, and distribute this software and
its documentation for any purpose and without fee is hereby
granted, provided that the above copyright notice appear in all
copies and that both that the copyright notice and this
permission notice and warranty disclaimer appear in supporting
documentation, and that the name of Lucent or any of its entities
not be used in advertising or publicity pertaining to
distribution of the software without specific, written prior
permission.

LUCENT DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE,
INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS.
IN NO EVENT SHALL LUCENT OR ANY OF ITS ENTITIES BE LIABLE FOR ANY
SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER
IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
```

goja `ftoa/internal/fast/LICENSE_V8` (port of V8 `fast-dtoa.cc`, BSD-3-Clause):

```
Copyright 2014, the V8 project authors. All rights reserved.
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above
      copyright notice, this list of conditions and the following
      disclaimer in the documentation and/or other materials provided
      with the distribution.
    * Neither the name of Google Inc. nor the names of its
      contributors may be used to endorse or promote products derived
      from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

regexp2 `ATTRIB`, parts ported from dotnet/corefx (MIT):

```
dotnet/corefx was released under this license:

The MIT License (MIT)

Copyright (c) Microsoft Corporation

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

go-sourcemap `LICENSE` (BSD-2-Clause):

```
Copyright (c) 2016 The github.com/go-sourcemap/sourcemap Contributors.
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## GitHub GraphQL schema (test fixture)

`apps/kira-space/internal/githost/github/testdata/github-schema.docs.graphql` is a copy of GitHub's
published public GraphQL schema, fetched from
<https://docs.github.com/public/fpt/schema.docs.graphql> (source: the `github/docs` repository).
Content in `github/docs` is licensed CC-BY-4.0 (<https://creativecommons.org/licenses/by/4.0/>);
the copy is unmodified and used only by `schema_test.go` to validate Kira's GraphQL documents.
Refresh it by downloading the same URL again.

## gqlparser

`github.com/vektah/gqlparser/v2` (MIT, with its dependency `github.com/agnivade/levenshtein`, MIT)
validates GraphQL documents in `schema_test.go`. Test-only; not shipped in the app binary.
