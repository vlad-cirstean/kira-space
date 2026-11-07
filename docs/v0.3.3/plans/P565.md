# P565 Repo rename: point URLs at kira-space (non-site)

Repo moved `vlad-cirstean/kira-studio` to `vlad-cirstean/kira-space` (`git remote -v` already shows
`kira-space`). Point every string that must reach the real repo at the new name, outside `site/` and
`docs/HISTORY.md`. Single implementer, no streams.

Discovery note: CodeGraph MCP failed to connect this session (`codegraph` not on `PATH`). Discovery
used `rg` over the whole tree instead.

## Why it matters now

- `scripts/install.sh:258` checks the asset URL prefix `https://github.com/vlad-cirstean/kira-studio/releases/download/$TAG/`.
  The API now returns `browser_download_url` under `kira-space`, so the check dies with "release
  asset URL is not the expected GitHub download URL". Install and in-app update stay broken until
  this lands on `main` (install.sh is fetched fresh from `main`).
- Old builds keep working through GitHub's rename redirect for `raw.githubusercontent.com` and
  `api.github.com` (Go's `http.Client` and `curl -L` follow it; checker sets no `CheckRedirect`).
  That redirect lasts only while nobody creates a new repo named `vlad-cirstean/kira-studio`.
- GitHub Pages does not redirect a renamed project site. The page moves to
  `https://vlad-cirstean.github.io/kira-space/`, so README screenshots and page links must move too.

## Scope

In: repo URLs (`github.com`, `raw.githubusercontent.com`, `api.github.com`), the Pages URL, the
clone directory name, the `verify-packaging.sh` pin, the release-notes template (via pending patch).

Out (user decision): Go module path `github.com/kirathecat/kira-studio`, identifiers
(`kira-studio-session` MCP server, `ci@kira-studio.test`, `~/.kira-studio` home), "Kira Studio"
prose, `site/` (P566), `docs/HISTORY.md`, `apps/kira-space/tests/claude/testdata/memory-eval/**`
(recorded eval data, not a repo pointer; P567 owns memory).

Checked, no change needed: release asset names (`install.sh:116` already `kira-space-macos-arm64.dmg`),
`apps/kira-space/Taskfile.yml` (module path only), `.githooks/` and `scripts/check-push-refs.sh` (no
repo name), `package.json` (no `repository` field), appupdate tests (no URL assertion), other
workflows (`pr.yml` only has the `ci@kira-studio.test` identifier).

## Edits

Replace `vlad-cirstean/kira-studio` with `vlad-cirstean/kira-space` and
`vlad-cirstean.github.io/kira-studio/` with `vlad-cirstean.github.io/kira-space/` at each line below.

| File | Lines | Change |
|---|---|---|
| `scripts/install.sh` | 28, 199, 258 | `RAW_BASE`, releases/latest API URL, asset URL prefix |
| `internal/appupdate/install.go` | 26 | `InstallScriptURL` |
| `internal/appupdate/checker.go` | 35 | `repoName = "kira-space"` |
| `scripts/verify-packaging.sh` | 145, 147 | S11 pin pattern and its fail message |
| `README.md` | 1, 5 (both URLs), 14, 18, 22, 26, 30, 34, 38, 56, 66, 67, 304 | badge, Pages links and shots, install curl, clone URL, `cd kira-space` |
| `apps/kira-space/README.md` | 3 (both URLs), 10, 151, 162 | badge, Pages link, install curl, `cd kira-space` |
| `docs/PACKAGING.md` | 490 | releases/latest URL |
| `docs/DEV_ENVIRONMENT.md` | 56, 65 | Pages URL, `gh api repos/vlad-cirstean/kira-space/pulls` |
| `docs/ARCHITECTURE.md` | 1654, 3867 | "against the kira-space GitHub releases/latest API"; Pages URL |
| `docs/ARCHITECTURE.md` | self-update block (near 1665) | add one bullet, below |
| `docs/pending-changes/.github__workflows__release.yml.patch` | new | release.yml:259 curl URL, see below |

Line numbers are from `e9a9d7e3`; P564 edits the same docs concurrently, so locate each by content,
not number.

New `docs/ARCHITECTURE.md` bullet, after the S10/S11 bullet:

```
- The repo was renamed from vlad-cirstean/kira-studio to vlad-cirstean/kira-space in v0.3.3. Builds up to v0.3.2 still fetch the old raw.githubusercontent.com and api.github.com URLs; GitHub's rename redirect serves them only while no new repo takes the old name, so never recreate vlad-cirstean/kira-studio.
```

### release.yml via pending patch

`.github/workflows/*` cannot be pushed from a Linux session (`docs/DEV_ENVIRONMENT.md`). Steps:

1. Confirm `docs/pending-changes/.github__workflows__release.yml.patch` does not exist yet.
2. Edit `.github/workflows/release.yml:259` locally to the `kira-space` raw URL.
3. Run `git diff -- .github/workflows/release.yml > docs/pending-changes/.github__workflows__release.yml.patch`.
4. Prepend one line: `# Repo renamed to kira-space (P565): release notes install URL.` (`git apply` ignores leading text before `diff --git`).
5. Run `git checkout -- .github/workflows/release.yml`. The real workflow file must stay unmodified.
6. Run `git apply --check docs/pending-changes/.github__workflows__release.yml.patch`. It must pass.

Report in the Result that the user must apply this patch from a Mac session and delete the patch
file in the same commit.

## Cross-stream ownership

Same checkout, concurrent agents. P564 deletes sentences in `README.md`, `apps/kira-space/README.md`,
`docs/DEV_ENVIRONMENT.md`, `docs/ARCHITECTURE.md`. P566 owns `site/`.

| File | Owner | P565 edit | Other stream's edit |
|---|---|---|---|
| `scripts/install.sh` | P565 | 3 URLs | none |
| `internal/appupdate/install.go`, `checker.go` | P565 | 1 const each | none |
| `scripts/verify-packaging.sh` | P565 | S11 pin | none |
| `docs/PACKAGING.md` | P565 | line 490 | none |
| `docs/pending-changes/…release.yml.patch` | P565 | new file | none |
| `README.md` | shared, P564 | URL-only line edits | P564 deletes migration sentences |
| `apps/kira-space/README.md` | shared, P564 | URL-only line edits | P564 deletes migration sentences (e.g. line 155) |
| `docs/DEV_ENVIRONMENT.md` | shared, P564 | 2 URL edits | P564 deletes migration text |
| `docs/ARCHITECTURE.md` | shared, P564 | 2 URL edits, 1 added bullet | P564 deletes one-time-move text |
| `site/**` | P566 | none | all site URLs, `base` |

Rules for shared files:

- Touch only the URL substring on a line. If P564 already deleted a line that held a URL, skip it.
- Never commit another stream's hunks. Before each commit, run `git diff -- <file>` and
  `git diff --cached --stat`. If the file holds only P565 hunks and the index is empty, use
  `git commit <paths>`. If it also holds foreign hunks, write a patch of only P565 hunks, run
  `git apply --cached <patch>`, confirm `git diff --cached` shows only P565 hunks, then `git commit`
  with no paths.
- Never `git add -A`, `git add .`, `git stash` or `git checkout` a shared file.

Dependency on P566: the new Pages links resolve only once P566 sets `site/astro.config.mjs`
`base: '/kira-space'` (and updates `site/tests/site.spec.ts:244`). Tell the orchestrator in the
Result so P566's plan covers `base`, not only `REPO_URL`. No ordering dependency: both land in
the same chapter PR, and Pages deploys after merge.

## Commits

1. `fix: point installer and update checker at kira-space repo`: `scripts/install.sh`,
   `internal/appupdate/install.go`, `internal/appupdate/checker.go`, `scripts/verify-packaging.sh`.
2. `docs: point repo and Pages URLs at kira-space`: `README.md`, `apps/kira-space/README.md`,
   `docs/PACKAGING.md`, `docs/DEV_ENVIRONMENT.md`, `docs/ARCHITECTURE.md`.
3. `chore: pending release.yml patch for kira-space install URL`: the pending patch.
4. `docs: mark P565 done`: `docs/v0.3.3/SPEC.md` row Done plus a short `### Result` under the P565 section.

## Gone checks

Run from repo root. Each must print nothing, except where noted.

```sh
rg -n --hidden -g '!.git' -g '!site/**' -g '!docs/HISTORY.md' -g '!docs/v0.3.3/**' \
  'vlad-cirstean/kira-studio|vlad-cirstean\.github\.io/kira-studio'
# expected: only .github/workflows/release.yml:259 (applied later from the pending patch)
rg -n '"kira-studio"' internal/appupdate
rg -n 'cd kira-studio' README.md apps/kira-space/README.md
git diff --exit-code -- .github/workflows/release.yml
```

Positive checks:

```sh
rg -c 'vlad-cirstean/kira-space' scripts/install.sh            # 3
rg -n 'kira-space' internal/appupdate/install.go internal/appupdate/checker.go
git apply --check docs/pending-changes/.github__workflows__release.yml.patch
```

## Tests

- `sh -n scripts/install.sh`; `shellcheck scripts/install.sh` when on `PATH`.
- `bun run verify:packaging` (S11 must pass with the new pin; run on Linux if the script allows,
  otherwise note which checks need macOS).
- `go test ./internal/appupdate/...`
- `go build ./...` and `go vet ./internal/appupdate/...`
- Pre-commit hook (lint, typecheck) passes on every commit, never `--no-verify`.

No new tests: a URL constant change is below the unit-test bar.
