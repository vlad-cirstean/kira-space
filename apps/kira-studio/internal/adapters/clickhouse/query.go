package clickhouse

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
)

// RunningQuery is query.ts's RunningQuery, keyed on the query_id (D8) rather than a thread id or
// backend pid — this adapter's Cancel targets query_id, not a connection-level handle.
type RunningQuery struct {
	QueryID string
}

// TrackQuery is query.ts's TrackQuery — the tracker shape, reused verbatim: registers the
// query about to run and hands back its own release, identity-checked by the caller (adapter.go's
// trackerFor) so a later statement in the same multi-statement op is never unregistered by an
// earlier one settling after it.
type TrackQuery func(RunningQuery) (release func())

// nullSentinel is the *Strings JSON formats' own literal for a Nullable NULL — chosen by
// ClickHouse itself specifically so it can't collide with an empty string. Verified empirically
// against clickhouse-server:26.3: a Nullable(String) NULL and an empty string come back as
// "ᴺᵁᴸᴸ" and "" respectively, never JSON null. It can still collide with a real string of
// the same text: decodeRow reads it as NULL only in a Nullable column, and the table read path
// resolves a Nullable column exactly with an isNull flag (read.go).
const nullSentinel = "ᴺᵁᴸᴸ"

// nullableColumns reports, per header type, whether a column can hold NULL.
func nullableColumns(types []string) []bool {
	out := make([]bool, len(types))
	for i, t := range types {
		_, out[i] = unwrapType(t)
	}
	return out
}

// decodeRow maps the sentinel to NULL for a Nullable column (or one past the header's types).
func decodeRow(values []string, nullable []bool) []*string {
	out := make([]*string, len(values))
	for i, v := range values {
		if v == nullSentinel && (i >= len(nullable) || nullable[i]) {
			continue
		}
		vv := v
		out[i] = &vv
	}
	return out
}

// buildURL is client.ts's own per-request URL construction (no persistent client-level session,
// so every setting travels as a query parameter on every request). extraParams are
// catalog bound values, sent as ClickHouse's own `param_<name>` convention — never
// interpolated into the SQL text.
func buildURL(h *Handle, queryID string, settings, extraParams map[string]string, readOnly bool) string {
	q := url.Values{}
	for k, v := range fixedSettings {
		q.Set(k, v)
	}
	q.Set("database", h.DefaultDatabase)
	if queryID != "" {
		q.Set("query_id", queryID)
	}
	// D7: sent per request, never baked into the client — every data/console/mutation request gets
	// it; Cancel's own KILL QUERY never does (readOnly is always false there).
	if readOnly {
		q.Set("readonly", "2")
	}
	for k, v := range settings {
		q.Set(k, v)
	}
	for k, v := range extraParams {
		q.Set("param_"+k, escapeParamValue(v))
	}
	return h.URL + "/?" + q.Encode()
}

// escapeParamValue doubles a raw backslash before it goes into a `param_<name>` value. The
// server-side value of one of these query parameters is run through ClickHouse's own
// string-literal escape-sequence parser, not taken verbatim — url.Values.Set already handles the
// HTTP transport encoding, but that happens after this: an un-escaped backslash server-side either
// combines with the next character into an unintended escape (e.g. "a\bc" silently becomes
// "a<backspace>c", since \b is a real recognized escape) or, dangling at the end of the value,
// fails outright with CANNOT_PARSE_ESCAPE_SEQUENCE — both observed against a real table named
// with a trailing backslash.
//
// A raw tab, newline or carriage return stops the same parser early ("only 1 of 3 bytes was
// parsed"), so those are escaped too — a database or table name containing one otherwise fails
// every catalog lookup.
func escapeParamValue(v string) string {
	return paramEscaper.Replace(v)
}

var paramEscaper = strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)

func doRequest(ctx context.Context, h *Handle, sql, queryID string, settings, extraParams map[string]string, readOnly bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildURL(h, queryID, settings, extraParams, readOnly), strings.NewReader(sql))
	if err != nil {
		return nil, adapters.New(adapters.CodeQuery, err.Error(), err)
	}
	if h.Username != "" {
		req.Header.Set("X-ClickHouse-User", h.Username)
	}
	if h.Password != "" {
		req.Header.Set("X-ClickHouse-Key", h.Password)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, mapTransportError(err)
	}
	return resp, nil
}

// maxCellBytes bounds a single cell's own text (the app-wide limit every adapter's read path
// respects); scanBufferBytes is the scanner's own line buffer, sized well past the default 64 KiB
// since a single row can carry many such cells.
const (
	maxCellBytes    = 64 * 1024
	scanBufferBytes = 8 * 1024 * 1024
)

// streamRows is query.ts's streamQuery, the reading half — the wire format: line 1 is column
// names, line 2 is column types, everything after is one JSON array of strings (or the sentinel)
// per row. Three terminal conditions (the third extraction site being the one
// @clickhouse/client hid): a non-2xx status, a clean end of body, or a line that fails to parse as
// a JSON array — which means the `__exception__` trailer has begun.
func streamRows(resp *http.Response, onHeader func(names, types []string), onRow func(values []*string) bool) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return mapHTTPError(resp.Header.Get("X-ClickHouse-Exception-Code"), string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, maxCellBytes), scanBufferBytes)
	lineIndex := 0
	var names []string
	var nullable []bool
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var values []string
		if err := json.Unmarshal(line, &values); err != nil {
			rest, _ := io.ReadAll(resp.Body)
			trailer := string(line) + "\n" + string(rest)
			return mapExceptionTrailer(trailer)
		}
		switch lineIndex {
		case 0:
			names = values
		case 1:
			nullable = nullableColumns(values)
			onHeader(names, values)
		default:
			// false: the caller has what it needs. Returning closes the body, ending the read.
			if !onRow(decodeRow(values, nullable)) {
				return nil
			}
		}
		lineIndex++
	}
	if err := scanner.Err(); err != nil {
		return adapters.New(adapters.CodeQuery, err.Error(), err)
	}
	return nil
}

// StreamQuery is query.ts's streamQuery — registers with track but never calls op.SetCommand():
// console.go's execute() calls it once for the whole batch (P5 D9's precedent) and read.go calls
// it itself before this runs.
func StreamQuery(ctx context.Context, h *Handle, queryID string, sql string, op *adapters.OpCtx, track TrackQuery, settings, params map[string]string, onHeader func(names, types []string), onRow func(values []*string) bool) error {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return err
	}
	release := track(RunningQuery{QueryID: queryID})
	_, err := adapters.RunWithAbortRace(ctx, release, func(reqCtx context.Context) (struct{}, error) {
		resp, err := doRequest(reqCtx, h, sql, queryID, settings, params, h.ReadOnly)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, streamRows(resp, onHeader, onRow)
	})
	return err
}

// RunCommand is query.ts's runCommand — a non-row-returning statement (INSERT, DDL) via a plain
// POST, never a FORMAT clause: an INSERT's own FORMAT names the *input* data's format, not an
// output format, so appending one would be a different statement entirely.
func RunCommand(ctx context.Context, h *Handle, queryID string, sql string, op *adapters.OpCtx, track TrackQuery, settings map[string]string) (int64, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return 0, err
	}
	release := track(RunningQuery{QueryID: queryID})
	return adapters.RunWithAbortRace(ctx, release, func(reqCtx context.Context) (int64, error) {
		resp, err := doRequest(reqCtx, h, sql, queryID, settings, nil, h.ReadOnly)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return 0, mapHTTPError(resp.Header.Get("X-ClickHouse-Exception-Code"), string(body))
		}
		// A command's own result body is never shown; drain it unbuffered (a misrouted SELECT can be huge).
		_, _ = io.Copy(io.Discard, resp.Body)
		return writtenRowsFromSummary(resp.Header.Get("X-ClickHouse-Summary")), nil
	})
}

func writtenRowsFromSummary(header string) int64 {
	if header == "" {
		return 0
	}
	var summary struct {
		WrittenRows string `json:"written_rows"`
	}
	if err := json.Unmarshal([]byte(header), &summary); err != nil {
		return 0
	}
	var n int64
	for _, c := range summary.WrittenRows {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// RunCatalogQuery is query.ts's runCatalogQuery — the catalog/read path's own entry point: calls
// op.SetCommand() (Adapter rule 3), binds params as real ClickHouse query parameters
// (`{name:Type}` placeholders — never interpolated), and parses the response as `FORMAT JSON`
// (appended to the adapter-composed SQL text here, never to a user's own console statement) —
// catalog rows are small metadata, not data the page builder needs to see as pre-stringified text.
func RunCatalogQuery[T any](ctx context.Context, h *Handle, queryID string, sql string, op *adapters.OpCtx, track TrackQuery, params map[string]string) ([]T, error) {
	op.SetCommand(sql)
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return nil, err
	}
	release := track(RunningQuery{QueryID: queryID})
	return adapters.RunWithAbortRace(ctx, release, func(reqCtx context.Context) ([]T, error) {
		resp, err := doRequest(reqCtx, h, sql+"\nFORMAT JSON", queryID, nil, params, h.ReadOnly)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, adapters.New(adapters.CodeQuery, err.Error(), err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, mapHTTPError(resp.Header.Get("X-ClickHouse-Exception-Code"), string(body))
		}
		var parsed struct {
			Data []T `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, adapters.New(adapters.CodeQuery, err.Error(), err)
		}
		return parsed.Data, nil
	})
}
