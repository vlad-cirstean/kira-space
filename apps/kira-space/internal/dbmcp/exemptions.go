package dbmcp

import (
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/ids"
)

// maxExemptions caps the store: a refusal mints a record, and an agent can loop.
const maxExemptions = 256

type exemptionRecord struct {
	connectionID, mode, path, sql string
	hits                          []LimitHit
	expiresAt                     time.Time
}

// exemptionStore holds the ids minted by limit refusals until the agent presents one. In memory
// only: a restart drops every id.
type exemptionStore struct {
	mu   sync.Mutex
	now  func() time.Time
	byID map[string]exemptionRecord
}

func newExemptionStore(now func() time.Time) *exemptionStore {
	return &exemptionStore{now: now, byID: map[string]exemptionRecord{}}
}

// mint stores rec under a fresh id valid for ApprovalTimeout. Purges expired records, then evicts the
// oldest past maxExemptions.
func (s *exemptionStore) mint(rec exemptionRecord) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, r := range s.byID {
		if !now.Before(r.expiresAt) {
			delete(s.byID, id)
		}
	}
	for len(s.byID) >= maxExemptions {
		oldest, first := "", true
		for id, r := range s.byID {
			if first || r.expiresAt.Before(s.byID[oldest].expiresAt) {
				oldest, first = id, false
			}
		}
		delete(s.byID, oldest)
	}
	rec.expiresAt = now.Add(ApprovalTimeout)
	id := ids.New(ids.Exemption)
	s.byID[id] = rec
	return id
}

// claim removes id and returns its record; ok is false when the id is unknown, used or expired.
func (s *exemptionStore) claim(id string) (exemptionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		return exemptionRecord{}, false
	}
	delete(s.byID, id)
	if !s.now().Before(rec.expiresAt) {
		return exemptionRecord{}, false
	}
	return rec, true
}
