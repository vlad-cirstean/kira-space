// Package maskrules is the rule-management service: the layer between storage
// (repos.MaskRulesRepo/MaskKeysRepo) and this rule set's two consumers — internal/dbmcp's
// render path (through the MaskSetFor seam it consumer-declares) and internal/bridge's IPC surface
// for the connection dialog's Privacy tab and the grid's header menu.
package maskrules

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Service holds the two repos the rules span (the rule table, and the cipher-backed key column) and
// the connection files that own the rules, plus a small per-connection read cache: MaskSetFor is on
// dbmcp's own run_query hot path, and re-querying + re-folding the rule set on every call
// would cost a query per row rendered across a busy connection for no benefit, since a rule set
// changes only through a connection file, which calls Invalidate.
type Service struct {
	rules *repos.MaskRulesRepo
	keys  *repos.MaskKeysRepo
	files Files

	mu    sync.Mutex
	cache map[string]mask.Set // connectionID -> resolved Set; cleared for a connection on any write to it
	// gen counts invalidations per connection: MaskSetFor stores a freshly folded set only if no
	// invalidation landed since it started reading, so a write racing the read never leaves a stale
	// set cached.
	gen map[string]uint64

	afterRead func() // test seam: runs between MaskSetFor's query and its store
}

// Files is the connection files: where every rule is written.
type Files interface {
	RecordOf(id string) (connfiles.Record, bool)
	Patch(id string, sets ...configfs.NodeSet) error
}

// New constructs a Service over the two repos it owns and the files that hold the rules.
func New(rules *repos.MaskRulesRepo, keys *repos.MaskKeysRepo, files Files) *Service {
	return &Service{rules: rules, keys: keys, files: files, cache: map[string]mask.Set{}, gen: map[string]uint64{}}
}

// List returns every rule on connectionID, repo order (lower(table_name), lower(column_name)).
func (s *Service) List(connectionID string) ([]model.MaskRule, error) {
	return s.rules.ListForConnection(connectionID)
}

// Upsert validates one rule and writes it into the connection's file: the masks list is the source,
// the rule rows its projection. A number rule never correlates ("A number never carries a correlation
// tag" — a bucket is many-to-one, so tagging it would be dishonest; internal/mask.Apply enforces the
// same rule again, defensively). table_name defaults to "*" (any table) when left blank — the header
// menu's own common case, marking a column PII without the user needing to know which table it came
// from. A file with a problem is refused, never edited.
func (s *Service) Upsert(connectionID string, f model.MaskRuleFields) (model.MaskRule, error) {
	if !model.ValidMaskKind(string(f.Kind)) {
		return model.MaskRule{}, model.Invalid("maskrules: invalid kind %q", f.Kind)
	}
	if strings.TrimSpace(f.ColumnName) == "" {
		return model.MaskRule{}, model.Invalid("maskrules: column name is required")
	}
	if strings.TrimSpace(f.TableName) == "" {
		f.TableName = "*"
	}
	if f.Kind == model.MaskKindNumber {
		f.Correlate = false
	}
	rec, ok := s.files.RecordOf(connectionID)
	if !ok {
		return model.MaskRule{}, connfiles.IPCError(connfiles.ErrNotFound)
	}
	next := make([]connfiles.Mask, 0, len(rec.File.Masks)+1)
	replaced := false
	for _, m := range rec.File.Masks {
		if sameColumn(m.MaskFields(), f) {
			if !replaced {
				next = append(next, connfiles.MaskOf(f))
			}
			replaced = true
			continue
		}
		next = append(next, m)
	}
	if !replaced {
		next = append(next, connfiles.MaskOf(f))
	}
	if err := s.files.Patch(connectionID, configfs.NodeSet{Path: []string{"masks"}, Value: next}); err != nil {
		return model.MaskRule{}, connfiles.IPCError(err)
	}
	s.Invalidate(connectionID)
	rows, err := s.rules.ListForConnection(connectionID)
	if err != nil {
		return model.MaskRule{}, err
	}
	for _, r := range rows {
		if sameColumn(model.MaskRuleFields{TableName: r.TableName, ColumnName: r.ColumnName}, f) {
			return r, nil
		}
	}
	return model.MaskRule{}, fmt.Errorf("maskrules: rule %s.%s is not readable after the write", f.TableName, f.ColumnName)
}

func sameColumn(a, b model.MaskRuleFields) bool {
	return strings.EqualFold(a.TableName, b.TableName) && strings.EqualFold(strings.TrimSpace(a.ColumnName), strings.TrimSpace(b.ColumnName))
}

// Remove deletes one rule by id from its connection's file — idempotent (removing an already-gone
// rule is not an error, the grid header menu's own "Not PII" action can race a concurrent edit
// harmlessly).
func (s *Service) Remove(id string) error {
	existing, err := s.rules.Get(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	rec, ok := s.files.RecordOf(existing.ConnectionID)
	if !ok {
		return nil
	}
	gone := model.MaskRuleFields{TableName: existing.TableName, ColumnName: existing.ColumnName}
	next := make([]connfiles.Mask, 0, len(rec.File.Masks))
	for _, m := range rec.File.Masks {
		if !sameColumn(m.MaskFields(), gone) {
			next = append(next, m)
		}
	}
	if err := s.files.Patch(existing.ConnectionID, configfs.NodeSet{Path: []string{"masks"}, Value: next}); err != nil {
		return connfiles.IPCError(err)
	}
	s.Invalidate(existing.ConnectionID)
	return nil
}

// RegenerateKey mints a brand-new correlation key for connectionID, discarding whatever existed
// before (plan §2.5's own "manual and explicit... never automatic" rotation). Every masked
// correlation tag for this connection changes as a result.
func (s *Service) RegenerateKey(connectionID string) error {
	if err := s.keys.Regenerate(connectionID); err != nil {
		return err
	}
	s.Invalidate(connectionID)
	return nil
}

// Counts returns every connection id with at least one rule, mapped to its rule count — the
// Settings glance's own backend (§7.5).
func (s *Service) Counts() (map[string]int, error) {
	return s.rules.CountByConnection()
}

// MaskSetFor resolves connectionID's own folded rule set and Masker — what dbmcp's
// render path consumes through the MaskRules interface it consumer-declares. A connection with no
// rules returns a Set with an empty Rules map (never nil pointer confusion — dbmcp itself decides
// whether an empty Set behaves as "no masking configured").
func (s *Service) MaskSetFor(connectionID string) (mask.Set, error) {
	s.mu.Lock()
	if cached, ok := s.cache[connectionID]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	startGen := s.gen[connectionID]
	s.mu.Unlock()

	rows, err := s.rules.ListForConnection(connectionID)
	if err != nil {
		return mask.Set{}, err
	}
	if s.afterRead != nil {
		s.afterRead()
	}
	if len(rows) == 0 {
		set := mask.Set{Masker: mask.New(nil)}
		s.store(connectionID, startGen, set)
		return set, nil
	}

	// §4.2's conflict fold: two rules matching the same column name (under different table_name
	// values) collapse to the stricter one, mask.Stricter's own ranking.
	folded := map[string]mask.Rule{}
	for _, row := range rows {
		r := mask.Rule{Kind: mask.Kind(row.Kind), KeepHint: row.KeepHint, Correlate: row.Correlate}
		key := strings.ToLower(row.ColumnName)
		if existing, ok := folded[key]; ok {
			folded[key] = mask.Stricter(existing, r)
		} else {
			folded[key] = r
		}
	}

	// §2.5's own lazy creation: a key is minted only if some folded rule actually correlates —
	// a connection with every rule set to redact-only, or only `number` rules, never gets one.
	needsKey := false
	for _, r := range folded {
		if r.Correlate {
			needsKey = true
			break
		}
	}
	var key []byte
	if needsKey {
		k, err := s.keys.EnsureKey(connectionID)
		if err != nil {
			return mask.Set{}, err
		}
		key = k
	}

	set := mask.Set{Masker: mask.New(key), Rules: folded}
	s.store(connectionID, startGen, set)
	return set, nil
}

// CorrelationKeyHex returns connectionID's own correlation key, hex-encoded, minting one first
// (the lazy creation) if the connection has at least one correlating rule and none exists yet.
// "" means the connection needs no key at all. It is added because the design (the grid preview
// computing tags locally via Web Crypto, to match MCP's output byte-for-byte) is otherwise
// impossible: the renderer has no other way to obtain the raw key. The renderer is not the
// adversary the MCP path defends against — exposing the key to it mirrors how a connection
// password already reaches the renderer for editing (connections.Reveal) — but this is still the
// one seam that ever sends the key outside this process, so it must never be reachable from
// anything but the grid preview's own local tag computation.
//
// Folding is deliberately NOT applied before checking "does anything correlate": a raw rule with
// correlate=true that later loses a fold to a stricter, non-correlating rule on the same
// column still triggers a mint here. That mints a key that ends up unused for that particular
// column — harmless (an unused key sitting in the store, never surfaced anywhere) rather than a
// correctness bug, and far simpler than re-deriving the fold just to decide whether to mint.
func (s *Service) CorrelationKeyHex(connectionID string) (string, error) {
	rows, err := s.rules.ListForConnection(connectionID)
	if err != nil {
		return "", err
	}
	needsKey := false
	for _, r := range rows {
		if r.Correlate {
			needsKey = true
			break
		}
	}
	if !needsKey {
		return "", nil
	}
	key, err := s.keys.EnsureKey(connectionID)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func (s *Service) store(connectionID string, startGen uint64, set mask.Set) {
	s.mu.Lock()
	if s.gen[connectionID] == startGen {
		s.cache[connectionID] = set
	}
	s.mu.Unlock()
}

// Invalidate drops connectionID's cached rule set; the connection files loader calls it when a file
// changes the masks.
func (s *Service) Invalidate(connectionID string) {
	s.mu.Lock()
	delete(s.cache, connectionID)
	s.gen[connectionID]++
	s.mu.Unlock()
}
