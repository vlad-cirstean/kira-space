package ipcfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// readFirstIDThenFilterToOne is clickhouse_test.go's and mysql_test.go's own shared shape: read
// tablePath's first page, pull the first row's id column, then re-read filtered to
// `id = '<value>'` and assert exactly one row survives. Neither caller reuses readReq/tabular
// afterward, so only the extracted id is returned.
func readFirstIDThenFilterToOne(t *testing.T, rec *Recorder, connectionID, tablePath string) *string {
	t.Helper()
	readReq := adapterhost.ReadRequestWire{
		OpID: "be-read-order-items", ConnectionID: connectionID, Path: tablePath,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	readResp := rec.DataRead(t, readReq, nil)
	logical, err := DecodePage(readResp.Page)
	if err != nil {
		t.Fatalf("decode page: %v", err)
	}
	tabular, ok := logical.(LogicalTabularPage)
	if !ok || len(tabular.Rows) == 0 {
		t.Fatalf("expected a non-empty tabular page, got %+v", logical)
	}
	idColumnIndex := -1
	for i, c := range tabular.Columns {
		if c.Name == "id" {
			idColumnIndex = i
			break
		}
	}
	if idColumnIndex < 0 {
		t.Fatalf("expected an id column in %+v", tabular.Columns)
	}
	firstID := tabular.Rows[0][idColumnIndex]
	if firstID == nil {
		t.Fatal("expected a non-null id in the first row")
	}

	filteredReq := readReq
	filteredReq.OpID = "be-read-order-items-filtered"
	filteredReq.Filter = strp(fmt.Sprintf("`id` = '%s'", *firstID))
	filteredResp := rec.DataRead(t, filteredReq, nil)
	if filteredResp.Page.Rows() != 1 {
		t.Fatalf("filtered read rows = %d, want 1", filteredResp.Page.Rows())
	}
	return firstID
}

func loadCommittedJSONFixture(t *testing.T, path string) committedJSONFixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ipcfixture: read committed fixture %s: %v", path, err)
	}
	var fx committedJSONFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("ipcfixture: parse committed fixture %s: %v", path, err)
	}
	return fx
}

// canonicalize round-trips v through json.Marshal/Unmarshal so two structurally-equal values
// compare equal regardless of Go map key ordering or numeric type — the same normalization the
// TypeScript backend spec's own assert-mode applies (JSON.parse(JSON.stringify(...))).
func canonicalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("ipcfixture: canonicalize: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("ipcfixture: canonicalize: %v", err)
	}
	return out
}

// assertMatchesCommittedJSONFixture is every TestFixture_*'s final step (§4.5 step 1/2): the real
// run's captured Control/Port snapshots must match jsonFixturePath's committed content, modulo the
// keyset continuation-token finding (frozen.go's MaskContinuationTokens).
func assertMatchesCommittedJSONFixture(t *testing.T, rec *Recorder, jsonFixturePath string) {
	t.Helper()
	want := loadCommittedJSONFixture(t, jsonFixturePath)

	gotControl := canonicalize(t, rec.Control)
	wantControl := canonicalize(t, want.ControlSnapshots)
	MaskContinuationTokens(gotControl)
	MaskContinuationTokens(wantControl)
	if diff := cmp.Diff(wantControl, gotControl); diff != "" {
		t.Errorf("control snapshots diff from committed fixture %s (-want +got):\n%s", jsonFixturePath, diff)
	}

	gotPort := canonicalize(t, rec.Port)
	wantPort := canonicalize(t, want.PortSnapshots)
	MaskContinuationTokens(gotPort)
	MaskContinuationTokens(wantPort)
	if diff := cmp.Diff(wantPort, gotPort); diff != "" {
		t.Errorf("port snapshots diff from committed fixture %s (-want +got):\n%s", jsonFixturePath, diff)
	}
}

// repoRootForWrite resolves the repository root from this package's own path
// (apps/kira-space/internal/ipcfixture), the same way every other repo-relative path this package
// needs (write.go's FixturePathFor) is anchored — os.Getwd() during `go test` is always the package
// directory, never the module root.
func repoRootForWrite(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("ipcfixture: getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
}

// maybeWriteFixture is the write-mode branch, the Go analogue of every
// backend.spec.ts's own `if (isFixtureWriteMode()) { writeFixtureModule(...); return; }` — when
// KIRA_IPC_FIXTURES=write, this writes rec's own captured (and already adapter-frozen, per
// frozen.go) snapshots straight to the real tests/ipc/<adapterName>/<adapterName>.fixture.ts,
// exactly as the TypeScript generator this package replaces did, and reports true so the caller
// skips the read-mode assertion. Read mode (the default, and every CI run) always returns false.
func maybeWriteFixture(t *testing.T, rec *Recorder, adapterName string) bool {
	t.Helper()
	if !IsWriteMode() {
		return false
	}
	path := FixturePathFor(repoRootForWrite(t), adapterName)
	if err := WriteFixtureModule(path, filepath.Join("testdata", adapterName+".fixture.json"), adapterName, rec.Control, rec.Port); err != nil {
		t.Fatalf("ipcfixture: write fixture %s: %v", adapterName, err)
	}
	t.Logf("ipcfixture: wrote %s (KIRA_IPC_FIXTURES=write)", path)
	return true
}
