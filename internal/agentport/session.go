package agentport

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// previewTTL is how long a preview stays applicable.
const previewTTL = 10 * time.Minute

// sessions keeps previews server side: decoded values with their literals never cross the bridge.
// A token is single use.
type sessions struct {
	mu  sync.Mutex
	m   map[string]*session
	now func() time.Time
}

type session struct {
	expires time.Time
	imp     *importSession
	exp     *exportSession
}

func newSessions(now func() time.Time) *sessions {
	return &sessions{m: map[string]*session{}, now: now}
}

func (s *sessions) put(sess *session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for t, o := range s.m {
		if s.now().After(o.expires) {
			delete(s.m, t)
		}
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	token := hex.EncodeToString(b[:])
	sess.expires = s.now().Add(previewTTL)
	s.m[token] = sess
	return token
}

// take removes and returns the live session of token.
func (s *sessions) take(token string) (*session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[token]
	delete(s.m, token)
	if !ok || s.now().After(sess.expires) {
		return nil, false
	}
	return sess, true
}

func (s *sessions) drop(token string) {
	s.mu.Lock()
	delete(s.m, token)
	s.mu.Unlock()
}

func (s *sessions) clear() {
	s.mu.Lock()
	s.m = map[string]*session{}
	s.mu.Unlock()
}
