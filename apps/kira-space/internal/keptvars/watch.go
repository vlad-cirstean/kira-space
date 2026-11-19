package keptvars

import "sync"

type watcher struct {
	ids  []string
	fn   func(Removal)
	once sync.Once
}

func (w *watcher) fire(r Removal) { w.once.Do(func() { w.fn(r) }) }

// OnRemove calls fn once, with the first removal among ids. An id already absent fires at once with the
// reason known for it, else expired. stop deregisters; after it the watcher never fires. fn runs outside
// the store's lock, so it may call back into the store.
func (s *Store) OnRemove(ids []string, fn func(Removal)) (stop func()) {
	w := &watcher{ids: ids, fn: fn}
	var b batch
	s.mu.Lock()
	s.dueLocked(&b)
	var absent *Removal
	for _, id := range ids {
		if it := s.byID[id]; it != nil {
			continue
		}
		why, ok := s.gone[id]
		if !ok {
			why = ReasonExpired
		}
		absent = &Removal{ID: id, Reason: why}
		break
	}
	if absent == nil {
		for _, id := range ids {
			s.watchers[id] = append(s.watchers[id], w)
		}
	}
	rev := s.commitLocked(&b)
	s.mu.Unlock()
	s.finish(&b, rev)
	if absent != nil {
		w.fire(*absent)
		return func() {}
	}
	return func() {
		w.once.Do(func() {})
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range w.ids {
			list := s.watchers[id]
			for i, x := range list {
				if x == w {
					s.watchers[id] = append(list[:i:i], list[i+1:]...)
					break
				}
			}
			if len(s.watchers[id]) == 0 {
				delete(s.watchers, id)
			}
		}
	}
}
