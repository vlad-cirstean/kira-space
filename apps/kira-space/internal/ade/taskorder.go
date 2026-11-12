package ade

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// TaskListOrder is the order the Tasks UI shows: the Plan sequence (merge order), then Later, then
// Backlog. Line-for-line port of frontend/src/ade/v2/board/taskOrder.ts (taskListOrder), kept equal
// by testdata/taskorder/cases.json; regenerate it with
// `UPDATE_TASK_ORDER=1 bun test apps/kira-space/tests/unit/ade-v2-task-order-parity.spec.ts`.
// today is a local `YYYY-MM-DD`.
func TaskListOrder(b adewire.Board, s model.AdeSettings, today string) []string {
	o := orderCtx{b: b, cal: newOrderCalendar(today, s)}
	o.graph = newOrderGraph(b)
	seq := o.sequence(s, today)
	ids := make([]string, 0, len(b.Tasks))
	for _, e := range seq {
		ids = append(ids, e.taskID)
	}
	ids = append(ids, o.shelfIDs(shelfLater)...)
	return append(ids, o.shelfIDs(shelfBacklog)...)
}

// orderLater is the "no day assigned yet" offset (calendar.ts LATER).
const orderLater = 9999

type orderShelf int

const (
	shelfPlan orderShelf = iota
	shelfLater
	shelfBacklog
)

type orderEntry struct {
	taskID   string
	kind     string
	day, end int
	pos      int
}

type orderCtx struct {
	b     adewire.Board
	cal   orderCalendar
	graph orderGraph
}

// --- calendar.ts ---------------------------------------------------------------------------------

type orderCalendar struct {
	todayDays    int
	offDays      map[int]bool
	workWeekends map[int]bool
}

func newOrderCalendar(today string, s model.AdeSettings) orderCalendar {
	t := isoToDays(today)
	offsets := func(dates []string) map[int]bool {
		m := make(map[int]bool, len(dates))
		for _, d := range dates {
			m[isoToDays(d)-t] = true
		}
		return m
	}
	return orderCalendar{todayDays: t, offDays: offsets(s.OffDays), workWeekends: offsets(s.WorkWeekendDays)}
}

// isoToDays is days since the epoch of a `YYYY-MM-DD`, in UTC (no DST drift), as Date.UTC / 86400000.
func isoToDays(iso string) int {
	p := strings.Split(iso, "-")
	if len(p) != 3 {
		return 0
	}
	y, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	d, _ := strconv.Atoi(p[2])
	sec := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Unix()
	return int(math.Floor(float64(sec) / 86400))
}

func (c orderCalendar) isWeekend(k int) bool {
	days := c.todayDays + k
	dow := (((days % 7) + 7) % 7)
	dow = (dow + 4) % 7 // 1970-01-01 was a Thursday (index 4).
	return dow == 0 || dow == 6
}

func (c orderCalendar) isOff(k int) bool {
	return c.offDays[k] || (c.isWeekend(k) && !c.workWeekends[k])
}

func (c orderCalendar) nextWork(k int) int {
	n := k + 1
	for c.isOff(n) {
		n++
	}
	return n
}

func (c orderCalendar) spanDays(start, n int) []int {
	out := []int{start}
	k := start
	for len(out) < n {
		k = c.nextWork(k)
		out = append(out, k)
	}
	return out
}

const maxSpanDays = 60

// jsSpace is JS `\s` and `String.prototype.trim`'s set; Go's `\s` and TrimSpace are narrower.
const jsSpace = "\t\n\v\f\r \u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

var (
	estPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)[` + jsSpace + `]*(m|h|d|w)$`)
	jsTrimSet  = "\t\n\v\f\r \u00a0\u1680\u2028\u2029\u202f\u205f\u3000\ufeff"
)

func isJSSpace(r rune) bool {
	return strings.ContainsRune(jsTrimSet, r) || (r >= 0x2000 && r <= 0x200a)
}

// estDays is parseEst's `days`; ok is false for an unparseable estimate.
func estDays(raw string, workdayHours float64) (days int, ok bool) {
	m := estPattern.FindStringSubmatch(strings.ToLower(strings.TrimFunc(raw, isJSSpace)))
	if m == nil {
		return 0, false
	}
	n, _ := strconv.ParseFloat(m[1], 64) // range error leaves +Inf, as JS
	clamp := func(v float64) int { return int(math.Min(maxSpanDays, math.Max(1, v))) }
	switch m[2] {
	case "m":
		return 1, true
	case "h":
		return clamp(math.Ceil(n/workdayHours - 1e-9)), true
	case "d":
		return clamp(math.Ceil(n)), true
	default:
		return clamp(math.Ceil(n * 5)), true
	}
}

// --- branchGraph.ts ------------------------------------------------------------------------------

type orderGraph struct {
	byBranch  map[string]adewire.Branch
	byTask    map[string]adewire.Task
	parentOf  map[string]string
	conflicts map[string]map[string]bool
}

func (g orderGraph) ancestors(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	for p, ok := g.parentOf[id]; ok && !seen[p]; p, ok = g.parentOf[p] {
		out = append(out, p)
		seen[p] = true
	}
	return out
}

func pairKey(a, b string) string { return a + "\x00" + b }

func newOrderGraph(b adewire.Board) orderGraph {
	g := orderGraph{
		byBranch:  make(map[string]adewire.Branch, len(b.Branches)),
		byTask:    make(map[string]adewire.Task, len(b.Tasks)),
		parentOf:  map[string]string{},
		conflicts: map[string]map[string]bool{},
	}
	for _, br := range b.Branches {
		g.byBranch[br.ID] = br
	}
	for _, t := range b.Tasks {
		g.byTask[t.ID] = t
	}
	var planBranches []adewire.Branch
	for _, t := range b.Tasks {
		for _, id := range t.BranchIDs {
			if br, ok := g.byBranch[id]; ok {
				planBranches = append(planBranches, br)
			}
		}
	}
	g.setParents(b, planBranches)
	g.setConflicts(b, planBranches)
	return g
}

// setParents gives each plan branch its queued-after branch, else its base when that is on the board.
func (g *orderGraph) setParents(b adewire.Board, planBranches []adewire.Branch) {
	for _, br := range planBranches {
		p := b.Plan.QueuedAfter[br.ID]
		if _, onBoard := g.byBranch[br.BaseBranchID]; p == "" && onBoard {
			p = br.BaseBranchID
		}
		if p != "" {
			g.parentOf[br.ID] = p
		}
	}
}

// setConflicts marks each own live branch and each review branch of its repo that conflict with it,
// unless the review is one of its ancestors.
func (g *orderGraph) setConflicts(b adewire.Board, planBranches []adewire.Branch) {
	pairs := make(map[string][]string, 2*len(b.Pairs))
	for _, p := range b.Pairs {
		pairs[pairKey(p.A, p.B)] = p.Conflicts
		pairs[pairKey(p.B, p.A)] = p.Conflicts
	}
	add := func(id, with string) {
		if g.conflicts[id] == nil {
			g.conflicts[id] = map[string]bool{}
		}
		g.conflicts[id][with] = true
	}
	for _, br := range planBranches {
		if br.Kind != "mine" || br.MergedIntoMain || br.Name == "" {
			continue
		}
		anc := g.ancestors(br.ID)
		for _, r := range planBranches {
			if r.Kind != "review" || r.CodeRepoID != br.CodeRepoID || slices.Contains(anc, r.ID) {
				continue
			}
			if len(pairs[pairKey(br.ID, r.ID)]) > 0 {
				add(br.ID, r.ID)
				add(r.ID, br.ID)
			}
		}
	}
}

// --- shelf.ts ------------------------------------------------------------------------------------

func (o orderCtx) shelfOf(taskID string) orderShelf {
	if slices.Contains(o.b.Plan.Backlog, taskID) {
		return shelfBacklog
	}
	var task *adewire.Task
	for i := range o.b.Tasks {
		if o.b.Tasks[i].ID == taskID {
			task = &o.b.Tasks[i]
			break
		}
	}
	if task == nil || task.Kind == "review" {
		return shelfPlan
	}
	if _, dated := o.b.Plan.Day[taskID]; !dated {
		return shelfLater
	}
	return shelfPlan
}

func (o orderCtx) shelfIDs(shelf orderShelf) []string {
	inOrder := make(map[string]bool, len(o.b.Plan.Order))
	ids := append([]string{}, o.b.Plan.Order...)
	for _, id := range o.b.Plan.Order {
		inOrder[id] = true
	}
	for _, t := range o.b.Tasks {
		if !inOrder[t.ID] {
			ids = append(ids, t.ID)
		}
	}
	out := []string{}
	for _, id := range ids {
		if o.shelfOf(id) == shelf {
			out = append(out, id)
		}
	}
	return out
}

// --- taskOrder.ts --------------------------------------------------------------------------------

func (o orderCtx) mineEntries(s model.AdeSettings, today string) []orderEntry {
	var out []orderEntry
	todayDays := isoToDays(today)
	for _, t := range o.b.Tasks {
		iso, dated := o.b.Plan.Day[t.ID]
		review := t.Kind == "review"
		if review && !dated {
			continue
		}
		day := orderLater
		if dated {
			day = isoToDays(iso) - todayDays
		}
		pos := 999
		for i, id := range o.b.Plan.Order {
			if id == t.ID {
				pos = i
				break
			}
		}
		span := 1
		if day != orderLater && !review {
			if d, ok := estDays(t.Est, s.WorkdayHours); ok {
				span = d
			}
		}
		days := []int{orderLater}
		if day != orderLater {
			days = o.cal.spanDays(day, span)
		}
		out = append(out, orderEntry{taskID: t.ID, kind: t.Kind, day: day, end: days[len(days)-1], pos: pos})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.end != b.end {
			return a.end < b.end
		}
		if a.day != b.day {
			return a.day < b.day
		}
		return a.pos < b.pos
	})
	return out
}

func (o orderCtx) buildsOnOrConflicts(t adewire.Task, rb string) bool {
	for _, bid := range t.BranchIDs {
		if slices.Contains(o.graph.ancestors(bid), rb) || o.graph.conflicts[bid][rb] {
			return true
		}
	}
	return false
}

// sequence is planSequence's `seq`: planned tasks in merge order with review items placed.
func (o orderCtx) sequence(s model.AdeSettings, today string) []orderEntry {
	var mine []orderEntry
	for _, e := range o.mineEntries(s, today) {
		if o.shelfOf(e.taskID) == shelfPlan {
			mine = append(mine, e)
		}
	}
	var reviews []adewire.Task
	for _, t := range o.b.Tasks {
		if _, dated := o.b.Plan.Day[t.ID]; t.Kind == "review" && !dated {
			reviews = append(reviews, t)
		}
	}
	var seq []orderEntry
	placed := map[string]bool{}
	for _, me := range mine {
		if me.kind == "review" {
			seq = append(seq, me)
			continue
		}
		t := o.graph.byTask[me.taskID]
		for _, r := range reviews {
			if placed[r.ID] || len(r.BranchIDs) == 0 || !o.buildsOnOrConflicts(t, r.BranchIDs[0]) {
				continue
			}
			placed[r.ID] = true
			seq = append(seq, orderEntry{taskID: r.ID, kind: "review", day: me.day, end: me.end, pos: 999})
		}
		seq = append(seq, me)
	}
	for _, r := range reviews {
		if !placed[r.ID] {
			seq = append(seq, orderEntry{taskID: r.ID, kind: "review", day: orderLater, end: orderLater, pos: 999})
		}
	}
	return seq
}
