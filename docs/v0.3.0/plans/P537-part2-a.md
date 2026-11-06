# P537 Part 2, Stream A notes

Base `677d6c60`. Worktree `kira-p537-a`, branch `p537-stream-a`.

## Done

1. One `flowharness`. `studioflowharness` merged in: one `App` over both homes, Studio HTTP/gRPC,
   docker, kube fakes, `cmd/flowservers`, `Contract`/`StudioContract` over one private helper.
2. One `flows/` tree. Seven clashes merged; Studio tests for services now merged deleted or adapted.
   `windowflow/studiotabkinds_test.go` checks the Studio tab-kind agreement. `layering_test.go`
   drops the `studio*` names.
3. One `tests/claude` module. `claude-studio` removed. `go.mod` gains `modernc.org/sqlite` direct.
4. `internal/appmerge` + `main.go` call (`carryOverStudio`) + `flows/appflow/appmerge_test.go`.
5. `StepOldStudio` in `startupfail`; `oldstudio_darwin.go` guard, no-op elsewhere.
6. Copy: About text, `dbmcp`, `mcpauth`, `datagrip`, `httpclient` User-Agent, `llmprompts` registry,
   `g1measure` anchor, `appupdate.Studio` removed, `apps/kira-studio` path comments.
7. Menu: Space items first, Studio items after. One app menu, one Settings item, one Window menu.

## Deviations and facts

- `appmerge` skips backup and marker work when the Studio side holds no rows and no
  `llm-prompts.json`: nothing to carry, nothing to protect. Marker row still written.
- Marker is `settings` key `appmerge.p537`, JSON `{state, fresh, backup}`. `fresh` is fixed on the
  first run so a resume still imports settings.
- Studio tabs keep living in the Studio DB (Part 1 routing). Re-key moves them to Space's first
  non-ephemeral window, creating the Studio `windows` row it needs. Active flag kept only if the
  target holds no active tab.
- Layout import: `modes.{studio,api,docker,kubernetes}.*` only; Space rows win. `automations`
  stays Space's.
- Multi-stack `dbmcp` script control and migration `0076` (script-run `mcp` trigger) came with
  the merged scripts service.
- `llmprompts.Bound.App == ""` now lists every entry (267, was 144 per app).
- Keychain constants in `internal/secrets/keyring_darwin.go` keep "Kira Studio": they key existing
  secrets.
- `internal/llmprompts/testdata/P289-runs/before.json` and
  `tests/claude/testdata/memory-eval/` stay: frozen recorded data naming Kira Studio.
- Darwin guard file not compiled here (Wails needs cgo on darwin); signatures checked by hand.
- Bindings regenerated: 62 services, 519 methods, tree identical to the Part 1 snapshot.
- Studio-stack-only tests deleted: `TestPreviewRefusesTaskInStudio`, `TestStudioRefusesTaskContext`
  (merged service accepts task context).

## Checks run

`go build ./...`; `go build -tags server ./apps/kira-space/...`; `go vet ./...`; `bun run lint:go`
(0 issues); bindings regenerate and diff; `go test` for `flows/...`, `flowharness`, `appwire`,
`bridge`, `storage`, `dbmcp`, `mcpauth`, `httpclient`, `datagrip`, `appshell`, `appmerge`,
`appupdate`, `llmprompts`, `startupfail`; flows ran in contract compare mode; `go vet` in
`tests/claude`.
