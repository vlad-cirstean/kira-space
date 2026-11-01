package scriptruns

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Clock is the scheduler's time source; a flow test supplies a fake.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

// Now drops the monotonic reading: it stops during system sleep on some platforms, wall time does not.
func (realClock) Now() time.Time                         { return time.Now().Round(0) }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

const (
	// defaultLate is how late a timer may fire before its fire is treated as missed (sleep, clock jump).
	defaultLate = 60 * time.Second
	// maxWait caps one timer. Runtime timers can stall during system sleep, so the loop re-reads
	// wall-clock time at least this often instead of trusting one long timer.
	maxWait = 30 * time.Second
)

type entry struct {
	next time.Time
	cron string
	tz   string
}

// Scheduler fires scripts with a schedule. Runs only while the app is open: a missed fire is
// skipped, never caught up. One goroutine owns every entry.
type Scheduler struct {
	Svc   *Service
	Clock Clock // nil = the real clock
	Late  time.Duration

	// fire, list and retire default to the Service paths; the unit test replaces them (no DB).
	fire   func(scriptID string, due time.Time)
	list   func() ([]scripts.CustomScript, error)
	retire func(scriptID, reason string)

	entries map[string]entry
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (s *Scheduler) clock() Clock {
	if s.Clock == nil {
		return realClock{}
	}
	return s.Clock
}

// Start loads the schedules and runs the loop.
func (s *Scheduler) Start() {
	s.once.Do(func() {
		if s.Late <= 0 {
			s.Late = defaultLate
		}
		if s.fire == nil {
			s.fire = s.fireScript
		}
		if s.list == nil {
			s.list = s.Svc.Scripts.List
		}
		if s.retire == nil {
			s.retire = s.Svc.retire
		}
		s.entries = map[string]entry{}
		s.wake, s.stop, s.done = make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
		go s.loop()
	})
}

// Reload re-reads the schedules; call it after any script change.
func (s *Scheduler) Reload() {
	if s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Close stops the loop and waits for a fire in progress.
func (s *Scheduler) Close() {
	if s.stop == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

func (s *Scheduler) loop() {
	defer close(s.done)
	clk := s.clock()
	s.reload(clk.Now())
	for {
		var timer <-chan time.Time
		earliest, ok := s.earliest()
		if ok {
			timer = clk.After(min(max(earliest.Sub(clk.Now()), 0), maxWait))
			// The clock may have moved between the two reads above.
			if !clk.Now().Before(earliest) {
				s.tick(clk.Now())
				continue
			}
		}
		select {
		case <-s.stop:
			return
		case <-s.wake:
			s.reload(clk.Now())
		case <-timer:
			s.tick(clk.Now())
		}
	}
}

func (s *Scheduler) earliest() (time.Time, bool) {
	var first time.Time
	for _, e := range s.entries {
		if first.IsZero() || e.next.Before(first) {
			first = e.next
		}
	}
	return first, !first.IsZero()
}

// reload syncs entries with the stored schedules: a changed cron or zone re-arms from now, a
// removed or disabled one drops and ends its waiting runs.
func (s *Scheduler) reload(now time.Time) {
	list, err := s.list()
	if err != nil {
		slog.Warn("scheduler: list scripts", "scope", "scriptruns", "err", err)
		return
	}
	known := map[string]*scripts.Schedule{}
	for i := range list {
		known[list[i].ID] = list[i].Schedule
	}
	for id := range s.entries {
		sch, exists := known[id]
		if exists && sch != nil && sch.Enabled {
			continue
		}
		delete(s.entries, id)
		reason := "the schedule was turned off"
		if !exists {
			reason = "the script was removed"
		}
		s.retire(id, reason)
	}
	for id, sch := range known {
		if sch == nil || !sch.Enabled {
			continue
		}
		if e, ok := s.entries[id]; ok && e.cron == sch.Cron && e.tz == sch.Timezone {
			continue
		}
		next, err := scripts.NextFires(sch.Cron, sch.Timezone, now, 1)
		if err != nil || len(next) == 0 {
			delete(s.entries, id)
			continue
		}
		s.entries[id] = entry{next: next[0], cron: sch.Cron, tz: sch.Timezone}
	}
}

// tick fires every due entry in order of its fire time, then moves each to its next one.
func (s *Scheduler) tick(now time.Time) {
	type due struct {
		id string
		e  entry
	}
	var dues []due
	for id, e := range s.entries {
		if !e.next.After(now) {
			dues = append(dues, due{id, e})
		}
	}
	sort.Slice(dues, func(i, j int) bool { return dues[i].e.next.Before(dues[j].e.next) })
	for _, d := range dues {
		if now.Sub(d.e.next) <= s.Late {
			s.fire(d.id, d.e.next)
		}
		s.advance(d.id, d.e, now)
	}
}

// advance moves an entry past its fire: the next instant, and from
// now when that is still in the past (a long sleep).
func (s *Scheduler) advance(id string, e entry, now time.Time) {
	nexts, err := scripts.NextFires(e.cron, e.tz, e.next, 1)
	if err == nil && (len(nexts) == 0 || !nexts[0].After(now)) {
		nexts, err = scripts.NextFires(e.cron, e.tz, now, 1)
	}
	if err != nil || len(nexts) == 0 {
		delete(s.entries, id)
		return
	}
	e.next = nexts[0]
	s.entries[id] = e
}

// fireScript starts one due run: a skipped row, a waiting row for the popup, or the run itself.
func (s *Scheduler) fireScript(scriptID string, due time.Time) {
	svc := s.Svc
	rec, err := svc.Scripts.Get(scriptID)
	if err != nil {
		slog.Warn("scheduler: load script", "scope", "scriptruns", "err", err)
		return
	}
	if rec == nil || rec.Schedule == nil || !rec.Schedule.Enabled {
		return
	}
	active, err := svc.Runs.Active(scriptID, "")
	if err != nil {
		slog.Warn("scheduler: active run", "scope", "scriptruns", "err", err)
		return
	}
	if active != nil {
		svc.skip(*rec, due, overlapText(*rec, *active))
		return
	}
	p, err := svc.plan(svc.lifeCtx(), scheduleArgs(*rec, nil), planOpts{headless: true})
	if err != nil {
		svc.skip(*rec, due, errText(err))
		return
	}
	if why := unstartable(rec, p); why != "" {
		svc.skip(*rec, due, why)
		return
	}
	if rec.Schedule.Confirm {
		run := svc.newRun(p, nil)
		run.State, run.StartedAt, run.CreatedAt, run.Trigger = StateWaiting, nil, due.UnixMilli(), TriggerScheduled
		run.SessionID = ""
		if svc.record(run) {
			svc.openPrompt(run)
		}
		return
	}
	if _, err := svc.startScheduled(p, nil); err != nil {
		svc.skip(*rec, due, errText(err))
	}
}

// unstartable says why a due run cannot go ahead, "". With confirm on a missing secret is fine:
// the popup asks for it.
func unstartable(rec *scripts.CustomScript, p *planned) string {
	if p.preview.Blocker != "" {
		return p.preview.Blocker
	}
	var missing []string
	for _, name := range p.preview.Missing {
		if rec.Schedule.Confirm && isSecret(rec.Params, name) {
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		why := "no value for " + strings.Join(missing, ", ")
		if p.preview.SecretIssue != "" {
			why += ". " + p.preview.SecretIssue
		}
		return why
	}
	return ""
}

func isSecret(params []scripts.Param, name string) bool {
	for _, p := range params {
		if p.Name == name {
			return p.Secret
		}
	}
	return false
}
