package enginecache

import (
	"crypto/sha1" //nolint:gosec // a cache key, not a security boundary — matches pages.ts's own sha1 use
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// DefaultPageBudgetBytes matches defaultSettings.cache.l2BudgetMb (pages.ts:7).
const DefaultPageBudgetBytes = 64 * 1024 * 1024

// ReadRequest is the cache-key half of data-ops.ts's ReadRequestWire: the fields normalizedRequest
// hashes, plus ConnectionID/Path for DropTarget/DropConnection's meta filtering. Path is already
// wire-encoded (model.EncodePath), the same as ReadRequestWire.path — encoding it is the caller's
// job, not the cache's.
type ReadRequest struct {
	ConnectionID string
	Path         string
	Projection   []string // nil = every column
	Filter       *string
	Sort         *model.SortSpec
	PageSize     int
	Cursor       model.PageCursor
}

func canonicalSort(sort_ *model.SortSpec) *string {
	if sort_ == nil {
		return nil
	}
	var s string
	if sort_.Kind == "text" {
		s = "text:" + sort_.Text
	} else {
		parts := make([]string, len(sort_.Terms))
		for i, t := range sort_.Terms {
			parts[i] = t.Column + ":" + t.Direction
		}
		s = "structured:" + strings.Join(parts, ",")
	}
	return &s
}

// normalizedRequest is pages.ts's normalizedRequest: sorting the projection and canonicalising the
// sort/filter is what turns "the user re-picked the same three columns in a different order" into a
// cache hit (D12). The cursor is included — each page/cursor pair is its own entry.
type normalizedRequest struct {
	ConnectionID string           `json:"connectionId"`
	Path         string           `json:"path"`
	Projection   []string         `json:"projection"`
	Filter       *string          `json:"filter"`
	Sort         *string          `json:"sort"`
	PageSize     int              `json:"pageSize"`
	Cursor       model.PageCursor `json:"cursor"`
}

// PageCacheKey is pages.ts's pageCacheKey: label is the canonical JSON, key is its sha1 hex.
func PageCacheKey(req ReadRequest) (key, label string) {
	var projection []string
	if req.Projection != nil {
		projection = append([]string{}, req.Projection...)
		sort.Strings(projection)
	}
	var filter *string
	if req.Filter != nil && trimmed(*req.Filter) != "" {
		f := trimmed(*req.Filter)
		filter = &f
	}
	norm := normalizedRequest{
		ConnectionID: req.ConnectionID,
		Path:         req.Path,
		Projection:   projection,
		Filter:       filter,
		Sort:         canonicalSort(req.Sort),
		PageSize:     req.PageSize,
		Cursor:       req.Cursor,
	}
	labelBytes, err := json.Marshal(norm)
	if err != nil {
		panic("enginecache: normalizedRequest must always marshal: " + err.Error())
	}
	label = string(labelBytes)
	sum := sha1.Sum(labelBytes) //nolint:gosec
	key = hex.EncodeToString(sum[:])
	return key, label
}

func trimmed(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// pageStore is the Go analogue of pages.ts's module-level state, held as a Cache field instead of a
// package-level singleton (§4.7's Go-specific note).
type pageStore struct {
	hits, misses int
	lru          *ByteLru[page.Page]
}

func newPageStore(warn func(string)) *pageStore {
	return &pageStore{lru: NewByteLru[page.Page](DefaultPageBudgetBytes, warn)}
}

func (s *pageStore) configureBudget(bytes int) { s.lru.SetBudget(bytes) }

func (s *pageStore) get(key string) (page.Page, bool) {
	p, ok := s.lru.Get(key)
	if ok {
		s.hits++
	} else {
		s.misses++
	}
	return p, ok
}

func (s *pageStore) put(key string, req ReadRequest, label string, p page.Page) {
	s.lru.Set(key, p, p.Size(), EntryMeta{ConnectionID: req.ConnectionID, Path: req.Path, Label: label})
}

func (s *pageStore) dropTarget(connectionID, path string) int {
	return s.lru.DeleteWhere(func(m EntryMeta) bool { return m.ConnectionID == connectionID && m.Path == path })
}

func (s *pageStore) dropConnection(connectionID string) int {
	return s.lru.DeleteWhere(func(m EntryMeta) bool { return m.ConnectionID == connectionID })
}

// clear mirrors pages.ts's clearPages: hits/misses reset too, since the hit rate reads as "since
// last clear" in the status bar and Settings → Cache.
func (s *pageStore) clear() {
	s.lru.Clear()
	s.hits, s.misses = 0, 0
}

type pageStats struct {
	bytes, budgetBytes, entries, hits, misses int
}

func (s *pageStore) stats() pageStats {
	return pageStats{
		bytes: s.lru.Bytes(), budgetBytes: s.lru.BudgetBytes(), entries: s.lru.Size(),
		hits: s.hits, misses: s.misses,
	}
}
