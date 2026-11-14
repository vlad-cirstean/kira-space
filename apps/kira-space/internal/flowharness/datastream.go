package flowharness

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page/wire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// DataStream is a client of the engine data plane (bridge.ServeEngineStream over an in-memory
// connection), the way the renderer's bridge/data.ts is: JSON request frames in, FlatBuffers
// frames out. It is the only way to drive grid read, count, preview and mutate, which have no bound
// service method.
type DataStream struct {
	t    testing.TB
	conn *pipeConn
	wg   sync.WaitGroup

	closeOnce sync.Once
	mu        sync.Mutex
	waiters   map[int]chan []byte
	nextID    int
}

// DataPage is a decoded page of any kind. Tabular rows and the other kinds' chunks are exposed as
// text with nil for NULL.
type DataPage struct {
	Kind      string
	Columns   []DataColumn
	RowCount  int
	Rows      [][]any // tabular: row-major; document: [id, body]; key-value: [field, value]; stream: [key, headers, attrs, timestamp, body]
	Truncated []bool  // per row, any cell clipped
	Binary    [][]bool
	HasMore   bool
	NextToken string
	PrevToken string
	Strategy  string
	// PosTruncated is PagePosition.truncated (a capped read).
	PosTruncated bool
}

// DataColumn is one tabular column descriptor.
type DataColumn struct {
	Name, DataType, TypeClass string
	Nullable, PK, Generated   bool
}

// DataError is a failed request as the wire carries it.
type DataError struct {
	Code, Message string
	// Details is the error frame's optional JSON ("" when absent, and then left out of contracts).
	Details string `json:",omitempty"`
}

func (e *DataError) Error() string { return e.Code + ": " + e.Message }

var dataStreamSeq atomic.Int64

// DataStream opens a data plane connection. Close runs at test cleanup.
func (a *App) DataStream(t testing.TB) *DataStream {
	t.Helper()
	client := &pipeConn{in: make(chan []byte, 64), out: make(chan []byte, 64)}
	server := &pipeConn{in: client.out, out: client.in}
	s := &DataStream{t: t, conn: client, waiters: map[int]chan []byte{}, nextID: int(dataStreamSeq.Add(1)) * 1000}
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		bridge.ServeEngineStream(a.W.Router, server)
		server.closeOut()
	}()
	go func() {
		defer s.wg.Done()
		for raw := range client.in {
			if !wire.FrameBufferHasIdentifier(raw) {
				continue
			}
			f := wire.GetRootAsFrame(raw, 0)
			if f.Kind() != wire.FrameKindres {
				continue
			}
			s.mu.Lock()
			w := s.waiters[int(f.Id())]
			delete(s.waiters, int(f.Id()))
			s.mu.Unlock()
			if w != nil {
				w <- raw
			}
		}
	}()
	t.Cleanup(s.Close)
	return s
}

// Close ends the connection and waits up to 10 s for its goroutines.
func (s *DataStream) Close() {
	s.closeOnce.Do(func() { s.conn.closeOut() })
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.t.Error("data stream did not shut down within 10s")
	}
}

// send issues one request and waits for the response frame.
func (s *DataStream) send(op string, payload any) (*wire.Frame, error) {
	s.t.Helper()
	reply, id := s.fire(op, payload)
	select {
	case raw := <-reply:
		return decodeResponse(id, raw)
	case <-time.After(60 * time.Second):
		s.t.Fatalf("data stream %s: no response in 60s", op)
		return nil, nil
	}
}

func (s *DataStream) fire(op string, payload any) (chan []byte, int) {
	s.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		s.t.Fatalf("data stream %s: marshal: %v", op, err)
	}
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	reply := make(chan []byte, 1)
	s.waiters[id] = reply
	s.mu.Unlock()
	frame, _ := json.Marshal(map[string]any{"kind": "req", "id": id, "op": op, "payload": json.RawMessage(body)})
	_ = s.conn.Send(frame)
	return reply, id
}

func decodeResponse(id int, raw []byte) (*wire.Frame, error) {
	f := wire.GetRootAsFrame(raw, 0)
	if !f.Ok() {
		e := f.Error(nil)
		if e == nil {
			return nil, &DataError{Code: string(ipcerr.CodeInternal), Message: fmt.Sprintf("request %d failed without detail", id)}
		}
		return nil, &DataError{Code: string(e.Code()), Message: string(e.Message()), Details: string(e.Details())}
	}
	return f, nil
}

// ReadArgs is one data:read request. Zero PageSize means 100; zero Cursor means offset 0.
type ReadArgs struct {
	OpID         string
	ConnectionID string
	Path         string
	Projection   []string
	Filter       *string
	Sort         *model.SortSpec
	PageSize     int
	Cursor       model.PageCursor
}

func (r ReadArgs) wire() adapterhost.ReadRequestWire {
	if r.OpID == "" {
		r.OpID = ids.NewID()
	}
	if r.PageSize == 0 {
		r.PageSize = 100
	}
	if r.Cursor.Mode == "" {
		r.Cursor = model.PageCursor{Mode: "offset"}
	}
	return adapterhost.ReadRequestWire{OpID: r.OpID, ConnectionID: r.ConnectionID, Path: r.Path, Projection: r.Projection,
		Filter: r.Filter, Sort: r.Sort, PageSize: r.PageSize, Cursor: r.Cursor}
}

// Read sends data:read and decodes the page.
func (s *DataStream) Read(args ReadArgs) (DataPage, string, error) {
	s.t.Helper()
	f, err := s.send("data:read", args.wire())
	if err != nil {
		return DataPage{}, "", err
	}
	var rr wire.ReadResponse
	rr.Init(payloadTab(s.t, f).Bytes, payloadTab(s.t, f).Pos)
	var p wire.Page
	if rr.Page(&p) == nil {
		s.t.Fatal("read response has no page")
	}
	return decodePage(s.t, &p), sourceName(rr.Source()), nil
}

// ReadFire starts a read without waiting; Wait returns its result. Pair with Router.Cancel(opID).
func (s *DataStream) ReadFire(args ReadArgs) (opID string, wait func() (DataPage, error)) {
	s.t.Helper()
	w := args.wire()
	reply, id := s.fire("data:read", w)
	return w.OpID, func() (DataPage, error) {
		raw := <-reply
		f, err := decodeResponse(id, raw)
		if err != nil {
			return DataPage{}, err
		}
		var rr wire.ReadResponse
		rr.Init(payloadTab(s.t, f).Bytes, payloadTab(s.t, f).Pos)
		var p wire.Page
		rr.Page(&p)
		return decodePage(s.t, &p), nil
	}
}

// Count sends data:count.
func (s *DataStream) Count(connID, path string, filter *string, refresh bool) (value int64, exact bool, err error) {
	s.t.Helper()
	f, err := s.send("data:count", adapterhost.CountRequestWire{OpID: ids.NewID(), ConnectionID: connID, Path: path, Filter: filter, Refresh: refresh})
	if err != nil {
		return 0, false, err
	}
	var cr wire.CountResponse
	tab := payloadTab(s.t, f)
	cr.Init(tab.Bytes, tab.Pos)
	return int64(cr.Value()), cr.Exact(), nil
}

// Preview sends data:preview and returns the rendered statements.
func (s *DataStream) Preview(connID, path string, ops []model.MutationRowOp) ([]string, error) {
	s.t.Helper()
	f, err := s.send("data:preview", adapterhost.PreviewRequestWire{ConnectionID: connID, Path: path, Ops: ops})
	if err != nil {
		return nil, err
	}
	var pr wire.PreviewResponse
	tab := payloadTab(s.t, f)
	pr.Init(tab.Bytes, tab.Pos)
	out := make([]string, pr.StatementsLength())
	for i := range out {
		out[i] = string(pr.Statements(i))
	}
	return out, nil
}

// MutateWire is the exact request the renderer sends; contracts record it.
func MutateWire(connID, path string, ops []model.MutationRowOp) adapterhost.MutateRequestWire {
	return adapterhost.MutateRequestWire{OpID: "<op>", ConnectionID: connID, Path: path, Ops: ops}
}

// Mutate sends data:mutate.
func (s *DataStream) Mutate(connID, path string, ops []model.MutationRowOp) (int, error) {
	s.t.Helper()
	return s.MutateOp(ids.NewID(), connID, path, ops)
}

// MutateOp is Mutate with a caller-chosen op id, so a test can Router.Cancel it.
func (s *DataStream) MutateOp(opID, connID, path string, ops []model.MutationRowOp) (int, error) {
	s.t.Helper()
	f, err := s.send("data:mutate", adapterhost.MutateRequestWire{OpID: opID, ConnectionID: connID, Path: path, Ops: ops})
	if err != nil {
		return 0, err
	}
	var mr wire.MutateResponse
	tab := payloadTab(s.t, f)
	mr.Init(tab.Bytes, tab.Pos)
	return int(mr.AffectedRows()), nil
}

// Invalidate sends data:invalidate (scope "", "all" or "pages").
func (s *DataStream) Invalidate(connID, path, scope string) error {
	s.t.Helper()
	_, err := s.send("data:invalidate", adapterhost.InvalidateRequestWire{ConnectionID: connID, Path: path, Scope: scope})
	return err
}

// Execute sends data:execute.
func (s *DataStream) Execute(connID, path string, stmts ...string) ([]DataPage, error) {
	s.t.Helper()
	f, err := s.send("data:execute", adapterhost.ExecuteRequestWire{OpID: ids.NewID(), ConnectionID: connID, Path: path, Statements: stmts})
	if err != nil {
		return nil, err
	}
	var er wire.ExecuteResponse
	tab := payloadTab(s.t, f)
	er.Init(tab.Bytes, tab.Pos)
	out := make([]DataPage, er.PagesLength())
	for i := range out {
		var p wire.Page
		er.Pages(&p, i)
		out[i] = decodePage(s.t, &p)
	}
	return out, nil
}

func payloadTab(t testing.TB, f *wire.Frame) flatbuffers.Table {
	t.Helper()
	var tab flatbuffers.Table
	if !f.Payload(&tab) {
		t.Fatal("response frame has no payload")
	}
	return tab
}

func sourceName(s wire.Source) string { return wire.EnumNamesSource[s] }

func decodeChunk(c *wire.Chunk) (cells []any, trunc, binary []bool) {
	if c == nil {
		return nil, nil, nil
	}
	pc := page.Chunk{Data: c.DataBytes(), Nulls: c.NullsBytes()}
	for i := 0; i < c.OffsetsLength(); i++ {
		pc.Offsets = append(pc.Offsets, c.Offsets(i))
	}
	for i := 0; i < c.TruncatedLength(); i++ {
		pc.Truncated = append(pc.Truncated, c.Truncated(i))
	}
	for i := 0; i < c.BinaryLength(); i++ {
		pc.Binary = append(pc.Binary, c.Binary(i))
	}
	n := max(len(pc.Offsets)-1, 0)
	for i := 0; i < n; i++ {
		if page.IsNull(pc, i) {
			cells = append(cells, nil)
		} else {
			cells = append(cells, page.CellText(pc, i))
		}
		trunc = append(trunc, page.IsTruncated(pc, i))
		binary = append(binary, page.IsBinary(pc, i))
	}
	return cells, trunc, binary
}

func decodePosition(dp *DataPage, pos *wire.PagePosition) {
	if pos == nil {
		return
	}
	dp.HasMore = pos.HasMore()
	dp.NextToken = string(pos.NextToken())
	dp.PrevToken = string(pos.PrevToken())
	dp.Strategy = wire.EnumNamesStrategy[pos.Strategy()]
	dp.PosTruncated = pos.Truncated()
}

func decodePage(t testing.TB, p *wire.Page) DataPage {
	t.Helper()
	var body flatbuffers.Table
	if !p.Body(&body) {
		t.Fatal("page has no body")
	}
	var dp DataPage
	switch p.BodyType() {
	case wire.PageBodyTabularPage:
		var tp wire.TabularPage
		tp.Init(body.Bytes, body.Pos)
		dp.Kind, dp.RowCount = "tabular", int(tp.RowCount())
		for i := 0; i < tp.ColumnsLength(); i++ {
			var cd wire.ColumnDescriptor
			tp.Columns(&cd, i)
			dp.Columns = append(dp.Columns, DataColumn{Name: string(cd.Name()), DataType: string(cd.DataType()), TypeClass: wire.EnumNamesTypeClass[cd.TypeClass()],
				Nullable: cd.Nullable(), PK: cd.IsPrimaryKey(), Generated: cd.Generated()})
		}
		dp.Rows = make([][]any, dp.RowCount)
		dp.Truncated = make([]bool, dp.RowCount)
		dp.Binary = make([][]bool, dp.RowCount)
		for r := range dp.Rows {
			dp.Rows[r] = make([]any, len(dp.Columns))
			dp.Binary[r] = make([]bool, len(dp.Columns))
		}
		for ci := 0; ci < tp.ChunksLength(); ci++ {
			var c wire.Chunk
			tp.Chunks(&c, ci)
			cells, tr, bin := decodeChunk(&c)
			for r := range cells {
				if r < dp.RowCount {
					dp.Rows[r][ci] = cells[r]
					dp.Truncated[r] = dp.Truncated[r] || tr[r]
					dp.Binary[r][ci] = bin[r]
				}
			}
		}
		decodePosition(&dp, tp.Position(nil))
	case wire.PageBodyDocumentPage:
		var d wire.DocumentPage
		d.Init(body.Bytes, body.Pos)
		dp.Kind, dp.RowCount = "document", int(d.RowCount())
		dp.Rows = zipChunks(d.Ids(nil), d.Bodies(nil))
		_, dp.Truncated, _ = decodeChunk(d.Bodies(nil))
		decodePosition(&dp, d.Position(nil))
	case wire.PageBodyKeyValuePage:
		var k wire.KeyValuePage
		k.Init(body.Bytes, body.Pos)
		dp.Kind, dp.RowCount = "keyvalue", int(k.RowCount())
		dp.Rows = zipChunks(k.Fields(nil), k.Values(nil))
		// Binary marks a value the page flagged as not text; the field cell is never binary.
		_, _, vbin := decodeChunk(k.Values(nil))
		dp.Binary = make([][]bool, len(vbin))
		for i, b := range vbin {
			dp.Binary[i] = []bool{false, b}
		}
		decodePosition(&dp, k.Position(nil))
	case wire.PageBodyStreamPage:
		var sp wire.StreamPage
		sp.Init(body.Bytes, body.Pos)
		dp.Kind, dp.RowCount = "stream", int(sp.RowCount())
		dp.Rows = zipChunks(sp.Keys(nil), sp.Headers(nil), sp.Attrs(nil), sp.Timestamps(nil), sp.Bodies(nil))
		decodePosition(&dp, sp.Position(nil))
	default:
		t.Fatalf("unknown page body type %v", p.BodyType())
	}
	return dp
}

func zipChunks(chunks ...*wire.Chunk) [][]any {
	cols := make([][]any, len(chunks))
	rows := 0
	for i, c := range chunks {
		cols[i], _, _ = decodeChunk(c)
		rows = max(rows, len(cols[i]))
	}
	out := make([][]any, rows)
	for r := range out {
		out[r] = make([]any, len(cols))
		for c := range cols {
			if r < len(cols[c]) {
				out[r][c] = cols[c][r]
			}
		}
	}
	return out
}

// DownloadFire starts data:objectDownload without waiting; Wait returns the byte count. Pair with
// Router.Cancel(opID).
func (s *DataStream) DownloadFire(connID, path, dest string) (opID string, wait func() (int64, error)) {
	s.t.Helper()
	opID = ids.NewID()
	reply, id := s.fire("data:objectDownload", adapterhost.ObjectDownloadRequestWire{OpID: opID, ConnectionID: connID, Path: path, DestPath: dest})
	return opID, func() (int64, error) {
		f, err := decodeResponse(id, <-reply)
		if err != nil {
			return 0, err
		}
		var dr wire.ObjectDownloadResponse
		tab := payloadTab(s.t, f)
		dr.Init(tab.Bytes, tab.Pos)
		return int64(dr.Bytes()), nil
	}
}

// Download sends data:objectDownload and waits.
func (s *DataStream) Download(connID, path, dest string) (int64, error) {
	s.t.Helper()
	_, wait := s.DownloadFire(connID, path, dest)
	return wait()
}
