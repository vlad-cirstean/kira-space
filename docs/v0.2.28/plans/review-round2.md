# v0.2.28 code review, round 2

Base: `ad451b19` (same as round 1). Diff `ad451b19..9198ca70`, full chapter. Round-1 fixer commits
(`46e22f33`, `60555938`, `0e17a23c`, `9198ca70`) and the full-suite fix commits reviewed as unreviewed code.

Totals: 0 High, 1 Medium, 0 Low.

## F1 (Medium): stem-derived connection secret names overwrite and delete secrets other files use

- Where: `apps/kira-studio/internal/connfiles/loader.go:570-617` (`Duplicate`, `dropCopies`);
  `apps/kira-studio/internal/connfiles/secrets.go:73-83` (`Copy`: `ON CONFLICT ... DO UPDATE`);
  `apps/kira-studio/internal/connfiles/loader.go:500-529` (`stemTaken`, `freeStemLocked`);
  `apps/kira-studio/internal/connections/service.go:287-327` (`Create`, `createAt`), `:440-446`
  (`UpdateCredentials` fallback name).
- Problem: every write path names a new secret `<stem>.password` / `<stem>.uri` and assumes the stem owns that
  name. `stemTaken` checks only files and rows, never `secret_values` or other files' `${secret:...}` refs.
  Secret refs survive a file rename, so a free stem can still have its secret name in use. `Copy` and
  `PutSecret` then overwrite that value, and the failure paths (`dropCopies`, `createAt`'s `DeleteSecret` on
  `ErrExists`) delete it. Same outcome as round-1 F4: one connection's credentials silently change another's.
- Failure scenario: user duplicates `prod` (gets `prod-copy.yaml`, secret `prod-copy.password`), renames the
  file to `staging.yaml` and sets a staging password. Stem `prod-copy` is now free. Duplicating `prod` again
  picks `prod-copy`, and `Copy` overwrites `prod-copy.password` with prod's password: `staging` now connects
  with prod's credentials. Creating a new connection named "prod copy" does the same via `createAt`. Race
  variant: two concurrent `Duplicate`s (or `Create`s) of one source both pick the same stem outside `l.mu`,
  both write the secret, the loser hits `ErrExists` and deletes the secret the winner's file refers to.
- Fix: treat a stem as taken when `<stem>.password` or `<stem>.uri` already exists for owner `connections`
  (or any file references it). Make the new-name writes insert-only (`ON CONFLICT DO NOTHING`, check rows
  affected, pick the next stem on conflict), so the failure path deletes only a value this call inserted.
  Apply the same to `UpdateCredentials`' fallback name.

## Checked, no finding

- Connections migration (`46e22f33`): `config_json` is seeded per row only after its file exists; `Snapshot`
  excludes unseeded rows, so a failed write cannot reach `Forget`. Rerun reuses stored stems, skips existing
  files, `copyEnvelope` is `DO NOTHING`, `dropLegacy` is idempotent. A failed migration skips load and watch;
  create, patch and delete still project only files, and `Reload` still excludes unseeded legacy rows.
- API migration (`46e22f33`): existing files are never overwritten; the promoted-variable stamp is applied
  only after the file exists, so a rerun renders the variables again. Legacy secret rows bind `secret_ref` in
  the transaction and rerun to the same name.
- Known gap 1, orphaned sealed promoted secret after a crash between commit and file write (or between write
  and stamp): the rerun seals a `-2` name, the first stays unreferenced. Judged no fix and no Known open items
  entry: one-shot migration, crash-only, the copy stays encrypted in the same store, and the API secrets list
  shows it as unreferenced with Delete.
- Known gap 2, one extra encode for the size cap (`0e17a23c`): the cap is on encoded size, so measuring needs
  one encode; the decode and copy are gone. One pass over at most 6 MiB per open. No fix, no entry.
- `GitWindows` closed set (`0e17a23c`): `get` checks the mark under `mu`; `Close` racing an in-flight call
  leaves that call on a closed Conn, which fails or detaches as before. `Reset` from page boot runs before
  every other call (`gitWindowReady` memo). The hide-instead-of-close path cancels `WindowClosing` before
  listeners run, so a hidden window is never marked dead. A late `ResetWindow` after `Close` clears the mark
  but creates no Conn, and the page is already `about:blank`. The `closed` map keeps one key per closed
  window: negligible.
- E_CONFLICT on a closed window only reaches a page that is unloading; no UI path depends on it.
- Transport `repo.close` (`9198ca70`): releases only the transport's own hold; the last release sends the
  bound close.
- `encodedSize` counts `Encoder.Encode`'s trailing newline: one byte off the cap, harmless.
- Full-suite fixes: the dropped Space spec covered the removed `script-use-ade-dir` Switch (no UI element
  left; `termflow/runs_file_test.go` covers `use_ade_dir`). Studio spec and visual baseline changes follow
  intended P477 UI changes (removed `color`/`readonly` menu items, new-connection dialog). The site visual
  spec's load/error wait settles every image without `decode()`; the screenshot comparison still retries
  until stable. No lost coverage.
