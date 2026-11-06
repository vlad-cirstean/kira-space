package redis

import (
	"context"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// go-redis reads one RESP reply whole, so a console command that replies with a whole collection
// (KEYS *, HGETALL, LRANGE 0 -1) would buffer all of it before the page cap could apply. For the
// commands below the console runs an equivalent that fetches at most one entry past the cap and
// shapes the reply back to what the original command returns. Every other command runs as typed.

// scanBatch is the COUNT hint sent with every SCAN-family round.
var scanBatch = strconv.Itoa(scanCount)

// boundPlan is how to run one bounded command: args replaces the command's own arguments (same
// command), or scan drives a SCAN-family loop. A zero plan means run as typed.
type boundPlan struct {
	args []string
	scan *scanPlan
}

// scanPlan: argv per round is head, cursor, extra. unit is the reply's entries-per-element
// (2 for HSCAN's field/value pairs); emit picks which of a unit's items the original reply holds.
type scanPlan struct {
	head, extra []string
	unit        int
	emit        []int
}

var (
	indexRangeCommands = map[string]string{"LRANGE": "LLEN", "ZRANGE": "ZCARD", "ZREVRANGE": "ZCARD"}
	limitRangeCommands = map[string]bool{"ZRANGEBYSCORE": true, "ZREVRANGEBYSCORE": true, "ZRANGEBYLEX": true, "ZREVRANGEBYLEX": true}
)

// planBounded decides how to bound command. want is the most entries to fetch (cap plus one, so
// the page can tell the result was cut). length answers LLEN/ZCARD for a negative-index range.
func planBounded(command string, args []string, want int, length func(cmd, key string) (int64, error)) (boundPlan, error) {
	upper := strings.ToUpper(command)
	switch upper {
	case "KEYS":
		if len(args) == 1 {
			return boundPlan{scan: &scanPlan{head: []string{"SCAN"}, extra: []string{"MATCH", args[0], "COUNT", scanBatch}, unit: 1, emit: []int{0}}}, nil
		}
	case "SMEMBERS":
		if len(args) == 1 {
			return boundPlan{scan: &scanPlan{head: []string{"SSCAN", args[0]}, extra: []string{"COUNT", scanBatch}, unit: 1, emit: []int{0}}}, nil
		}
	case "HGETALL", "HKEYS", "HVALS":
		if len(args) == 1 {
			emit := map[string][]int{"HGETALL": {0, 1}, "HKEYS": {0}, "HVALS": {1}}[upper]
			return boundPlan{scan: &scanPlan{head: []string{"HSCAN", args[0]}, extra: []string{"COUNT", scanBatch}, unit: 2, emit: emit}}, nil
		}
	case "XRANGE", "XREVRANGE":
		return planCount(args, want), nil
	}
	if lengthCmd, ok := indexRangeCommands[upper]; ok && !hasRangeMode(upper, args) {
		return planIndexRange(lengthCmd, args, want, length)
	}
	if limitRangeCommands[upper] || (upper == "ZRANGE" && hasRangeMode(upper, args)) {
		return planLimit(args, want), nil
	}
	return boundPlan{}, nil
}

// hasRangeMode reports whether a ZRANGE carries BYSCORE or BYLEX (the LIMIT form, not indexes).
func hasRangeMode(upper string, args []string) bool {
	if upper != "ZRANGE" {
		return false
	}
	for _, a := range args[min(3, len(args)):] {
		if u := strings.ToUpper(a); u == "BYSCORE" || u == "BYLEX" {
			return true
		}
	}
	return false
}

// planIndexRange clamps `key start stop ...` so at most want entries come back. Negative indices
// resolve against the collection length; a range already within want runs as typed.
func planIndexRange(lengthCmd string, args []string, want int, length func(cmd, key string) (int64, error)) (boundPlan, error) {
	if len(args) < 3 {
		return boundPlan{}, nil
	}
	start, err1 := strconv.ParseInt(args[1], 10, 64)
	stop, err2 := strconv.ParseInt(args[2], 10, 64)
	if err1 != nil || err2 != nil {
		return boundPlan{}, nil // the server reports the bad index
	}
	if start < 0 || stop < 0 {
		n, err := length(lengthCmd, args[0])
		if err != nil {
			return boundPlan{}, err
		}
		if start < 0 {
			start = max(start+n, 0)
		}
		if stop < 0 {
			stop += n
		}
	}
	if stop < start || stop-start < int64(want) {
		return boundPlan{}, nil
	}
	out := append([]string(nil), args...)
	out[1], out[2] = strconv.FormatInt(start, 10), strconv.FormatInt(start+int64(want)-1, 10)
	return boundPlan{args: out}, nil
}

// planLimit bounds a BYSCORE/BYLEX range: add `LIMIT 0 want`, or lower an existing count that is
// negative (unbounded) or above want. Range arguments start at index 3 only for ZRANGE; the
// legacy forms have the same key min max prefix.
func planLimit(args []string, want int) boundPlan {
	if len(args) < 3 {
		return boundPlan{}
	}
	w := strconv.Itoa(want)
	for i := 3; i < len(args); i++ {
		if !strings.EqualFold(args[i], "LIMIT") {
			continue
		}
		if i+2 >= len(args) {
			return boundPlan{}
		}
		count, err := strconv.ParseInt(args[i+2], 10, 64)
		if err != nil || (count >= 0 && count <= int64(want)) {
			return boundPlan{}
		}
		out := append([]string(nil), args...)
		out[i+2] = w
		return boundPlan{args: out}
	}
	return boundPlan{args: append(append([]string(nil), args...), "LIMIT", "0", w)}
}

// planCount bounds XRANGE/XREVRANGE: add `COUNT want`, or lower an existing larger count.
func planCount(args []string, want int) boundPlan {
	if len(args) < 3 {
		return boundPlan{}
	}
	w := strconv.Itoa(want)
	for i := 3; i < len(args); i++ {
		if !strings.EqualFold(args[i], "COUNT") {
			continue
		}
		if i+1 >= len(args) {
			return boundPlan{}
		}
		count, err := strconv.ParseInt(args[i+1], 10, 64)
		if err != nil || count <= int64(want) {
			return boundPlan{}
		}
		out := append([]string(nil), args...)
		out[i+1] = w
		return boundPlan{args: out}
	}
	return boundPlan{args: append(append([]string(nil), args...), "COUNT", w)}
}

// runBoundedCommand runs c, bounded per planBounded when limit has a row cap.
func runBoundedCommand(ctx context.Context, conn *goredis.Client, c consoleCommand, argv []any, limit page.ResultCap) (any, error) {
	if limit.Rows <= 0 {
		return runConsoleCommand(ctx, conn, argv, c.readTimeout)
	}
	want := limit.Rows + 1
	plan, err := planBounded(c.command, c.args, want, func(cmd, key string) (int64, error) {
		reply, err := runConsoleCommand(ctx, conn, []any{cmd, key}, c.readTimeout)
		if err != nil {
			return 0, err
		}
		n, _ := reply.(int64)
		return n, nil
	})
	if err != nil {
		return nil, err
	}
	switch {
	case plan.scan != nil:
		return runScan(ctx, conn, *plan.scan, want, limit.Bytes, c.readTimeout)
	case plan.args != nil:
		bounded := make([]any, 0, len(plan.args)+1)
		bounded = append(bounded, c.command)
		for _, a := range plan.args {
			bounded = append(bounded, a)
		}
		return runConsoleCommand(ctx, conn, bounded, c.readTimeout)
	default:
		return runConsoleCommand(ctx, conn, argv, c.readTimeout)
	}
}

// runScan walks a SCAN-family cursor until want distinct entries are held or the cursor ends,
// returning the flat reply the original command would have sent. maxBytes > 0 also stops once
// emitted items pass it by one entry, so the page builder still sees that more existed.
func runScan(ctx context.Context, conn *goredis.Client, sp scanPlan, want, maxBytes int, timeout time.Duration) (any, error) {
	seen := map[string]struct{}{}
	held, capped := 0, false
	out := []any{}
	cursor := "0"
	for {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		argv := make([]any, 0, len(sp.head)+len(sp.extra)+1)
		for _, h := range sp.head {
			argv = append(argv, h)
		}
		argv = append(argv, cursor)
		for _, e := range sp.extra {
			argv = append(argv, e)
		}
		reply, err := runConsoleCommand(ctx, conn, argv, timeout)
		if err != nil {
			return nil, err
		}
		round, ok := reply.([]any)
		if !ok || len(round) != 2 {
			return nil, adapters.New(adapters.CodeQuery, "unexpected "+sp.head[0]+" reply", nil)
		}
		next, _ := round[0].(string)
		items, _ := round[1].([]any)
		for i := 0; i+sp.unit <= len(items); i += sp.unit {
			id := formatReplyItem(items[i])
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			for _, k := range sp.emit {
				out = append(out, items[i+k])
				held += len(formatReplyItem(items[i+k]))
			}
			if len(seen) >= want || (maxBytes > 0 && capped) {
				return out, nil
			}
			capped = maxBytes > 0 && held >= maxBytes
		}
		if next == "0" || next == "" {
			return out, nil
		}
		cursor = next
	}
}
