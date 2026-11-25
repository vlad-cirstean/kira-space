# Audit: false "merged into main"

Producer: `computeBranch`, `apps/kira-space/internal/ade/board_facts.go:389-400`:
`hadCommits := sb.HadCommits || depth > 0` (depth = tip commits not in main), `byAncestor := mergedRule(hasMain && depth == 0, hadCommits)`,
`MergedIntoMain = byAncestor || sb.MergedAt != nil`. Second producer: `checkPrMerges` (`ade/board.go:1159-1182`).
`had_commits` and `merged_at` are write-once (`MarkBranchFacts`, `storage/repos/adetask.go:1060-1089`).
Consumers: board_facts.go:403/406, artifacts.go:407, archive.go:107, rebase.go:111/192, board.go:812;
frontend wire.ts:282 -> actions.ts, branchGraph.ts, rebaseActions.ts, taskOrder.ts, timeline.ts, headerActions.ts, usePlanModel.ts.

Causes (1-3 reproduced or evidenced; repro test in the audit scratchpad `zz_mergedfp_test.go`, uses `boardHarness`):
1. `merged_at` never clears: new commits after a merge still read merged (reproduced, ahead=1 merged=true). Makes every false signal permanent.
2. Child stacked on a parent or a task based on `develop` with zero own commits: `had_commits` computed against main counts the parent's commits; parent merged later makes the child tip an ancestor of main (reproduced for `--no-ff` and fast-forward).
3. `checkPrMerges` accepts any merged PR matched by head ref name only (`gitsession/changerequests.go:539-593`, `githost/github/pr.go:149` state=all). No HeadSha check. Reused names (untitled tasks all derive `feat/new-task`, `setup.go:230`) hit this (not measured).
4. Commits dropped with `had_commits` kept (reset --hard origin/main, restack) read merged. Reverted commits still read merged (accepted).
Ruled out: fresh zero-commit branch at main tip; stale origin/main (false negatives only).

Definition: merged only if the branch has at least one own commit (counted from creation/resolved base, not main) and either all own commits are reachable from main, or a merged PR HeadSha equals or contains the current tip. No own commits: never merged. A recorded merge stops counting once the tip moves past it.

Fixes:
1. `board_facts.go:389`: `hadCommits := sb.HadCommits || wb.Ahead > 0` with Ahead against resolved base (stronger: store start tip at creation, count start..tip). Existing rows hold wrong `had_commits=1`: one-time reset where Ahead == 0.
2. Add `merged_tip` column written by `MarkBranchFacts` (row.Tip for ancestor path, PR HeadSha for PR path); `MergedIntoMain = byAncestor || (MergedAt != nil && MergedTip == row.Tip)`; migration.
3. `checkPrMerges` (`board.go:1172`): require `cr.HeadSha == wb.Tip` or tip ancestor of HeadSha; optionally PR updated after `sb.AddedAt`.
4. Fix `mergedRule` comment (`facts.go:124-126`).
Also check `newlyMerged` (`board.go:1070`) false "merged into" notice (inferred).

Tests (flow + UI per CLAUDE.md): adeflow contract JSON next to `base_test.go`/`rebase_test.go` (`tests/contract/ade-base.json` style): (a) stacked child, parent merged --no-ff: not merged; (b) task on develop, develop ff into main: not merged; (c) merged then follow-up commit: not merged, Ahead 1; (d) forge returns merged PR with other HeadSha, same name: not merged; (e) control real merge reads merged. UI spec (extend `ade-v2-main-branches.spec.ts` or `ade-v2-plan.spec.ts`): no Merged tag, rebase/conflict actions offered for a-d.
