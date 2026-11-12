package ade

import (
	"context"
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// base.go: the base of a task branch (P241). A base is stored on the branch row; baseCtxFor and
// resolveChoice turn a caller's choice into what to store, validated the same way on every write.

// baseCtxFor builds the git inputs the base resolver needs for one repo, with every live branch of
// that repo indexed.
func (b *TaskBoard) baseCtxFor(ctx context.Context, entry *gitsession.RepoEntry, codeRepoID string) (*boardCtx, error) {
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return nil, err
	}
	remote, _ := entry.DefaultRemote(ctx)
	sc := &boardCtx{
		entry: entry, repoID: codeRepoID, inv: inv, remote: remote,
		resolved: map[string]resolvedRef{}, byName: map[string]model.AdeTaskBranch{}, byID: map[string]model.AdeTaskBranch{},
	}
	mainFull, mainTip, hasMain, err := entry.MainRef(ctx)
	if err != nil {
		return nil, err
	}
	if hasMain {
		sc.mainName, sc.mainRef = mainDisplay(mainFull)
		sc.mainFull, sc.mainTip, sc.hasMain = mainFull, mainTip, true
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return nil, err
	}
	for _, br := range live {
		if br.CodeRepoID != codeRepoID {
			continue
		}
		sc.byID[br.ID] = br
		if br.Name != "" {
			putByName(sc.byName, br)
		}
	}
	return sc, nil
}

// baseTarget is a validated base choice: what to store and what git sees.
type baseTarget struct {
	base         string // stored ref name, '' = main
	baseBranchID string // stored planner parent
	name         string // display name
	ref          string // spelling git takes
	tip          string
	ok           bool
	reason       baseReason
}

func (t baseTarget) display(mainName string) string {
	if t.name != "" {
		return t.name
	}
	return mainName
}

// resolveChoice validates c as the base of self (nil for a branch not stored yet) and resolves it.
func (b *TaskBoard) resolveChoice(sc *boardCtx, self *model.AdeTaskBranch, nick string, c adewire.BaseChoice) (baseTarget, error) {
	if c.BranchID != "" {
		other, ok := sc.byID[c.BranchID]
		if !ok {
			return baseTarget{}, invalid("the base must be a branch of the same repo")
		}
		return b.plannerChoice(sc, self, other)
	}
	if c.Ref == "" || c.Ref == sc.mainName {
		return baseTarget{name: sc.mainName, ref: sc.mainRef, tip: sc.mainTip, ok: sc.hasMain}, nil
	}
	if self != nil && self.Name != "" && c.Ref == self.Name {
		return baseTarget{}, invalid("a branch cannot be its own base")
	}
	if other, ok := sc.byName[c.Ref]; ok {
		return b.plannerChoice(sc, self, other)
	}
	row, ok := resolveBaseRow(sc.inv, c.Ref, sc.remote)
	if !ok {
		return baseTarget{}, invalid("branch %q not found in %s", c.Ref, nick)
	}
	return baseTarget{base: c.Ref, name: c.Ref, ref: refSpelling(row), tip: row.Tip, ok: true}, nil
}

func (b *TaskBoard) plannerChoice(sc *boardCtx, self *model.AdeTaskBranch, other model.AdeTaskBranch) (baseTarget, error) {
	if self != nil {
		if other.ID == self.ID {
			return baseTarget{}, invalid("a branch cannot be its own base")
		}
		if queueCycle(sc.byID, self.ID, other) {
			return baseTarget{}, invalid("that base would form a cycle (%s is stacked on this branch)", cmpNonEmpty(other.Name, "an unnamed branch"))
		}
	}
	if other.Name == "" {
		return baseTarget{baseBranchID: other.ID, reason: baseParentDraft}, nil
	}
	br, ok := sc.plannerRef(other)
	if !ok {
		return baseTarget{}, invalid("branch %q not found in its repo", other.Name)
	}
	return baseTarget{base: other.Name, baseBranchID: other.ID, name: other.Name, ref: br.ref, tip: br.tip, ok: true}, nil
}

// storedBase resolves each repo's choice for a branch about to be stored, "" base for no choice.
func (b *TaskBoard) storedBase(ctx context.Context, codeRepoID, nick string, c adewire.BaseChoice) (base, baseBranchID string, err error) {
	if c.Ref == "" && c.BranchID == "" {
		return "", "", nil
	}
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return "", "", err
	}
	sc, err := b.baseCtxFor(ctx, entry, codeRepoID)
	if err != nil {
		return "", "", err
	}
	t, err := b.resolveChoice(sc, nil, nick, c)
	if err != nil {
		return "", "", err
	}
	return t.base, t.baseBranchID, nil
}

func (b *TaskBoard) repoNickOf(codeRepoID string) string {
	cfgs, err := b.repoConfigs()
	if err != nil {
		return codeRepoID
	}
	for _, c := range cfgs {
		if c.CodeRepoID == codeRepoID {
			return cmpNonEmpty(c.Nickname, c.Name)
		}
	}
	return codeRepoID
}

// SetBranchBase changes the base of a branch that does not exist in git yet. A created branch
// changes base only through Rebase.
func (b *TaskBoard) SetBranchBase(ctx context.Context, args adewire.SetBranchBaseArgs) error {
	sb, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return err
	}
	live, ok := tc.branch(args.BranchID)
	if !ok {
		return invalid("branch %s is not on a live task", args.BranchID)
	}
	if live.Kind != model.AdeBranchKindMine {
		return invalid("only your own branch has a base to set")
	}
	if live.Name != "" {
		return invalid("the branch exists: use Change base, which rebases it")
	}
	entry, err := b.openRepo(ctx, live.CodeRepoID)
	if err != nil {
		return err
	}
	sc, err := b.baseCtxFor(ctx, entry, live.CodeRepoID)
	if err != nil {
		return err
	}
	t, err := b.resolveChoice(sc, &live, tc.nick[live.CodeRepoID], args.Base)
	if err != nil {
		return err
	}
	if err := b.deps.Tasks.SetBranchBase(live.ID, t.base, t.baseBranchID, ""); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// descendants returns the ids of every live branch stacked, directly or not, on id.
func descendants(byID map[string]model.AdeTaskBranch, id string) map[string]bool {
	out := map[string]bool{}
	for _, br := range byID {
		if br.ID != id && queueCycle(byID, id, br) {
			out[br.ID] = true
		}
	}
	return out
}

// RepoBranches lists what the base picker offers for a repo: its main, the planner branches and the
// git branches, each excluded with a reason when it cannot be the base of branchID.
func (b *TaskBoard) RepoBranches(ctx context.Context, args adewire.RepoBranchesArgs) (adewire.RepoBranches, error) {
	entry, err := b.openRepo(ctx, args.CodeRepoID)
	if err != nil {
		return adewire.RepoBranches{}, err
	}
	sc, err := b.baseCtxFor(ctx, entry, args.CodeRepoID)
	if err != nil {
		return adewire.RepoBranches{}, err
	}
	out := adewire.RepoBranches{MainName: sc.mainName, Branches: []adewire.BasePick{}}
	var self model.AdeTaskBranch
	var below map[string]bool
	if args.BranchID != "" {
		self = sc.byID[args.BranchID]
		out.Previous = self.BasePendingFrom
		below = descendants(sc.byID, args.BranchID)
	}
	exclusion := func(id, name string) string {
		switch {
		case id != "" && id == args.BranchID, name != "" && name == self.Name:
			return "self"
		case id != "" && below[id]:
			return "stacked"
		}
		return ""
	}
	titles := map[string]string{}
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		return adewire.RepoBranches{}, err
	}
	for _, t := range tasks {
		titles[t.ID] = taskTitle(t, nil)
	}
	planned := map[string]bool{}
	for _, br := range sc.byID {
		if br.Name != "" {
			planned[br.Name] = true
		}
		pick := adewire.BasePick{
			Name: br.Name, BranchID: br.ID, TaskID: br.TaskID, TaskTitle: titles[br.TaskID], Draft: br.Name == "",
			Excluded: exclusion(br.ID, br.Name),
		}
		if br.Name != "" {
			pick.Local, pick.Remote = refPresence(sc, br.Name)
		}
		out.Branches = append(out.Branches, pick)
	}
	seen := map[string]bool{}
	for _, remoteOnly := range []bool{false, true} {
		for _, r := range sc.inv {
			if (r.Remote != "") != remoteOnly || seen[r.Short] || planned[r.Short] || r.Short == sc.mainName {
				continue
			}
			seen[r.Short] = true
			local, remote := refPresence(sc, r.Short)
			out.Branches = append(out.Branches, adewire.BasePick{Name: r.Short, Local: local, Remote: remote, Excluded: exclusion("", r.Short)})
		}
	}
	sort.SliceStable(out.Branches, func(i, j int) bool {
		bi, bj := out.Branches[i], out.Branches[j]
		if (bi.BranchID != "") != (bj.BranchID != "") {
			return bi.BranchID != ""
		}
		return bi.Name < bj.Name
	})
	return out, nil
}

func refPresence(sc *boardCtx, short string) (local, remote bool) {
	for _, r := range sc.inv {
		if r.Short != short {
			continue
		}
		if r.Remote == "" {
			local = true
		} else {
			remote = true
		}
	}
	return local, remote
}
