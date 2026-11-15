package agentmcp

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Item statuses of a state reply. pending: the user must answer. waiting: Kira or a queue is on it.
const (
	StatusDone    = "done"
	StatusPending = "pending"
	StatusWaiting = "waiting"
	StatusDenied  = "denied"
	StatusFailed  = "failed"
	StatusInvalid = "invalid"
)

var statusOrder = []string{StatusDone, StatusPending, StatusWaiting, StatusDenied, StatusFailed, StatusInvalid}

// Item is one change of a state call and how it stands. Fix, set on an invalid item, is one example call.
type Item struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// Reply is the shared reply shape of the state tool: a summary line, one item per change, what to do
// next, and notices that waited for the caller.
type Reply struct {
	Summary string   `json:"summary"`
	Items   []Item   `json:"items"`
	Next    string   `json:"next"`
	Notices []string `json:"notices"`
}

// newReply counts items by status into the summary line.
func newReply(items []Item, next string, notices []string) Reply {
	count := map[string]int{}
	for _, it := range items {
		count[it.Status]++
	}
	var parts []string
	for _, st := range statusOrder {
		if n := count[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, st))
		}
	}
	if notices == nil {
		notices = []string{}
	}
	return Reply{Summary: strings.Join(parts, ", "), Items: items, Next: next, Notices: notices}
}

// text is the summary line, one line per item, then next and the notices.
func (r Reply) text() string {
	var sb strings.Builder
	sb.WriteString(r.Summary)
	for _, it := range r.Items {
		fmt.Fprintf(&sb, "\n%s %s: %s", it.ID, it.Status, it.Message)
		if it.Fix != "" {
			sb.WriteString(" Fix: " + it.Fix)
		}
	}
	if r.Next != "" {
		sb.WriteString("\nnext: " + r.Next)
	}
	for _, n := range r.Notices {
		sb.WriteString("\nnotice: " + n)
	}
	return sb.String()
}

// result is the reply as a tool result: text for the agent, the JSON as structured content.
func (r Reply) result(extra ...mcp.Content) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content:           append([]mcp.Content{&mcp.TextContent{Text: r.text()}}, extra...),
		StructuredContent: r,
	}
}

func oneItem(it Item, next string, notices []string) Reply {
	return newReply([]Item{it}, next, notices)
}
