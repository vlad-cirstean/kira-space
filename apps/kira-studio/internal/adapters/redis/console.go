package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
)

// isHexDigit/hexDigitVal back tokenize's own \xHH escape (F12) — redis-cli's sdssplitargs accepts
// either case.
func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func hexDigitVal(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	default:
		return int(r-'A') + 10
	}
}

// redisDoubleQuoteEscapes is redis-cli's own sdssplitargs table for a `\X` sequence inside a
// double-quoted console token (F12) — anything not listed here (including `\"` and `\\`) falls
// through to the escaped character itself, matching sdssplitargs's own `default: c = *p`.
var redisDoubleQuoteEscapes = map[rune]rune{
	'n': '\n',
	'r': '\r',
	't': '\t',
	'b': '\b',
	'a': '\a',
}

// scanQuotedToken consumes one quoted token starting at runes[i] (runes[i] is the opening quote),
// applying F12's own escaping rules, and returns the decoded token text plus the index just past
// the closing quote. Split out of tokenize to keep gocognit happy — the quoting rules themselves
// are exactly redis-cli's own sdssplitargs: double quotes recognize \n/\r/\t/\b/\a, \xHH, and any
// other `\X` as a literal X; single quotes recognize only `\'`, everything else literal (including a
// bare `\`); and a closing quote must be followed by whitespace or end of input, never trailing
// content glued onto the same token.
func scanQuotedToken(runes []rune, i int) (string, int, error) {
	n := len(runes)
	quote := runes[i]
	i++
	var out strings.Builder
	for i < n && runes[i] != quote {
		if quote == '"' && runes[i] == '\\' && i+1 < n {
			if runes[i+1] == 'x' && i+3 < n && isHexDigit(runes[i+2]) && isHexDigit(runes[i+3]) {
				out.WriteByte(byte(hexDigitVal(runes[i+2])*16 + hexDigitVal(runes[i+3])))
				i += 4
				continue
			}
			esc := runes[i+1]
			if r, ok := redisDoubleQuoteEscapes[esc]; ok {
				out.WriteRune(r)
			} else {
				out.WriteRune(esc)
			}
			i += 2
			continue
		}
		if quote == '\'' && runes[i] == '\\' && i+1 < n && runes[i+1] == '\'' {
			out.WriteRune('\'')
			i += 2
			continue
		}
		out.WriteRune(runes[i])
		i++
	}
	if i >= n {
		return "", i, adapters.T(adapters.CodeQuery, "backend.studio.adapter.quotedStringUnterminated.body", nil, nil)
	}
	i++ // closing quote
	if i < n && !unicode.IsSpace(runes[i]) {
		return "", i, adapters.T(adapters.CodeQuery, "backend.studio.adapter.closingQuoteFollow.body", nil, nil)
	}
	return out.String(), i, nil
}

// scanBareToken consumes one unquoted, whitespace-delimited token starting at runes[i].
func scanBareToken(runes []rune, i int) (string, int) {
	n := len(runes)
	var out strings.Builder
	for i < n && !unicode.IsSpace(runes[i]) {
		out.WriteRune(runes[i])
		i++
	}
	return out.String(), i
}

// §8.14: "for non-SQL engines the console takes that engine's native command form" — real Redis
// CLI syntax is flat whitespace-separated tokens with optional single/double quoting, not a JSON
// DSL like Mongo's shell (P9's D11, C12).
func tokenize(line string) ([]string, error) {
	runes := []rune(line)
	n := len(runes)
	var tokens []string
	i := 0
	for i < n {
		for i < n && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= n {
			break
		}
		var (
			token string
			err   error
		)
		if runes[i] == '"' || runes[i] == '\'' {
			token, i, err = scanQuotedToken(runes, i)
			if err != nil {
				return nil, err
			}
		} else {
			token, i = scanBareToken(runes, i)
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func formatReplyItem(value any) string {
	if value == nil {
		return "(nil)"
	}
	switch v := value.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// hashReadCommands is resultToPage's own limited command-aware set: these three
// reads carry a genuine per-value field name — HGET/HMGET's from the command's own arguments (the
// caller-supplied field names), HGETALL's from the reply itself (server-returned field/value
// pairs) — so their page is built with FieldsAreColumns=true and a real Field, letting a mask rule
// on that hash's field name match. Every other command keeps the generic formatting below
// with FieldsAreColumns=false: an array index or a bare command name is never a genuine field
// identifier a rule could legitimately match, so dbmcp's renderKeyValuePage refuses to mask such a
// page rather than silently pass a real value through under a Field nothing could ever match.
var hashReadCommands = map[string]bool{"HGET": true, "HMGET": true, "HGETALL": true}

// resultToPage is console.ts's own: any RESP reply is formatted generically — no
// per-command result shape, unlike Mongo's console — except for hashReadCommands above. A scalar's
// field name is the upper-cased command; an array's are the indices. The page stops at limit and
// reports Truncated when another entry existed.
func resultToPage(command string, args []string, reply any, limit page.ResultCap) page.KeyValuePage {
	upper := strings.ToUpper(command)
	if hashReadCommands[upper] {
		if pg, ok := hashReadPage(upper, args, reply, limit); ok {
			return pg
		}
		// Reply didn't match the shape this command is supposed to return (a protocol surprise, or
		// a mock in a test) — fall through to the generic, columnless rendering below rather than
		// guess at a Field that might be wrong.
	}
	builder := page.NewKeyValuePageBuilder("string", nil, nil, false)
	builder.SetFieldsAreColumns(false)
	truncated := false
	if arr, ok := reply.([]any); ok {
		for i, item := range arr {
			if limit.Reached(builder.RowCount(), builder.Bytes()) {
				truncated = true
				break
			}
			builder.Push(strconv.Itoa(i), formatReplyItem(item))
		}
	} else {
		builder.Push(strings.ToUpper(command), formatReplyItem(reply))
	}
	pageSize := builder.RowCount()
	if _, isArray := reply.([]any); !isArray {
		pageSize = 1
	}
	return builder.Finish(page.CappedPosition(pageSize, truncated))
}

// pushCapped pushes count entries, stopping early at limit; it reports whether an entry was left out.
func pushCapped(builder *page.KeyValuePageBuilder, limit page.ResultCap, count int, entry func(i int) (field, value string)) (truncated bool) {
	for i := 0; i < count; i++ {
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			return true
		}
		field, value := entry(i)
		builder.Push(field, value)
	}
	return false
}

// hashReadPage builds hashReadCommands' own real-field-name page, ok=false when reply isn't the
// shape that command is documented to return.
func hashReadPage(upper string, args []string, reply any, limit page.ResultCap) (page.KeyValuePage, bool) {
	switch upper {
	case "HGET":
		if len(args) == 0 {
			return page.KeyValuePage{}, false
		}
		builder := page.NewKeyValuePageBuilder("hash", nil, nil, true)
		builder.Push(args[0], formatReplyItem(reply))
		return builder.Finish(page.UnpagedPosition(1)), true
	case "HMGET":
		arr, ok := reply.([]any)
		if !ok {
			return page.KeyValuePage{}, false
		}
		builder := page.NewKeyValuePageBuilder("hash", nil, nil, false)
		truncated := pushCapped(builder, limit, len(arr), func(i int) (string, string) {
			field := strconv.Itoa(i)
			if i < len(args) {
				field = args[i]
			}
			return field, formatReplyItem(arr[i])
		})
		return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), true
	case "HGETALL":
		builder := page.NewKeyValuePageBuilder("hash", nil, nil, false)
		switch v := reply.(type) {
		case []any: // RESP2: a flat field, value, field, value, ... array
			if len(v)%2 != 0 {
				return page.KeyValuePage{}, false
			}
			truncated := pushCapped(builder, limit, len(v)/2, func(i int) (string, string) {
				return formatReplyItem(v[2*i]), formatReplyItem(v[2*i+1])
			})
			return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), true
		case map[string]any: // RESP3: a real map reply
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys) // deterministic order — the wire map has none
			truncated := pushCapped(builder, limit, len(keys), func(i int) (string, string) {
				return keys[i], formatReplyItem(v[keys[i]])
			})
			return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), true
		default:
			return page.KeyValuePage{}, false
		}
	default:
		return page.KeyValuePage{}, false
	}
}

// ClassifiesRemotely satisfies adapters.RemoteClassifier: a COMMAND table cache miss queries the
// server.
func (a *Adapter) ClassifiesRemotely() bool { return true }

// ClassifyStatement satisfies adapters.StatementClassifier over this package's own
// tokenize/isReadOnlyCommand — Redis has no DDL, and one run_query call is one command (tokenize
// treats a newline as ordinary whitespace, so extra lines become arguments, never a second
// command). The COMMAND table is server-wide, so the db index does not matter for classification.
func (a *Adapter) ClassifyStatement(ctx context.Context, statement string) (adapters.OpClass, error) {
	set, err := a.requireSet()
	if err != nil {
		return adapters.ClassUnknown, err
	}
	tokens, err := tokenize(statement)
	if err != nil {
		return adapters.ClassUnknown, err
	}
	if len(tokens) == 0 {
		return adapters.ClassUnknown, nil
	}
	conn, release, err := set.get(ctx, a.state.Load().defaultDbIndex)
	if err != nil {
		return adapters.ClassUnknown, err
	}
	defer release()
	if set.isReadOnlyCommand(ctx, conn, tokens[0], tokens[1:]) {
		return adapters.ClassRead, nil
	}
	return adapters.ClassWrite, nil
}

// deniedConsoleCommands is the denylist: every command go-redis's pooled *redis.Client cannot
// safely run through a raw conn.Do — go-redis does not track connection/session state a raw command
// changes, so any of these, issued on the shared per-db-index pooled connection, would silently
// corrupt every later grid/tree/mutate/console call sharing it. SELECT repoints the connection at a
// different db index; MULTI/EXEC/DISCARD/WATCH/UNWATCH leave a transaction half-open or make later
// commands reply QUEUED; SUBSCRIBE and its siblings leave the connection stuck in push mode;
// MONITOR never stops streaming; HELLO/AUTH/RESET change the negotiated protocol or effective user;
// QUIT closes the connection out from under the pool.
var deniedConsoleCommands = map[string]bool{
	"SELECT":       true,
	"MULTI":        true,
	"EXEC":         true,
	"DISCARD":      true,
	"WATCH":        true,
	"UNWATCH":      true,
	"SUBSCRIBE":    true,
	"PSUBSCRIBE":   true,
	"SSUBSCRIBE":   true,
	"UNSUBSCRIBE":  true,
	"PUNSUBSCRIBE": true,
	"SUNSUBSCRIBE": true,
	"MONITOR":      true,
	"HELLO":        true,
	"AUTH":         true,
	"RESET":        true,
	"QUIT":         true,
}

// rejectConnectionStateCommand is the guard, checked before every console command reaches
// conn.Do. CLIENT REPLY OFF/SKIP desyncs the reply stream for every later command on this same
// pooled connection the exact same way the commands in deniedConsoleCommands do — matched as the
// two-token command CLIENT REPLY specifically, so an ordinary CLIENT subcommand (GETNAME, INFO,
// LIST, and friends) stays usable.
func rejectConnectionStateCommand(command string, args []string) error {
	upper := strings.ToUpper(command)
	if upper == "CLIENT" && len(args) > 0 && strings.EqualFold(args[0], "REPLY") {
		return adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisClientReply.body", map[string]string{"command": "CLIENT REPLY"}, nil)
	}
	if !deniedConsoleCommands[upper] {
		return nil
	}
	if upper == "SELECT" {
		return adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisSelect.body", map[string]string{"command": "SELECT"}, nil)
	}
	return adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisSharedState.body", map[string]string{"command": upper}, nil)
}

// consoleCommand is one already-parsed, already-validated console line (F11): execute below parses
// and validates every statement in the batch before running any of them.
type consoleCommand struct {
	line        string
	command     string
	args        []string
	readTimeout time.Duration
}

const (
	maxBlockSeconds = 600
	blockMargin     = 5 * time.Second
)

// consoleReadTimeout bounds the wait for any non-blocking console reply; a var so tests can
// shorten it.
var consoleReadTimeout = 60 * time.Second

// blockingTimeout reports the server-side wait a blocking command asks for. ok is false for every
// other command; an unparsable argument also reports false and is left to the server to reject.
func blockingTimeout(command string, args []string) (d time.Duration, ok bool) {
	var raw string
	unit := time.Second
	switch strings.ToUpper(command) {
	case "BLPOP", "BRPOP", "BRPOPLPUSH", "BZPOPMIN", "BZPOPMAX", "BLMOVE":
		if len(args) == 0 {
			return 0, false
		}
		raw = args[len(args)-1]
	case "BLMPOP", "BZMPOP":
		if len(args) == 0 {
			return 0, false
		}
		raw = args[0]
	case "WAIT", "WAITAOF":
		i := 1
		if strings.EqualFold(command, "WAITAOF") {
			i = 2
		}
		if len(args) <= i {
			return 0, false
		}
		raw, unit = args[i], time.Millisecond
	case "XREAD", "XREADGROUP":
		for i, a := range args {
			if strings.EqualFold(a, "BLOCK") && i+1 < len(args) {
				raw, unit = args[i+1], time.Millisecond
				break
			}
		}
		if raw == "" {
			return 0, false
		}
	default:
		return 0, false
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return time.Duration(n * float64(unit)), true
}

// consoleReadTimeoutFor is how long to wait for command's reply. A blocking command waits its own
// timeout plus a margin; an unbounded or very long block is refused, since nothing but the read
// deadline would end it once Stop abandons the call.
func consoleReadTimeoutFor(command string, args []string) (time.Duration, error) {
	d, blocking := blockingTimeout(command, args)
	if !blocking {
		return consoleReadTimeout, nil
	}
	if d == 0 || d > maxBlockSeconds*time.Second {
		return 0, adapters.T(adapters.CodeQuery, "backend.studio.adapter.redisBlockTimeout.body", map[string]string{"command": strings.ToUpper(command), "seconds": fmt.Sprint(maxBlockSeconds)}, nil)
	}
	return d + blockMargin, nil
}

// runConsoleCommand sends one command on a client that never re-sends it (a retried write would
// run twice) and reads the reply for at most timeout. It races Stop: the call finishes on its own
// connection within the read deadline, but the caller returns at once.
func runConsoleCommand(ctx context.Context, conn *goredis.Client, argv []any, timeout time.Duration) (any, error) {
	cc := conn.WithTimeout(timeout)
	cc.Options().MaxRetries = 0
	return adapters.RunWithAbortRace(ctx, func() {}, func(qctx context.Context) (any, error) {
		reply, err := cc.Do(qctx, argv...).Result()
		if err != nil {
			return nil, mapError(err)
		}
		return reply, nil
	})
}

// execute is console.ts's execute — one op-log row for the whole batch.
// A read-only connection is gated per-command via Redis's own COMMAND table (isReadOnlyCommand),
// not a blanket refusal — SPEC.md's read-only contract disables "anything but a read", and a flat
// console-execution ban would take reads away too.
//
// Every statement is tokenized, denylist-checked, and read-only-checked up front — before any
// of them runs against the connection. The previous shape parsed statement N only after 1..N-1 had
// already executed, so a typo further down a batch discarded the already-committed earlier writes
// with no rollback path; re-running after fixing the typo then double-applied them.
func execute(ctx context.Context, set *dbConnectionSet, dbIndex int, readOnly bool, op *adapters.OpCtx, statements []string, limit page.ResultCap) ([]page.Page, error) {
	var lines []string
	for _, s := range statements {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}
	op.SetCommand(strings.Join(lines, "\n"))

	conn, release, err := set.get(ctx, dbIndex)
	if err != nil {
		return nil, err
	}
	defer release()

	commands := make([]consoleCommand, 0, len(lines))
	for _, line := range lines {
		tokens, err := tokenize(line)
		if err != nil {
			return nil, err
		}
		if len(tokens) == 0 {
			continue
		}
		command, args := tokens[0], tokens[1:]
		if err := rejectConnectionStateCommand(command, args); err != nil {
			return nil, err
		}
		if readOnly && !set.isReadOnlyCommand(ctx, conn, command, args) {
			return nil, adapters.AssertWritable(true)
		}
		readTimeout, err := consoleReadTimeoutFor(command, args)
		if err != nil {
			return nil, err
		}
		commands = append(commands, consoleCommand{line: line, command: command, args: args, readTimeout: readTimeout})
	}
	if len(commands) == 0 {
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}

	pages := make([]page.Page, 0, len(commands))
	for _, c := range commands {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		argv := make([]any, 0, len(c.args)+1)
		argv = append(argv, c.command)
		for _, a := range c.args {
			argv = append(argv, a)
		}
		reply, err := runBoundedCommand(ctx, conn, c, argv, limit)
		if err != nil {
			return nil, err
		}
		p := resultToPage(c.command, c.args, reply, limit)
		pages = append(pages, p)
		limit = adapters.RemainingCap(limit, p.Size())
	}
	return pages, nil
}
