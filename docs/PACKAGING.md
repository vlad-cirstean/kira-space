# Packaging: local build and verification

See the [README](../README.md) for what the app is and how to run it.

Kira Studio ships as an unsigned (ad-hoc), local, arm64-only macOS build in v1 — see SPEC.md §1/§3.

Packaging is Wails v3's own [Task](https://taskfile.dev)-based pipeline (P57): config lives in
`apps/kira-studio/build/config.yml`, the task graph in `apps/kira-studio/Taskfile.yml` and `apps/kira-studio/build/darwin/Taskfile.yml`,
and entry points in the repo-root `package.json`'s `scripts` plus `scripts/sign-bundle.sh` and
`scripts/verify-packaging.sh`. There is no `electron-builder.yml`, no asar, and no native-module ABI
rebuild step — all deleted with Electron in P57 M7. **As of P58f M10, there is no vendored Node
runtime and no bundled Node engine either** — `scripts/vendor-node.sh` is deleted outright, and
every database adapter is served in-process by the Go binary itself.

## 1. Building locally

Requires macOS arm64, Bun, Go (`go.mod`: 1.27.0), and Xcode command-line tools. Everything else —
`bun install`, `go mod download`, and `wails3` CLI at the version `go.mod` pins — installs
itself:

```sh
bun run package:studio          # installs everything first, then wails3 task darwin:package:dmg, then scripts/sign-bundle.sh
```

`bun run package:studio` and `bun run dev:studio` both run `bun run setup` first (wired as `prepackage:studio`/`predev:studio`),
which is `scripts/setup.sh`, the one entry point for everything a fresh clone needs before building.
It sources `scripts/lib.sh` for shared `ROOT_DIR`, `require_cmd`, `ensure_gopath_on_path`,
`sha256_file` and `pinned_wails_version`/`go_directive` helpers, then:

- runs `bun install` and `go mod download`;
- installs the pinned `wails3` CLI **with `GOTOOLCHAIN` pinned to `go.mod`'s own `go` directive** —
  never `auto`, which resolves the toolchain from Wails' own floor and silently degrades the
  bindings generator's type-checker (`setup.sh:64-71`, `:88-92`);
  it reinstalls only when the pinned version changed, or when the installed binary's own build
  toolchain is older than the directive (`setup.sh:73-80`);
- delegates bindings regeneration to `wails3 task common:generate:bindings`, **for both this app
  and Kira Space** (`apps/kira-space`, as of v1.9 P100) — a loop over the two app dirs, each gated
  on its own identity stamp (`apps/<app>/.task/bindings.stamp`) and its own bindings directory's
  presence (`setup.sh`'s bindings-regeneration block) — `apps/kira-studio/frontend/src/bridge/*.ts`
  (and Kira Space's own equivalent) import their generated bindings directly, so `bun run build:studio`
  fails without them.

`setup.sh` does only what is missing on each run. To run just the install step (e.g. to warm up a
machine before writing code), use `bun run setup`.
`apps/kira-studio/{bin,frontend/dist,frontend/bindings}` are gitignored (`apps/kira-studio/.gitignore`).

Expected artifacts (nothing lands in `dist/` or `out/` any more):

- `apps/kira-studio/bin/Kira Studio.dmg` — **the shipped artifact** (P10): the styled, ad-hoc-signed
  disk image holding the app and an `/Applications` shortcut to drag it onto.
- `apps/kira-studio/bin/Kira Studio.app` — the packaged, ad-hoc-signed bundle the image is built
  from. Still produced, and still what you run locally; it is no longer what ships.
- `apps/kira-studio/bin/Kira Studio` — the bare Go binary the bundle is assembled around.

**No app artifact carries a `.vsix`.** The VS Code extension was removed in P243 Part 2.

No `.zip` is produced anywhere any more — the release workflow uploads the `.dmg` (§7).

**Bundle layout** (`apps/kira-studio/build/darwin/Taskfile.yml`'s `create:app:bundle`):

```
apps/kira-studio/bin/Kira Studio.app/
  Contents/
    Info.plist                       CFBundleName/CFBundleExecutable "Kira Studio",
                                     CFBundleIdentifier com.kirathecat.kira-studio
    Resources/
      icons.icns                     fallback icon; Assets.car (compiled from appicon.icon on macos-26) is the primary (§6)
    MacOS/
      Kira Studio                    the compiled Go binary — the literal filename, space included,
                                     must equal CFBundleExecutable or the bundle neither launches
                                     nor codesigns
```

There is no `runtime/` subtree any more (P58f M10) — the compiled Go binary is the whole app.
`create:app:bundle` used to assert `runtime/{node,engine}` existed and copy that tree in before
signing; both the guard and the copy step are gone, since there is nothing left to vendor.

**Dev loop:** `bun run dev:studio` (`cd apps/kira-studio && wails3 task dev`) launches a real native window
with hot reload — the Wails task's own dev-mode config drives the frontend build with a blocking
`common:build:frontend` for the embedded bundle, then runs `common:dev:frontend` (the Vite dev
server) in the background for HMR; `predev:studio` runs `bun run setup` first. `wails3 task darwin:run`
builds and runs a `Kira Studio.dev.app` from `build/darwin/Info.dev.plist` without packaging.

## 2. Config summary

**`apps/kira-studio/build/config.yml`** — the source Wails generates build assets from:

- `info.productName: "Kira Studio"`, `info.productIdentifier: "com.kirathecat.kira-studio"` (P57 D11:
  the Wails build is the only build now, so P51/P52's deliberately-distinct `…-shell` identifier is
  gone), `info.companyName`, and `info.version` — **the app's version, and the only place one is
  defined** (see "Where the version comes from" below).
- `dev_mode` drives `wails3 dev`: watched extensions, ignore lists, and the three dev commands.
- Editing `info` requires `wails3 task common:update:build-assets` to regenerate the assets, which
  overwrites hand edits under `build/` — so `Info.plist` below is regenerated, not hand-maintained.

**`apps/kira-studio/build/darwin/Info.plist`** — `CFBundleName` and `CFBundleExecutable` are both `Kira Studio`,
`CFBundleIdentifier` is `com.kirathecat.kira-studio`, `CFBundleVersion`/`CFBundleShortVersionString`
`0.0.0` (the scaffolded value; the *bundle's* copy is stamped at packaging time, below),
`CFBundleIconFile` `icons`, `CFBundleIconName` `appicon`, `LSMinimumSystemVersion` `26.0.0`. That floor is hand-narrowed
from Wails' template default of `12.0.0`, and matches the build's deployment target and the README's macOS 26+ product claim. The template hardcodes `12.0.0`, so a
`wails3 task common:update:build-assets` run reverts both plists — re-narrow them after regenerating.
Kira Space's plists also carry a generated document-type block (P332); rerun
`go run ./apps/kira-space/tools/gendoctypes` after the regeneration.
The minimum macOS is 26.0 on purpose: macOS 14 and 15 are dropped (`LSMinimumSystemVersion` in all four plists,
`install.sh` preflight, README and site copy; S14 guards the plists). The SDK is whatever Xcode the building
machine has, the VS Code/Electron model: newest SDK, AppKit styles by the linked SDK (larger macOS 26 traffic
lights), the runtime OS decides availability. `build:native` sets `-mmacosx-version-min` and
`MACOSX_DEPLOYMENT_TARGET` to `MACOS_MIN`, resolved from `xcrun --show-sdk-version`: 26.0 with an SDK 26+, else
14.0. The fallback keeps a runner with an older SDK building. CI and release run on `macos-26` (SDK 26+), so
CI and release binaries link SDK 26 with minos 26.0, matching dev. `verify-packaging.sh` A6 fails if a bundle
links SDK 26+ without minos 26.0, and only prints a warning note while the SDK is below 26.
The floor is 14 rather than 13 because Go 1.27 builds its own objects for macOS 13 minimum: a lower
`-mmacosx-version-min` only earns a linker warning that the object file was built for a newer macOS
than is being linked.

### Where the version comes from

`build/config.yml`'s `info.version` is the single source, and everything else is stamped from it at
build time:

| Consumer | How it gets there |
|---|---|
| The Go binary | `apps/kira-studio/Taskfile.yml`'s `APP_VERSION` reads `config.yml`; `BUILD_FLAGS` links it in with `-ldflags "-X …/internal/buildinfo.Version=<version>"`, in both the dev and production branches |
| The About dialog | `main.go`'s `application.Options.Description`. The macOS About item is the `application.About` role (`internal/shell/menutemplate.go`), which Wails renders with its own dialog — name, description, icon, and **no version field of its own** — so the version rides in the description |
| `AppService.Info()`'s `appVersion` | reads the same `buildinfo.Version`. Nothing in the renderer calls it today; it is API surface, not a display path |
| `Contents/Info.plist` (`CFBundleShortVersionString`, `CFBundleVersion`) — what Finder's Get Info and the version column read | `create:app:bundle` runs PlistBuddy over the bundle's *copy* after `cp`. The checked-in plist keeps its scaffolded `0.0.0`: it is a generated file, and a hand-edit there would not survive `common:update:build-assets` anyway |

An unlinked build — `go run`, `go test`, `go build ./...` by hand — reports `internal/buildinfo`'s own
literal, `0.0.0-dev`, which is how you can tell one from a real build. `verify-packaging.sh`'s **A5**
asserts the packaged bundle's `CFBundleShortVersionString` equals `config.yml`'s `info.version`, so a
bundle assembled around a stale plist fails the check rather than shipping quietly — and the
PlistBuddy step itself no longer skips silently off macOS: its guard is `-x /usr/libexec/PlistBuddy`,
and the `else` branch hard-fails `create:app:bundle` with an explicit "broken macOS environment"
error rather than continuing past it.

The root `package.json`'s `version` is **not** it — that is npm metadata for a private, unpublished
workspace root, and nothing in the app or the build reads it.

**`apps/kira-studio/Taskfile.yml` + `apps/kira-studio/build/darwin/Taskfile.yml`** — the task graph `bun run package:studio` drives:

| Task | What it does |
|---|---|
| `darwin:package:dmg` | what `bun run package:studio` invokes: `deps: package`, then `create:dmg` |
| `create:dmg` | `wails3 tool package --format dmg` over the built `.app` — background `darwin/dmg-background.png`, volume icon *and* file icon `darwin/icons.icns`, window 540×380. The tool adds the `/Applications` symlink and places the two icons itself. macOS only (`platforms: [darwin]`) |
| `darwin:package` | `deps: build`, then `create:app:bundle`. Still callable on its own when all you want is the bundle |
| `darwin:build` → `build:native` | preconditioned on `uname -s = Darwin` — there is no off-macOS branch any more (§5); `deps` = `common:go:mod:tidy`, `common:build:frontend`, `common:generate:icons`; then `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" -o bin/Kira Studio` with `GOOS=darwin CGO_ENABLED=1 GOARCH=$ARCH` (host arch unless overridden) and `MACOSX_DEPLOYMENT_TARGET=14.0` (`MACOS_MIN`: 26.0 with an SDK 26+, else 14.0, P346) |
| `create:app:bundle` | `rm -rf`s any previous bundle, then makes `Contents/{MacOS,Resources}` and copies `icons.icns`, `Assets.car` (when actool 26 produced one), the binary and `Info.plist`, then ends unconditionally with `codesign:adhoc` — only reachable behind `build`'s own Darwin precondition, so there is no off-macOS fallback to speak of. The `rm -rf` matters: every other step only copies *into* the bundle, so without it a resource an earlier build produced outlives the build that stopped producing it |

`common:build:frontend` runs the same `bun run build:studio` the checklist above already ran; Task's
`sources`/`generates` up-to-date checking makes the second invocation a no-op against fresh output, so
building the renderer first is cheap insurance, not duplicated work.

**`scripts/sign-bundle.sh`** — ad-hoc signing, run by `bun run package:studio` after the Task pipeline. macOS
only; exits 1 elsewhere.

```sh
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"
```

One deep sign over the whole bundle is all that is left (P58f M10) — there is no nested vendored Node
binary or Kafka native module to sign individually any more, so the two `codesign --force --sign -`
calls that used to precede the deep sign are gone with them. `create:app:bundle` already ran its own
`codesign --force --deep --sign -`; `sign-bundle.sh` re-signs and then *verifies* — redundant but
harmless, and it is the script a human or CI actually invokes.

The `.dmg` is signed there too, and the order is what makes that work: `create:app:bundle` signs the
bundle *before* `create:dmg` copies it into the image, so the copy inside is already signed and
`sign-bundle.sh`'s later re-sign of the outer `.app` — which cannot reach inside a built image — does
not need to. The image itself gets a plain `codesign --force --sign -`; `--deep` is deliberately
absent, since a disk image is a flat file with no bundle tree to recurse into.

**Deliberately present in the Taskfile but not wired into any `package.json` script:** `darwin:sign`
and `darwin:sign:notarize` (`wails3 tool sign [--notarize]`, Developer ID) and
`darwin:build:universal`/`package:universal`. Signing/notarization are deferred past v1 by
SPEC.md §3; the app is arm64-only, so a universal binary has nothing to be universal across.

**Keychain consequence of ad-hoc signing.** Credentials are encrypted under an AES-256-GCM key held in
a macOS Keychain generic-password item (`internal/secrets/keyring_darwin.go`, service
"Kira Studio Secrets", shared by Studio and Space). Keychain ACLs are keyed to a stable code-signing identity, which is
exactly what ad-hoc signing defers, so the first launch after installing a new build may show one
"Kira Studio wants to use your confidential information stored in…" prompt; **Always Allow** answers it
permanently for that build. Expected, and the honest cost of deferring signing — not worth working
around with a bundled private key.

**Whether ad-hoc signing affects `LAContext.evaluatePolicy` too is an open question (P14 F7 item
1), not yet answered.** The credential-reveal confirmation gate (`internal/localauth`) calls
`LAContext.evaluatePolicy(.deviceOwnerAuthentication)` to confirm the device owner before
decrypting a saved password for display; whether an ad-hoc-signed bundle is allowed to present that
prompt at all has not been confirmed on real hardware. If it turns out not to be, the app is still
correct — it falls back to the existing in-app confirmation dialog (`confirmDialog()`) exactly as
it does on Linux — but "macOS Touch ID confirmation" would then be a Developer-ID-signing-only
capability until this app is properly signed. §6.3 of
`docs/HISTORY.md (v1.1 P14)` is where the answer, once observed on a
real Mac, gets recorded.

## 3. Verification checklist — results from this environment

This environment is **Linux, with no macOS, no `codesign`, and no `/usr/libexec/PlistBuddy`**. Nothing
below is a claim about a real packaged bundle; the layout in §1 is read from `create:app:bundle`, not
observed in one. It is kept as the record of *that* environment: P10 later ran the packaging on real
macOS arm64 hardware, and what that run observed is recorded in §4's own items, not by rewriting the
table below.

| Check | Result |
|---|---|
| `bun run build:studio` (vite → `apps/kira-studio/frontend/dist`) | **pass** — run here this session |
| `bun run typecheck`, `bun run lint` | **pass** |
| `sh scripts/verify-packaging.sh` | **pass** — static checks S1/S2/S5 ran; `verify-packaging: note — skipped A1/A3/A5/N2 — "apps/kira-studio/bin/Kira Studio.app" not present` and `verify-packaging: note — skipped A4/N3 — "apps/kira-studio/bin/Kira Studio.dmg" not present`, then `verify-packaging: all checks passed` |
| `wails3 task darwin:package` | **not run** — needs macOS (§5) |
| `scripts/sign-bundle.sh` | **not run** — `codesign` is macOS-only and the script refuses to run off Darwin |
| Real bundle contents, `Info.plist` values, ad-hoc signatures | **not verified** — no bundle was produced here |
| `.app` on-disk size | **not measured** — expect substantially less than the pre-P58f figure now that there is no ~126 MB vendored `runtime/` tree to carry, likely close to the compiled Go binary's own size alone; not a recorded number |

Not verifiable off macOS (needs a human on real hardware — see §4): code signature checks, Gatekeeper
quarantine behavior, launching the app at all, `~/.kira-studio/` creation, cold-start timing, packaged
RSS, and the `.app` size.

## 4. Human checklist (run on macOS 26+ arm64)

Every item is a pass/fail a human should record here after running it for real. Since P295 these run after the release is published (§7), against its published assets, not before it. Items 1–3, 10 and 11
are also checked automatically on macOS by `release.yml`'s own "Assert packaged bundle and disk
image" step (§7) — but this list is
only updated from an *observed* run, never from expectation. The items marked **pass (P10)** were
observed on macOS 26.5.2 arm64 while wiring up the DMG; everything still marked *not yet run* needs a
human to launch the packaged app and use it.

1. `bun run package:studio` completes and `sign-bundle.sh` prints `signed and verified`. — **pass**
   (P10, macOS 26.5.2 arm64): printed twice, once for the `.app` and once for the `.dmg`.
2. `codesign -dv --verbose=2 "apps/kira-studio/bin/Kira Studio.app"` reports `Signature=adhoc`. There is no
   nested vendored Node binary to check separately any more (P58f M10). — **pass** (P10):
   `Signature=adhoc`, `Identifier=com.kirathecat.kira-studio`, `TeamIdentifier=not set`.
3. `bun run verify:packaging` passes against the real bundle — i.e. A1/A3/N2 actually execute
   instead of reporting "skipped". — **pass** (P10): all checks passed with both artifacts present,
   so A1/A3/N2 and P10's own A4/N3 all executed.
4. Launching the app shows the workbench with the engine status dot green — "the engine" is this
   process itself now (P58f D11), so there is no vendored runtime for a missing-file check to name;
   a launch failure here is an ordinary Go panic/crash, not a missing-sidecar error. — *not yet run*
5. Gatekeeper: an unsigned, unnotarized build needs right-click → Open, or
   `xattr -dr com.apple.quarantine "/Applications/Kira Studio.app"`. Expected, not a defect. —
   *not yet run*
6. `~/.kira-studio/` is created with `kira.db` and `logs/` on first launch — the real home, since
   `KIRA_HOME` is unset in a packaged run. — *not yet run*
7. Create a connection, expand the tree, open a data tab, scroll, open the cell editor, quit cleanly.
   The View menu has no Reload / Toggle DevTools in a `-tags production` build. — *not yet run*
8. Kafka: connecting is **expected to succeed** in a packaged build — Kafka has been served
   in-process by the native Go adapter since P58e M9, and as of P58f M10 there is no Node engine
   left for a native-module `require()` to fail against even in principle. — *not yet run*
9. Cold start: launch 3 times, discard the first, take the median from the startup log line in
   `~/.kira-studio/logs/`; record against the ≤ 1500 ms target. RSS cross-check: rebuild the
   5-connection/10-tab scenario by hand and sum the app's processes via `ps -o rss=`, against 350 MB —
   expect a materially better number than any pre-P58f measurement, since there is no second
   process's baseline RSS to add on top of the webview and shell any more. —
   *not yet run, see `docs/PERF.md` §3, whose recorded numbers still predate this bundle*
10. Record the `.app` on-disk size (`du -sh`, lever L-D, budget ≤ 300 MB) — expect a large drop from
    any pre-P58f measurement, now that the ~126 MB vendored `runtime/` tree is gone. — **pass**
    (P10): **42 MB** for the `.app`, **16 MB** for the compressed `.dmg` around it, against a
    300 MB budget.
11. The disk image installs the way it looks like it should (P10): it mounts, holds `Kira Studio.app`
    beside an `/Applications` symlink, and carries the app's own icon as both its Finder icon and its
    volume icon. — **partial**: mount, contents, both icons and `Signature=adhoc` on the image were
    verified from the shell; the drag-onto-Applications gesture and how the window *looks* when
    Finder opens it — background, icon placement — still want a human's eyes.
12. **Removed in P243 Part 2** — the extension install button and `.vsix` bundling no longer exist.

**P119's curl installer and in-app update, items 13-20 — none of these can run in this Linux
sandbox.** They need a published release with both DMGs, which only the first real run of the
patched `release.yml` produces; §11.3 in `docs/HISTORY.md (v1.9 P119)` has the
full step-by-step. All *not yet run*.

13. Fresh machine, no app installed: the Studio curl command installs and opens the app with **no**
    Gatekeeper prompt (`xattr -p com.apple.quarantine` on the installed `.app` reports no such
    attribute). Repeat for Space.
14. Running the same command again while the app is open fails with "Quit Kira Studio first" —
    nothing changed, no disk image left mounted, no stale `/Applications/.studio-install.*`.
15. An older stamped build, launched, auto-opens the update dialog naming both versions; **Later**
    closes it without re-opening on the next launch for that same version; clicking the status-bar
    item reopens it on demand.
16. **Update** shows an installing state, quits the app once staged, reopens the new version within
    about a minute, and the About dialog and `install.log` both show the new version. Repeat for
    Space.
17. **Cancel** during install returns to idle with no leftover `kira-install` process, mount, or
    staging directory.
18. A failure after hand-off (bundle made immutable before the swap) relaunches the old version and
    shows an alert naming the install log.
19. Whether any macOS App Management privacy prompt appeared during the self-replace, in which
    mode, and what the user had to grant (§2.6's own open risk).
20. A dev build makes no request to `api.github.com` at all (no update item, no dialog) — checked
    with Little Snitch or `nettop` during a `bun run dev:studio` session.

## 5. Off-macOS: what this environment could actually check

Unlike the electron-builder pipeline this replaced — which assembled a complete, inspectable `.app` on
Linux by downloading prebuilt Electron binaries, and only failed at the macOS-only `sips` step for
`dmg` — the Wails pipeline compiles a real Go binary with `CGO_ENABLED=1` against macOS system
frameworks. On Linux that leaves two doors, and neither was opened here:

- **`build:native`** requires macOS — `darwin:build` preconditions on `uname -s = Darwin` and has no
  off-macOS branch at all any more (§2). Not attempted.
- **`codesign`** does not exist off Darwin at all, and there is no fallback path to reach: every step
  that would call it is behind the same Darwin precondition. `sign-bundle.sh` exits 1 on purpose off
  macOS.

Consequently `scripts/verify-packaging.sh` degrades honestly off macOS: with no bundle it prints one
"skipped A1/A3/A5/N2" note and one "skipped A4/N3" note and passes on the static checks alone,
and even with a bundle it would skip A1/A3/A5/N2 for want of `codesign`/`PlistBuddy`. **A green
`verify:packaging` on Linux proves only the static checks (S1/S2/S5/S6/S7/S8/S10/S11/S12/S13), not that
any bundle is correct.** The static checks run over committed files, so they mean something even without a bundle. A1/A3/A5/N2
need the real `.app`.

What *is* fully verifiable off macOS: everything that feeds the bundle rather than being the bundle —
the renderer build, typecheck, lint, the Go unit tests, and the static half of `verify:packaging`.

## 6. Known gaps

- **Closed (P57 M7, then P58e M9, then P58f M10, in that order).** The packaging gap started as "Kafka's
  native module was never vendored into the packaged bundle" (P57 M7): `build:engine` marked
  `@confluentinc/kafka-javascript` `--external` and no build step vendored it, so a packaged build's
  Kafka connections would fail at `require()` time. P58e M9 made the gap moot without closing it:
  Kafka went native in Go (`docs/HISTORY.md (v1 P58e)`) and stopped reaching the Node engine child
  at all, so the dead `require()` path was never exercised — `sign-bundle.sh` and
  `verify-packaging.sh` kept probing for the native module and printing a harmless "not present" note.
  P58f M10 closed it for real: the Node engine, `build:engine`, and every check that probed for that
  module are deleted outright, not merely dead-code-kept. There is no native-module packaging
  question left to have a gap in.
- **Ad-hoc signature only** (identity `-`). The build is not distributable outside the machine that
  built it, and SPEC.md §3 defers signing/notarization past v1. `wails3 tool sign [--notarize]` is
  available via `darwin:sign`/`darwin:sign:notarize` but is wired into nothing.
- **The DMG is what ships (P10).** `bun run package:studio` runs `darwin:package:dmg`, so a build produces
  the `.app` *and* the styled image around it, and `scripts/sign-bundle.sh` ad-hoc signs both. The
  window is 540×380 with the app at 28% and the `/Applications` shortcut at 72% of its width
  (positions come from `wails3 tool package` itself, not from anything in this repo); the volume
  icon and the image's own Finder icon are both `darwin/icons.icns`, and `darwin/dmg-background.png`
  is a flat gradient in the app icon's own palette with an arrow between the two icon positions.
  The scaffolded `dmg-file-icon.icns`/`.png` were Wails-branded drive artwork and were deleted with
  the same reasoning as `Assets.car`. What is still deferred is Developer ID signing and
  notarization, which SPEC.md §3 puts past v1 — an ad-hoc-signed image is still Gatekeeper-blocked
  on first launch (§4's checklist covers the workaround).
- **§4 items 1, 2, 3 and 10 passed on real macOS 26.5.2 arm64 hardware (P10); item 11 is partial;
  items 4-9 are still unrun.** Whoever next has macOS hardware should run items 4-9 and finish
  item 11's outstanding half — the drag-onto-Applications gesture and how the Finder window looks.
- **`Assets.car` ships, compiled from a full-bleed `appicon.icon` (P349).** On a macOS 26 host with
  Xcode 26 (`macos-26` CI runner), `wails3 generate icons` runs `actool` over `appicon.icon`, writes
  `darwin/Assets.car`, and overwrites `darwin/icons.icns` with actool's own render. Both are generated
  and gitignored (`Assets.car`, `temp.plist`); `create:app:bundle` copies `Assets.car` in. The plists
  carry `CFBundleIconName` `appicon` beside `CFBundleIconFile` `icons`; the two belong together, as
  `CFBundleIconName` resolves against `Assets.car` first. The SVG layer is full-bleed (artwork scaled
  to the whole 1024 canvas, no rounded clip) because the system applies the squircle mask; the old
  pre-masked layer left a transparent margin that showed the default light fill as a white ring.
  `icon.json` sets a document-level solid `fill` matching each background's top stop. The tracked
  `icons.icns` and `appicon.png` are the fallback for hosts without actool 26 (never a shipped macOS
  build). `verify-packaging.sh` S15 checks the sources; A7 checks `Assets.car` and `CFBundleIconName`
  in the built bundles. Unverified off-macOS: the `fill` key form under actool; if actool rejects it,
  take the form Icon Composer 26 writes.

## 7. CI, releases, install and update

**Status:** applied. `.github/workflows/` holds `pr.yml`, `pr-auto-merge.yml`, `pr-rerun-flaky.yml`,
`release-on-merge.yml`, `release.yml`, `cache-warm.yml` and `pages.yml`
(`db-compat.yml` and `test-matrix.yml` existed at one point but were folded into `release.yml`'s own
jobs, see `docs/ARCHITECTURE.md`).

**`pr.yml`** (trigger: `pull_request` to `main` plus `workflow_dispatch`; no push, so `main` is reached
only through a PR). No job `needs` another; every job starts at once:

| Job | Runner | What it does |
|---|---|---|
| `checks (macOS)` | `macos-26` (pinned, not `macos-latest`) | cgo-aware `go vet`/build (catches darwin build-tag breaks) and the darwin&&cgo-only test packages |
| `checks (Linux)` | `ubuntu-latest` | lint, typecheck, frontend builds, `lint:dead`, `verify:packaging` and the bun unit tests |
| `ui (Linux, shard N/4)` | `ubuntu-latest` | Playwright `ui` project in 4 shards, fail-fast off |
| `ui-extra (Linux)` | `ubuntu-latest` | ipc-frontend (chromium), visual (webkit), then ui-timing alone on an idle machine |
| `go (Linux)` | `ubuntu-latest` | Go packages that start no container, plus every package the container split does not name |
| `go-containers (Linux)` | `ubuntu-latest` | Go packages that start Testcontainers; no bun, bindings or real frontend build |

The bundle assertion lives only in `release.yml`'s own "Assert packaged bundle and disk image" step.

**Chapter PR to release (P330).**

1. The close phase pushes `chapter/vX.Y.Z` and opens the PR to `main`.
2. `pr-auto-merge.yml` refuses a malformed head or an existing tag, then enables squash auto-merge.
3. `pr.yml` runs. `pr-rerun-flaky.yml` reruns failed jobs while `run_attempt < 3`, only for the
   PR's current head and newest run. A cancelled run reruns only when a job hit its timeout, never
   when superseded or cancelled by hand (P365).
4. Once the ruleset's required checks pass, GitHub merges.
5. `release-on-merge.yml` tags the squash commit (`merge_commit_sha`) as `vX.Y.Z`, dispatches
   `pages.yml` and `cache-warm.yml` (once per merge, deduplicated by the warm run's title), and
   dispatches `release.yml -f tag=vX.Y.Z`. A merge made with `GITHUB_TOKEN` starts no workflow
   except `workflow_dispatch` and `repository_dispatch`. So it has four entry points into one
   idempotent script: `workflow_run` of `PR` (waits for the merge), `pull_request: closed` (a human
   merge), an hourly sweep (whatever both missed), and a manual dispatch.

**Main cache warming (P330).** `pr.yml` has no push trigger, and PR runs save caches into their own
`refs/pull/N/merge` scope, which no other PR or release restores. `cache-warm.yml` runs on `main`
after each merge and saves what `pr.yml` restores: Go modules, the four Go build kinds
(`macos`, `checks`, `go-rest`, `go-ct`), bun, the `wails3` CLI, bindings, tsbuildinfo, the
golangci-lint binary, both Playwright browser sets and the Testcontainers images (on a miss only).
It installs and compiles, no tests: `go test -run '^$' -exec true` builds test binaries and runs
nothing. Cost: about 30-55 runner-minutes in parallel per merge, free on a public repo. On a private
repo the macOS job bills at 10x, about 190 billed minutes in total.

**Cutting a release** (`release.yml`, on tags matching `v*.*.*` or dispatched with a `tag` input, P119 one tag for both apps, P295 direct publish):

1. `release-on-merge.yml` creates the tag. A manual `git tag vX.Y.Z <sha> && git push origin vX.Y.Z`
   still works and starts `release.yml` through its tag trigger. The `build` job writes the tag (minus its `v`) into both apps' `build/config.yml`
   `info.version` in its own checkout — no pre-tag version-bump commit is needed — and from there
   each reaches its own binary, About dialog and bundle `Info.plist` (see "Where the version comes
   from"); it asserts both writes landed before building.
2. `tag` resolves the tag to a commit and refuses a tag that already has a published release, and
   a non-prerelease tag whose commit is not on `main`; then `build (macOS)`, `test-matrix (Linux)` and
   `db-compat (Linux, min|max)` start together at that commit. `build` restores the setup action's
   caches from `main` and saves none; `cache-warm.yml` is what fills `main`'s caches. It runs
   `bun run package:studio` and `bun run package:space`, names each `.dmg`
   (`kira-studio-macos-arm64.dmg`, `kira-space-macos-arm64.dmg`), runs `verify:packaging` and the
   bundle and disk image assert, hashes both images and uploads them as artifact `release-dmgs`.
   `lint` and `typecheck` do not rerun here; `main` takes only PR merges, which the ruleset gates on
   PR CI, and `tag` refuses any other commit.
3. `publish` (ubuntu) runs only when every job above passed. It first refuses a tag that moved since
   `tag` resolved it, re-checks the artifact hashes, refuses
   a tag that already has a published release, creates a draft with both images, verifies each
   asset (`state`, `size`, `sha256` digest), then publishes with one API call. A failed job leaves
   no release and no draft; a failure after the draft exists deletes it.
4. The release is public at once and `/releases/latest` serves it. The §4 real-hardware checks run
   after publish against the published assets. A failure found there means a fixed next patch tag
   (the updater offers only strictly newer versions); optionally delete the bad release so
   `/releases/latest` falls back to the previous one. Installed copies are not downgraded.

Recovery:

- Flaky job: "Re-run failed jobs".
- Tag moved after a failed run: start a new run (`gh workflow run release.yml -f tag=vX.Y.Z`).
  Rerunning the old run's failed jobs is refused.
- No release started: `gh workflow run release-on-merge.yml -f pr=<n>`, or
  `gh workflow run release.yml -f tag=vX.Y.Z` when the tag exists.
- Tag creation refused: run the commands the job log prints.
- Real failure: delete the tag (`git push --delete origin vX.Y.Z`); the fix ships as the next
  chapter (patch plus 1).

**No *silent* auto-update — P119 adds a real one, gated on an explicit click.** SPEC.md defers
*silent* auto-update past v1 (§1's deferred list, §3's app-identity line); a user-initiated
install/update, run through `scripts/install.sh`, is not that. macOS's own silent auto-update
also requires a signed and notarized app, which §6 still defers. `verify-packaging.sh` keeps
re-asserting the absence of the silent kind:

- **S1** — no `electron-updater`/`update-electron-app` dependency in `package.json`.
- **S2** — no updater code in `apps/` or `packages/` (there has been no `src/` since P3's
  restructure and P58f's `src/engine/` deletion).
- **S5** — `package.json`'s `package:studio`/`package:space` scripts still run
  `wails3 task darwin:package:dmg` (P10 moved this from the plain `.app`-only task, so the shipped
  `.dmg` always has something to upload); this check fails loudly if either packaging entry point
  is swapped for something that could publish.

P29 added three more static checks, guarding §2.1/§2.2's own findings against regressing:

- **S6** — `frontend/dist/assets/*.js` (when it exists) does not carry the Playwright debug-hook
  identifiers (`__kiraCount`, `__kiraCacheStats`, `__kiraRetention`, `__kiraRetainedBytes`,
  `__kiraTreeConnectionIds`) — the guard against `build:frontend`'s Task fingerprint trap: it keys
  off `frontend`'s own `sources` (excluding `dist`), so `bun run test:ui:studio` (which writes the
  hooks-enabled `build:test:studio` bundle to `dist`) followed by `bun run package:studio` with no intervening
  source edit would otherwise let Task's up-to-date check skip the rebuild and embed the wrong
  bundle. Checked against `frontend/dist` rather than the `.app` bundle so it also runs on Linux
  and before packaging.
- **S7** — `frontend/src/main.ts` assigns no `window.__kira*` outside its
  `if (__KIRA_DEBUG_HOOKS__)` block — the check that would have caught
  `window.__kiraGridEngine` shipping unconditionally, and catches the next hook added the same way.
- **S8** — `apps/kira-studio/main.go` calls no `os.Getenv` at all — true since `KIRA_G1_BLANK` was
  deleted. A precise, low-false-positive invariant for the app's own entry point.
- **S10** — no reference under `apps/`, `packages/` or `internal/` to `browser_download_url` or
  `releases/download` — rescoped by P119 to include `internal/` (`appupdate` now lives there).
  `scripts/` is deliberately excluded: `scripts/install.sh` is the one sanctioned downloader, see
  below. P210's embedding-model download is a user-clicked app-code download from a Hugging Face
  `resolve/` URL with a pinned SHA-256, not a release asset, so S10 stays green. Native code is never
  downloaded at runtime: the ONNX Runtime dylib is fetched at build time by `scripts/fetch-onnxruntime.sh`.
- **S11** (P119) — the curl installer and its Go counterpart agree on their one shared contract:
  `scripts/install.sh` exists and passes `sh -n`; its line-2 contract marker
  (`# kira-install-contract: 1`) matches `internal/appupdate/install.go`'s own constant;
  `raw.githubusercontent.com` appears exactly once across `apps/`, `packages/`, `internal/`
  (`InstallScriptURL`); `shellcheck` runs against the script when it's on `PATH`.
- **S12** (P210) — the bundled ONNX Runtime version agrees: `internal/memory/embed/paths.go`'s
  `ORTVersion` equals `scripts/fetch-onnxruntime.sh`'s `ORT_VERSION`, and `create:app:bundle` in
  `apps/kira-space/build/darwin/Taskfile.yml` references `libonnxruntime.<version>.dylib`.
- **S13** (P332) — Kira Space's two Info plists match `go run ./apps/kira-space/tools/gendoctypes
  --check`: the generated document-type and imported-UTI block equals `internal/filetypes`.

There is no publish provider, no update feed, no `latest-mac.yml`, and no `.blockmap` — the last of
those was an electron-builder differential-update artifact that has no equivalent here, so it is absent
by construction rather than deleted per build.

**P66 (v1.6) added an update-*availability* check; P119 (v1.9) added the install itself, still only
on an explicit click.** A tagged release build polls `GET https://api.github.com/repos/
vlad-cirstean/kira-studio/releases/latest` (repo-root `internal/appupdate`, shared by both apps, no
`gh` CLI dependency — public metadata needs no per-user credential) and, when the tag is newer than
the running build, shows a status-bar item (`internal/bridge/update.go`,
`packages/workbench/src/components/{UpdateAvailableItem,UpdateDialog}.vue`) that opens a modal
dialog with an **Update** button. Clicking it re-checks availability, then `internal/
appupdate.Installer` fetches `scripts/install.sh` fresh from `main` (never bundled), validates its
contract marker, and spawns it detached with `--app=studio|space`; the script and the app hand off
over an fd once the disk image is staged, the app quits, and the script swaps the bundle into
`/Applications` and reopens it. **Nothing downloads or installs without that click** — S10/S11
above are what keep the app-code half of that true; `scripts/install.sh` itself is the one
sanctioned exception, run by the user's own action. A dev/test build (`buildinfo.Version` one of
`0.0.0`/`0.0.0-dev`/`0.0.0-unknown`) makes no request and refuses `InstallUpdate` outright. And
because `release.yml` publishes the release only after every gate and asset check passed (step 3
above), a release is visible to this check from the moment it is published. It goes through a draft
first, and `/releases/latest` excludes drafts and prereleases by construction, so a failed run
never shows up.

**What a future *silent* auto-update (no click at all) would require, in order:** code signing and
notarization (SPEC.md §1/§3), then a SPEC.md scope change reversing "no silent auto-update", then an
update feed and background-updater wiring — P119's click-triggered install is a deliberately
different, smaller thing than this.

**Whether the release workflow has actually run:** yes, by tag push since v0.2.x. The dispatch path
first runs at the v0.2.12 merge.

**The on-demand DB compatibility suite is outside `pr.yml`, but gates `release.yml`.**
`scripts/db-compat.sh` (`bun run test:compat`, P16) runs the same per-engine conformance packages
against each kind's oldest and newest supported server image, sixteen (kind, min|max) pairs —
deliberately outside `bun run test:go` and outside `pr.yml`, since it's too expensive to run on
every push. It used to also have its own standalone `workflow_dispatch`-only workflow,
`.github/workflows/db-compat.yml`; that file is gone — `release.yml`'s own `db-compat` job now runs
it directly, as a job (min and max in parallel) every release tag must pass before `publish` creates
the release, and a one-off single-kind/extreme run during adapter work uses `bun run
test:compat -- --only <kind> --min|--max` locally instead. `scripts/test-matrix.sh` (P25's complete
auth/config tier) is the same shape and made the same move, for the same reason — see
`docs/ARCHITECTURE.md`'s own note on it.

**P19's `actions/{checkout,setup-go,upload-artifact}` bumps to `@v7` across `pr.yml` and
`release.yml`, together with P20's rerouting of all three inline binding-generation blocks through
`sh scripts/setup.sh`, spent several phases staged under
`p1{6,9}-pending-ci-workflow/`** plan folders (v1.1, `docs/HISTORY.md`) because the sessions that wrote them had GitHub
push access without the `workflow` OAuth scope, which GitHub requires for any commit touching
`.github/workflows/*.yml`. A session with that scope applied them; both staging directories are
gone.

### Repository settings (user-applied)

1. Ruleset "Protected branches" (`refs/heads/main`): add **Require status checks to pass**, source
   GitHub Actions. Add these checks: `checks (macOS)`, `checks (Linux)`, `ui (Linux, shard 1/4)`,
   `ui (Linux, shard 2/4)`, `ui (Linux, shard 3/4)`, `ui (Linux, shard 4/4)`, `ui-extra (Linux)`,
   `go (Linux)`, `go-containers (Linux)`. Leave "Require branches to be up to date" off: one chapter PR at a time, a stale base is rare, and a forced rebase would rerun full CI. Keep the
   squash-only and PR rules. **Do this before step 2.** With no required checks, auto-merge merges
   the moment it is enabled.
2. Settings, General, Pull Requests: enable **Allow auto-merge**. Squash stays allowed.
3. New branch ruleset "Branch names", enforcement Active. Target: include all branches (`~ALL`),
   exclude `refs/heads/main` and `refs/heads/chapter/v*`. Rule: **Restrict creations**. The
   "Restrict branch names" metadata rule (regex) needs GitHub Enterprise. Use it if the UI offers it:
   `^(main|chapter/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))$`. Otherwise
   fnmatch cannot express semver, and the hook is the exact check. Existing legacy branches stay
   untouched. Apply after the `chapter/v2.0`, `v2.6`, `v2.8` and `v0.2.10` renames.
4. Leave the "Protected tags" ruleset disabled. Its "Restrict creations" would block the workflow's
   tag. If it is ever enabled, it needs GitHub Actions as a bypass actor.
5. Settings, Actions, General, Workflow permissions: default read stays. Every job declares its own.
   "Allow GitHub Actions to create and approve pull requests" is not needed.

## 8. Kira Space packaging

Kira Space embeds two gitignored bundles: `frontend/dist` (desktop UI) and `frontend/dist-mobile` (phone app,
`//go:embed all:frontend/dist-mobile`; since P223 only `index.html`, `favicon.ico` and `assets/`: no
service worker, manifest or setup page). Both must exist before `go build`. The Taskfile `build:frontend`
task builds both (`bun run build` then `bun run build:mobile`), so packaged builds embed the real phone app.
Clean checkout or fresh worktree: `sh scripts/prepare-worktree.sh` builds both. CI: `bun run build:space` chains both.

**As of v1.9 P100, Kira Space is a separate app with its own bundle, own DMG, and its own copy of
the whole pipeline above** — `apps/kira-space/build/config.yml`, `apps/kira-space/Taskfile.yml` and
`apps/kira-space/build/darwin/Taskfile.yml`, generated and packaged exactly the way §1-§2 describe
for Kira Studio, `CFBundleIdentifier com.kirathecat.kira-space`. `bun run package:space` (`cd
apps/kira-space && wails3 task darwin:package:dmg`, then `sh scripts/sign-bundle.sh apps/kira-space
"Kira Space"`) is the entry point — `sign-bundle.sh` takes the app dir and app name as `$1`/`$2` as
of P100 Part 2, so this one script signs both apps' bundles rather than a near-identical copy per
app. Expected artifacts: `apps/kira-space/bin/Kira Space.dmg`/`.app`/`Kira Space` (the DMG, the
bundle, and the bare binary). **As of P119, `Kira Space.dmg` ships as `kira-space-macos-arm64.dmg` on the
same release as Kira Studio's own disk image** — one `release.yml` build job, one tag, one release
(§7) — rather than a separate release or a separate `release-space` job.

**ONNX Runtime (P210).** `darwin:package` and `darwin:package:universal` depend on `fetch:onnxruntime`
(`scripts/fetch-onnxruntime.sh osx-arm64`: MIT release tgz, SHA-256 pinned, extracted to the gitignored
`apps/kira-space/build/onnxruntime/osx-arm64/`). `create:app:bundle` copies `libonnxruntime.1.31.1.dylib`
into `Contents/Frameworks/` and its LICENSE and ThirdPartyNotices into `Contents/Resources/`, before
`codesign:adhoc`, and fails loudly if the dylib is missing. The dylib is arm64-only (ORT 1.31.1 has no
x86_64 macOS build); the universal bundle's x86_64 slice reports embeddings unavailable. Adds about
43 MB to the bundle. `memory-embed` is the same executable run as a subcommand, so no extra binary
needs signing. Mac verification is a human item (see `docs/ARCHITECTURE.md` Known open items):
`Contents/Frameworks/libonnxruntime.1.31.1.dylib` present, `codesign --verify --deep --strict` passes,
worker footprint in Activity Monitor at most 150 MB.

**Not yet run in this environment** for the same reason as Kira Studio's own §3/§4: this sandbox is
Linux, with no macOS, no `codesign`, and no `code` CLI to probe for. What §5's off-macOS limits say
about Kira Studio's own build applies identically here — `build:native` needs macOS, `codesign`
does not exist off Darwin — so nothing above has been observed against a real bundle, only read
from the Taskfiles that produce one.
