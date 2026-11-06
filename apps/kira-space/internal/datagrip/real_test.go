package datagrip

import (
	"os"
	"testing"
)

// TestRealDataGripProject is the opt-in check for a machine that has DataGrip
// installed and a real project to point at — never run in CI, never in this sandbox (no display,
// DataGrip is commercial). Set KIRA_DATAGRIP_REAL_PROJECT to a real DataGrip project folder (one
// containing .idea/dataSources.xml) to run it. This is the check that would upgrade the
// "format read out of the source" into "verified against a real profile" — nothing else in this
// package's test suite does that.
func TestRealDataGripProject(t *testing.T) {
	dir := os.Getenv("KIRA_DATAGRIP_REAL_PROJECT")
	if dir == "" {
		t.Skip("set KIRA_DATAGRIP_REAL_PROJECT to a real DataGrip project folder to run this")
	}

	preview, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan(%s): %v", dir, err)
	}
	if len(preview.Rows) == 0 {
		t.Fatalf("Scan found no data sources in %s", dir)
	}

	var uuids []string
	for _, row := range preview.Rows {
		if row.Importable {
			uuids = append(uuids, row.UUID)
		}
	}
	if len(uuids) == 0 {
		t.Fatalf("no importable data sources in %s", dir)
	}

	// recordingCreator (apply_test.go) never actually writes a connection — this test only
	// exercises the scan/decrypt path, never internal/connections.
	report, err := Apply(dir, uuids, true, &recordingCreator{})
	if err != nil {
		t.Fatalf("Apply(%s): %v", dir, err)
	}
	found := false
	for _, row := range report.Rows {
		if row.PasswordImported {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no data source in %s resolved a non-empty password; report: %+v", dir, report.Rows)
	}
}
