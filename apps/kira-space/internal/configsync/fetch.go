package configsync

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// FetchNow fetches the repository and recomputes what a pull would change. Credential prompts go to
// the window of windowKey. A failed fetch is a problem in the status, not an error.
func (s *Service) FetchNow(ctx context.Context, windowKey string) (Status, error) {
	if se := s.fetch(ctx, windowKey, true); se != nil && se.kind == "" {
		return Status{}, errf(ErrNotFound, "backend.space.configSync.notConfigured.body")
	}
	return s.Status(), nil
}

// fetch runs one fetch. It returns nil on success and the classified failure otherwise; a zero
// syncError means no repository is set.
func (s *Service) fetch(ctx context.Context, windowKey string, explicit bool) *syncError {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	s.mu.Lock()
	_ = s.load()
	set := s.set
	if set.Repo == nil {
		s.mu.Unlock()
		return &syncError{}
	}
	s.fetching = true
	s.mu.Unlock()
	s.emit()

	var sha string
	var se *syncError
	limit := fetchTimeout
	if explicit {
		limit = promptTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	if explicit && s.o.Auth != nil {
		if err := s.o.Auth(ctx, windowKey, func(env []string) error {
			sha, se = s.mirror.fetch(ctx, set.Repo.URL, set.branch(), env)
			return nil
		}); err != nil && se == nil {
			se = &syncError{kind: ProblemGit, message: firstLine(err.Error())}
		}
	} else {
		sha, se = s.mirror.fetch(ctx, set.Repo.URL, set.branch(), nil)
	}
	s.opMu.Lock()
	if se == nil {
		var tree *sharedTree
		if tree, se = s.mirror.readTree(ctx, sha); se == nil {
			s.mu.Lock()
			s.st.Fetched, s.st.FetchedAt, s.st.Problem = sha, s.o.Now().UTC().Format(time.RFC3339), nil
			s.st.Manifest = tree.modules
			s.sess = nil
			_ = saveState(s.o.Home, s.st)
			s.mu.Unlock()
			s.refreshLocked(ctx)
		}
	}
	if se != nil {
		s.setProblem(se.problem())
	}
	s.mu.Lock()
	s.fetching = false
	s.mu.Unlock()
	s.opMu.Unlock()
	s.emit()
	if explicit {
		if se == nil {
			s.rearm()
		}
	}
	return se
}

// arm schedules the next background fetch after d, replacing a pending one.
func (s *Service) arm(d time.Duration) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.closed {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, s.tick)
}

func (s *Service) disarm() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.bo = nil
}

// rearm restarts the schedule after a successful explicit fetch, clearing a failure stop and backoff.
func (s *Service) rearm() {
	s.mu.Lock()
	minutes, ok := s.set.minutes(), s.set.Repo != nil
	s.mu.Unlock()
	s.timerMu.Lock()
	s.bo = nil
	s.timerMu.Unlock()
	if ok && minutes > 0 {
		s.arm(time.Duration(minutes) * time.Minute)
	}
}

// tick is one background fetch. It reads the interval fresh, backs off on transient failures and
// stops on a permanent one until an explicit fetch succeeds.
func (s *Service) tick() {
	s.mu.Lock()
	_ = s.load()
	minutes, configured := s.set.minutes(), s.set.Repo != nil
	s.mu.Unlock()
	if !configured || minutes <= 0 {
		return
	}
	interval := time.Duration(minutes) * time.Minute
	se := s.fetch(s.ctx, "", false)
	s.timerMu.Lock()
	if s.closed {
		s.timerMu.Unlock()
		return
	}
	switch {
	case se == nil:
		s.bo = nil
		s.timerMu.Unlock()
		s.arm(interval)
	case se.transient:
		if s.bo == nil {
			s.bo = backoff.NewExponentialBackOff()
			s.bo.InitialInterval, s.bo.RandomizationFactor = interval, 0
			s.bo.MaxInterval, s.bo.MaxElapsedTime = max(interval, backoffCap), 0
			s.bo.Reset()
		}
		d := s.bo.NextBackOff()
		s.timerMu.Unlock()
		s.arm(d)
	default:
		s.timerMu.Unlock()
	}
}
