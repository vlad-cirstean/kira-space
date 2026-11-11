package mobileweb

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/lannet"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/tokenauth"
)

type fakeStore struct {
	mu      sync.Mutex
	rows    map[string]repos.MobileDeviceRow
	lookErr error
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]repos.MobileDeviceRow{}} }

func (f *fakeStore) ByID(id string) (repos.MobileDeviceRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lookErr != nil {
		return repos.MobileDeviceRow{}, false, f.lookErr
	}
	r, ok := f.rows[id]
	return r, ok, nil
}

func (f *fakeStore) Insert(r repos.MobileDeviceRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[r.ID] = r
	return nil
}

func (f *fakeStore) TouchLastSeen(string, int64, string) error { return nil }

func (f *fakeStore) Revoke(id string, now int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return errors.New("no such device")
	}
	r.RevokedAt = &now
	f.rows[id] = r
	return nil
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

// addDevice inserts a device without write permissions and returns its cookie value.
func (f *fakeStore) addDevice(t *testing.T, id string) string {
	t.Helper()
	return f.addDeviceWith(t, id, false, false)
}

// addDeviceWith inserts a device with the given permission flags and returns its cookie value.
func (f *fakeStore) addDeviceWith(t *testing.T, id string, write, agentInput bool) string {
	t.Helper()
	plain, hash, salt, err := tokenauth.Mint()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Insert(repos.MobileDeviceRow{ID: id, Label: "phone", TokenHash: hash, TokenSalt: salt, CanWrite: write, CanAgentInput: agentInput, ExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	return id + "." + plain
}

type fakeReader struct {
	board adewire.Board
	calls int
	mu    sync.Mutex
	gate  chan struct{}
}

func (f *fakeReader) Board(context.Context) (adewire.Board, error) {
	f.mu.Lock()
	f.calls++
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return f.board, nil
}
func (f *fakeReader) ChangeRequests(context.Context) (adewire.ChangeRequestsResult, error) {
	return adewire.ChangeRequestsResult{}, nil
}
func (f *fakeReader) Sessions(context.Context) (adewire.SessionsResult, error) {
	return adewire.SessionsResult{}, nil
}
func (f *fakeReader) Workflows(context.Context) (adewire.WorkflowsResult, error) {
	return adewire.WorkflowsResult{}, nil
}
func (f *fakeReader) Repos(context.Context) (adewire.ReposResult, error) {
	return adewire.ReposResult{Repos: []adewire.Repo{{CodeRepoID: "r1", Name: "api", PrepareScript: "secret"}}}, nil
}
func (f *fakeReader) HostProviders(context.Context) (githost.Providers, error) {
	return githost.Providers{}, nil
}
func (f *fakeReader) ArchivedTask(context.Context, adewire.TaskArgs) (adewire.ArchivedTask, error) {
	return adewire.ArchivedTask{}, nil
}
func (f *fakeReader) ReadLog(_ context.Context, a adewire.ReadLogArgs) (adewire.LogPage, error) {
	return adewire.LogPage{NextSeq: a.AfterSeq + 1}, nil
}

// fakeWriter records the calls a phone write reaches and answers with scripted results.
type fakeWriter struct {
	mu    sync.Mutex
	calls []string
	args  []any
	err   error
}

func (f *fakeWriter) record(name string, a any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	f.args = append(f.args, a)
	return f.err
}

func (f *fakeWriter) MarkSessionSeen(context.Context, adewire.MarkSessionSeenArgs) (bool, error) {
	return false, nil
}

func (f *fakeWriter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeWriter) CaptureTask(_ context.Context, a adewire.CaptureTaskArgs) (adewire.Task, error) {
	return adewire.Task{ID: "new", Title: a.Title}, f.record("CaptureTask", a)
}
func (f *fakeWriter) ShelveTask(_ context.Context, a adewire.ShelveTaskArgs) error {
	return f.record("ShelveTask", a)
}
func (f *fakeWriter) SetTaskStep(_ context.Context, a adewire.SetTaskStepArgs) (adewire.Task, error) {
	return adewire.Task{ID: a.TaskID, EntryStepID: a.StepID}, f.record("SetTaskStep", a)
}
func (f *fakeWriter) StartRun(_ context.Context, a adewire.StartRunArgs) (adewire.StartRunResult, error) {
	return adewire.StartRunResult{RunIDs: []string{"run1"}}, f.record("StartRun", a)
}
func (f *fakeWriter) LaunchStep(_ context.Context, a adewire.LaunchStepArgs) (LaunchResult, error) {
	return LaunchResult{SessionID: "sess1"}, f.record("LaunchStep", a)
}
func (f *fakeWriter) Send(_ context.Context, a adewire.SendArgs) error { return f.record("Send", a) }
func (f *fakeWriter) TakeOver(_ context.Context, a adewire.TakeOverArgs) (LaunchResult, error) {
	return LaunchResult{SessionID: a.SessionID}, f.record("TakeOver", a)
}

type fakeTerminals struct {
	mu       sync.Mutex
	served   []string
	released []string
}

func (f *fakeTerminals) Serve(w http.ResponseWriter, _ *http.Request, _ repos.MobileDeviceRow, id string, _ func() bool) {
	f.mu.Lock()
	f.served = append(f.served, id)
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
func (f *fakeTerminals) ReleaseDevice(id string) {
	f.mu.Lock()
	f.released = append(f.released, id)
	f.mu.Unlock()
}
func (f *fakeTerminals) ReleaseAll(string) {}

func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html":  {Data: []byte("<html>app</html>")},
		"assets/a.js": {Data: []byte("//a")},
	}
}

func newTestServer(t *testing.T) (*Server, *fakeStore, *fakeReader) {
	t.Helper()
	store, reader := newFakeStore(), &fakeReader{}
	s := New(Config{
		Reader: reader, Writer: &fakeWriter{}, Terminals: &fakeTerminals{}, AgentInputEnabled: func() bool { return true },
		AgentSessions: func() any { return map[string]any{"sessions": []any{}} },
		Devices:       store, Hub: NewHub(), Broker: NewBroker(time.Now, nil), Assets: testAssets(),
		Port: 7790, Network: loopbackNet(),
	})
	s.isLAN = loopbackOK
	return s, store, reader
}

// do runs a request through the guarded app handler as a peer at remote.
func do(s *Server, method, target, remote string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	return doBody(s, method, target, remote, "", mutate)
}

func doBody(s *Server, method, target, remote, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1:7790"+target, strings.NewReader(body))
	req.RemoteAddr = remote
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	s.guard(securityHeaders(s.appMux())).ServeHTTP(rec, req)
	return rec
}

func withCookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: v}) }
}

// loopbackNet binds tests to 127.0.0.1 inside 127.0.0.0/8; loopbackOK widens the LAN check to it.
func loopbackNet() lannet.Network {
	return lannet.Network{Interface: "lo", Addr: netip.MustParsePrefix("127.0.0.1/8")}
}

func loopbackOK(a netip.Addr) bool { return a.Is4() && (a.IsLoopback() || lannet.IsLAN(a)) }

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func portStr(p int) string { return strconv.Itoa(p) }

func contextCanceled() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, cancel
}
