# v0.2.28 code review, round 1

Base: `ad451b19` (PR #22 squash merge of v0.2.27). No later review session commit exists, so the diff is
`ad451b19..5051f070`: config-as-files (configfs, runs, API, Space repo, Studio connections plus their
migrations), git IPC on bound `GitService` (P476 to P479), RAM work (P480, P481), full-suite fixes. Fixer and
full-suite commits were reviewed as unreviewed code.

Totals: 1 High, 3 Medium, 4 Low.

## F1 (High): failed connections migration deletes the connections it did not write

- Where: `apps/kira-studio/internal/connfiles/migrate.go:21-38` (`migrate`), `:107-161` (`bindLegacyRows`),
  `:199` (seeds `config_json`); `apps/kira-studio/internal/connfiles/loader.go:84-88` (`Start`);
  `apps/kira-studio/internal/connfiles/projection.go:64-79,106-131` (`Snapshot`, `Forget`).
- Problem: `bindLegacyRows` commits stems and `config_json` for every legacy row in one transaction. Files are
  written after that commit. `Snapshot` reports every row with a stem and a non-empty `config_json` as last
  good. If any `WriteAtomic` fails (disk full, permission error on `~/.kira-studio/connections`, read-only home),
  `migrate` returns an error and `Start` only logs it, then calls `Reload`. `configfs.Folder.List` sees those
  rows in the snapshot without a file and calls `Forget`. `Forget` runs `OnForget` and `DELETE FROM connections`.
  Eight tables reference `connections(id) ON DELETE CASCADE`.
- Failure scenario: user upgrades with 10 connections; the 4th file write fails. Rows 4 to 10 are deleted with
  their cascaded state. The next start reruns the migration, but the rows are gone, so nothing is exported.
  Sealed passwords stay orphaned in `secret_values`. Unrecoverable data loss.
- Contrast: runs (`internal/scripts/loader.go:333-395`) writes the file first and seeds `config_json` after;
  API (`apifiles`) keeps its last good state in `config_last_valid`, which only a load writes.
- Fix: seed `config_json` per row only after its file exists (write the file, then `UPDATE ... config_json`),
  as runs does. Also skip `Reload` and the watch when `MigrateOnce` fails, so an unfinished migration never
  projects. Add a flow test that fails one file write and asserts every row survives and the rerun completes.

## F2 (Medium): API migration rerun loses promoted Postman variables and overwrites files

- Where: `apps/kira-studio/internal/apifiles/migrate.go:22-35` (`migrate`), `:251-272` (`promoteOrigin`),
  `:274-310` (`collections`).
- Problem: `configfs.MigrateOnce` requires `fn` to be safe to rerun. `promoteOrigin` strips `variable` from
  `origin_json` and sets `variables_promoted = 1` inside the committed transaction. The promoted variables live
  only in the rendered file text. On a rerun, `c.promoted` is true, so `promoted` is nil and those variables
  are absent from the regenerated text. `migrate` then writes every file unconditionally (no existence check,
  unlike connections and runs), so a file the first run already wrote is overwritten with the poorer text.
- Failure scenario: first run writes `collection.yaml` with 5 promoted variables, then the environment file
  write fails (or the process dies before the marker). Next start rewrites `collection.yaml` without the 5
  variables. Their non-secret values are lost; their sealed secrets stay orphaned. Any edit the user made to
  the file between the two starts is also overwritten.
- Fix: skip a file that already exists, as connections and runs do. Keep the promoted variables recoverable on
  rerun: either stamp `variables_promoted` and strip `origin_json` only after the file is on disk, or persist
  the promoted list in the transaction and reread it.

## F3 (Medium): a bound git call after window close recreates and leaks the window's Conn

- Where: `apps/kira-space/internal/bridge/gitservice.go:26-47` (`callGit` uses `windows.get`);
  `apps/kira-space/internal/bridge/gitwindows.go:74-110` (`get`, `getLocked`);
  `apps/kira-space/internal/gitrpc/handlers.go:111-125` (per-Conn goroutines).
- Problem: `OnWindowClosing` calls `GitWindows.Close(key)`. A bound call that reaches Go afterwards (a
  `status.get` poll, a `repo.close` from `repoHolds.release`, any call queued while the window was closing)
  goes through `get`, which creates a fresh Conn for the dead key. Nothing closes it again. The graph plane
  avoids this with `lookup` (its comment states the rule), but every `GitService` method does not.
- Failure scenario: each leaked Conn keeps two goroutines and a `repoSettingsChanged` subscription until
  process exit, and emits `repoSettings.changed` to a dead window key. A late `repo.open` also holds a
  `RepoEntry`, so its watcher and the 5-minute teardown never run. Leaks grow with window churn.
- Fix: track closed keys, or have `GitWindows.Close` mark the key dead, and answer `E_CONFLICT` for calls on
  a dead key, the same refusal the plane uses. Only `ResetWindow` (page boot) may create the Conn again; let
  it clear the dead mark. Also correct `docs/ARCHITECTURE.md:1644` ("a late open never recreates a closed
  window" holds for the plane only).

## F4 (Medium): changing a duplicated connection's password silently changes the original's

- Where: `apps/kira-studio/internal/connfiles/loader.go:565-584` (`Duplicate`: "Secret references are
  shared with the original"); `apps/kira-studio/internal/connections/service.go:352-363` (`Duplicate`, still
  bound via `internal/bridge/connections.go:51`), `:424-459` (`UpdateCredentials`).
- Problem: the copy's file holds `${secret:<orig>.password}`. `UpdateCredentials` on the copy finds that name
  through `SecretName(rec.File.Password)` and `PutSecret`s the new value under it. The original connection now
  connects with the copy's password. Its live session is not reconnected either (only the copy's id is), so
  the change surfaces later as a confusing auth failure on the original.
- Failure scenario: user duplicates "prod" to make "staging", enters the staging password in the credentials
  prompt; "prod" now carries the staging password.
- Fix: copy-on-write in `UpdateCredentials`: when another file references the same secret name, seal the new
  value under a fresh `<stem>.password` and patch this file's ref to it. Alternatively, `Duplicate` copies the
  sealed envelopes to `<newstem>.password` / `<newstem>.uri` (verbatim copy is valid: the scope is the AD).

## F5 (Low): transport `repo.close` bypasses the window refcount

- Where: `apps/kira-space/frontend/src/repo/git/transport.ts:144-154`.
- Problem: since P479 the transport's `repo.close` sends a bound `repo.close` to the shared window Conn, on top
  of releasing its `repoHolds` entry. Go releases the Conn's single hold for that repo even while `repoHeads`,
  `worktrees` or review consumers still count on it; their requests then fail with `repoNotOpen` and their
  `repo.changed` events stop. Before P479 this close went to the transport's own stream Conn, so it was
  harmless. Latent today: `RepoState.close()` (`packages/git-ui/src/state/repo.ts:78`) has no production
  caller.
- Fix: drop the direct `send` and rely on `hold.release()`, which already sends `repo.close` on the last
  release.

## F6 (Low): stale docs and comments after the stream protocol removal

- `apps/kira-space/internal/graphplane/credit.go:8-12`: names deleted `rpc.ts` and says `graph.stream`
  "never calls acquire"; `emit` now acquires on every chunk.
- `docs/ARCHITECTURE.md:1702`: a window's Conn is "released ... when its last stream closes"; no stream owns
  the Conn now, only `GitWindows.Close` and `ResetWindow` release it.
- `docs/ARCHITECTURE.md:1894`: "no process writes them over a bound call" for the prepare script and base
  path. `RepoConfigService.SaveFile` (bound) writes whole repo file text, prepare script included. Same trust
  level as the removed `AdeTaskService.UpdateRepo`, so not a regression; state that the guard covers
  `repoSettings.set` only.
- `packages/git-ui/src/state/graphView.ts:607`: refers to `rpc.ts`'s per-chunk behaviour.
- Fix: correct the four texts.

## F7 (Low): `apifiles.Store.migrated` is written, never read

- Where: `apps/kira-studio/internal/apifiles/store.go:40,68-74`.
- Problem: dead field. It looks like the intended guard for F1/F2-style partial migrations (do not project
  before the migration finished) that was never wired.
- Fix: use it to gate `Sync` and the watch, or delete it.

## F8 (Low): size-capped git results are encoded three times

- Where: `apps/kira-space/internal/bridge/gitservice.go:36-44`.
- Problem: `commit.fileDiff` and `file.read` encode their result to `json.RawMessage` for the 6 MiB cap,
  `callGit` decodes it into `T`, and Wails encodes `T` again for the renderer. A large diff pays two extra
  full JSON passes and a decoded copy on every open.
- Fix: return the capped `json.RawMessage` straight through for those two methods (it implements
  `json.Marshaler`), keeping the typed result only for the generated binding signature, or move the cap check
  to the encoded size Wails produces.

## Checked, no finding

- `graphplane` credit and cancel: cancel unblocks `acquire` through the stream ctx, `Serve` waits for every
  stream before stopping the writer, and the client stops granting after abort. The duplicate-id path cannot
  occur from the shipped client (each dial is a new `Serve` with its own ids).
- `repoHolds.ts` ordering per path, `boundGit.ts` reset memo and abort handling, `ResetWindow` against the old
  page's in-flight `repo.open` (`Conn.closed` releases it).
- Repo config, runs migrations: idempotent; legacy tables drop only after files exist.
- Connection files never hold a password or URI: `x-kira-secret` refs only, secret option keys refused, the
  migration copies sealed envelopes without decrypting. No CLI credential is touched.
- New Vue files: `<script setup lang="ts">`, no scoped `<style>`.
