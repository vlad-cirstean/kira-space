// Package approvals is the one queue of agent approvals an app holds. Owners block their own
// callers and keep their answer semantics; the queue holds view items and routes answers.
package approvals

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

// Kinds known to the apps.
const (
	KindDbQuery       = "db.query"
	KindDbResponse    = "db.response"
	KindDbExemption   = "db.exemption"
	KindKubeRead      = "kube.read"
	KindProofReview   = "proof.review"
	KindBranchRequest = "branch.request"
	KindScriptRun     = "script.run"
	KindAPIRequest    = "api.request"
)

// Item is one pending approval as the window renders it. Payload is per kind, validated by the client.
type Item struct {
	// ID is "<kind>:<ref>", stable for the item's life.
	ID string `json:"id"`
	// Kind is the subject key the client registry maps to renderers.
	Kind string `json:"kind"`
	// CreatedAt and ExpiresAt are ms; ExpiresAt 0 means no expiry.
	CreatedAt int64     `json:"createdAt"`
	ExpiresAt int64     `json:"expiresAt"`
	Origin    Origin    `json:"origin"`
	Target    Target    `json:"target"`
	Operation Operation `json:"operation"`
	// Reason is permission, heavy, preview, review, request or exemption.
	Reason  string          `json:"reason"`
	Payload json.RawMessage `json:"payload"`
	// Title is the prompt router title; the view never reads it.
	Title string `json:"-"`
}

// Origin says who asked. Unset fields are "".
type Origin struct {
	Client string `json:"client"`
	Cwd    string `json:"cwd"`
	Task   string `json:"task"`
	Step   string `json:"step"`
	// Source is session, run, automation or "".
	Source string `json:"source"`
}

// Target is what the request acts on.
type Target struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Color    string `json:"color"`
	ReadOnly bool   `json:"readOnly"`
	Location string `json:"location"`
}

// Operation classifies the request. Class is read, write, ddl, unknown, create, rename, review, run or
// request; Tone is neutral, caution or danger.
type Operation struct {
	Class string `json:"class"`
	Tone  string `json:"tone"`
}

// Answer is one decision. Options is per kind.
type Answer struct {
	Approve bool            `json:"approve"`
	Options json.RawMessage `json:"options,omitempty"`
}

// Owner answers its own items. It returns an ipcerr NotFound when the item already resolved.
type Owner interface {
	Answer(ref string, a Answer) error
}

// Result is the outcome of answering one id.
type Result struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

// Queue is safe for concurrent use.
type Queue struct {
	prompts prompts.Sink

	mu     sync.Mutex
	items  map[string]Item
	owners map[string]Owner
	em     notify.OrderedEmitter[[]Item]
}

// New returns a queue; sink may be nil, which routes no prompts.
func New(sink prompts.Sink) *Queue {
	return &Queue{prompts: sink, items: map[string]Item{}, owners: map[string]Owner{}}
}

// Register sets the owner that answers items of kind.
func (q *Queue) Register(kind string, o Owner) {
	q.mu.Lock()
	q.owners[kind] = o
	q.mu.Unlock()
}

// OnChange subscribes to the full item list after every change.
func (q *Queue) OnChange(fn func([]Item)) (unsubscribe func()) { return q.em.Subscribe(fn) }

func (q *Queue) listLocked() []Item {
	out := make([]Item, 0, len(q.items))
	for _, it := range q.items {
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b Item) int {
		if a.CreatedAt != b.CreatedAt {
			return int(a.CreatedAt - b.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// List is every pending item, oldest first.
func (q *Queue) List() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.listLocked()
}

// Put inserts it or replaces the item with the same id. A new id opens its router prompt.
func (q *Queue) Put(it Item) {
	q.mu.Lock()
	_, existed := q.items[it.ID]
	q.items[it.ID] = it
	if !existed && q.prompts != nil {
		// Under mu so a fast answer cannot Close before this Open.
		q.prompts.Open(prompts.Prompt{
			ID: promptID(it.ID), Kind: prompts.KindApproval, Ref: it.ID, Title: it.Title, CreatedAt: it.CreatedAt,
		})
	}
	q.publishLocked()
}

// Remove drops the item and closes its router prompt; an absent id is a no-op.
func (q *Queue) Remove(id string) {
	q.mu.Lock()
	if _, ok := q.items[id]; !ok {
		q.mu.Unlock()
		return
	}
	delete(q.items, id)
	if q.prompts != nil {
		q.prompts.Close(promptID(id))
	}
	q.publishLocked()
}

// publishLocked emits the list; it releases mu.
func (q *Queue) publishLocked() {
	seq, list := q.em.NextSeq(), q.listLocked()
	q.mu.Unlock()
	q.em.Emit(seq, list)
}

func promptID(id string) string { return prompts.ID(prompts.KindApproval, id) }

// Answer answers each id through its owner. A per-id failure is reported and never aborts the rest.
func (q *Queue) Answer(ids []string, approve bool, options map[string]json.RawMessage) []Result {
	out := make([]Result, 0, len(ids))
	for _, id := range ids {
		out = append(out, q.answerOne(id, Answer{Approve: approve, Options: options[id]}))
	}
	return out
}

func (q *Queue) answerOne(id string, a Answer) Result {
	q.mu.Lock()
	it, ok := q.items[id]
	owner := q.owners[it.Kind]
	q.mu.Unlock()
	if !ok || owner == nil {
		e := ipcerr.NotFoundNamed("approval")
		return Result{ID: id, Code: e.Code, Error: e.Message}
	}
	err := owner.Answer(strings.TrimPrefix(id, it.Kind+":"), a)
	if err == nil {
		return Result{ID: id}
	}
	e, _ := ipcerr.Wrap(err).(*ipcerr.Error)
	return Result{ID: id, Code: e.Code, Error: e.Message}
}
