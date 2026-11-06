package datagrip

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// recordingCreator implements Creator, recording every input it was given and optionally failing
// on a chosen 1-based call index (§4.2 case 24's "a Creator error on row 2 of 4").
type recordingCreator struct {
	inputs  []CreatorInput
	failOn  int // 1-based; 0 means never fail
	callNum int
}

func (c *recordingCreator) Create(in CreatorInput) (model.ConnectionSummary, error) {
	c.callNum++
	c.inputs = append(c.inputs, in)
	if c.failOn != 0 && c.callNum == c.failOn {
		return model.ConnectionSummary{}, fmt.Errorf("simulated failure on row %d", c.callNum)
	}
	return model.ConnectionSummary{ID: fmt.Sprintf("id-%d", c.callNum), ConnectionFields: in.ConnectionFields}, nil
}

// fourSourceProject writes four plain postgres-shaped sources with distinct uuids, hosts and
// usernames, none of them password-bearing via any credential store (secret-storage=forget) so
// Apply's Creator-recording behaviour can be asserted with no filesystem credential lookups
// involved at all.
func fourSourceProject(t *testing.T) (dir string, uuids []string) {
	t.Helper()
	dir = t.TempDir()
	ideaDir := filepath.Join(dir, ".idea")
	if err := os.MkdirAll(ideaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var shared, local string
	shared = `<?xml version="1.0" encoding="UTF-8"?><project version="4"><component name="DataSourceManagerImpl" format="xml" multifile-model="true">`
	local = `<?xml version="1.0" encoding="UTF-8"?><project version="4"><component name="dataSourceStorageLocal" created-in="DB-243.22562.220">`
	for i := 1; i <= 4; i++ {
		uuid := fmt.Sprintf("bbbbbbbb-0000-0000-0000-00000000000%d", i)
		uuids = append(uuids, uuid)
		name := fmt.Sprintf("row%d", i)
		host := fmt.Sprintf("host%d.example.internal", i)
		shared += fmt.Sprintf(`<data-source source="LOCAL" name="%s" uuid="%s"><driver-ref>postgresql</driver-ref><jdbc-driver>x</jdbc-driver><jdbc-url>jdbc:postgresql://%s:5432/db%d</jdbc-url></data-source>`, name, uuid, host, i)
		local += fmt.Sprintf(`<data-source name="%s" uuid="%s"><secret-storage>forget</secret-storage><user-name>user%d</user-name></data-source>`, name, uuid, i)
	}
	shared += `</component></project>`
	local += `</component></project>`

	if err := os.WriteFile(filepath.Join(ideaDir, "dataSources.xml"), []byte(shared), 0o644); err != nil {
		t.Fatalf("write dataSources.xml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ideaDir, "dataSources.local.xml"), []byte(local), 0o644); err != nil {
		t.Fatalf("write dataSources.local.xml: %v", err)
	}
	return dir, uuids
}

// TestApplyCreatesOnlySelectedRowsInOrder is case 24's first half: selected rows create in order
// with the right kind/host/port/database/username; an unselected row creates nothing.
func TestApplyCreatesOnlySelectedRowsInOrder(t *testing.T) {
	dir, uuids := fourSourceProject(t)
	creator := &recordingCreator{}

	// Select rows 1, 2 and 4 — row 3 must never reach Create.
	report, err := Apply(dir, []string{uuids[0], uuids[1], uuids[3]}, true, creator)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(report.Rows) != 3 {
		t.Fatalf("got %d report rows, want 3", len(report.Rows))
	}
	if len(creator.inputs) != 3 {
		t.Fatalf("got %d Create calls, want 3", len(creator.inputs))
	}
	for i, want := range []struct {
		host, username, database string
	}{
		{"host1.example.internal", "user1", "db1"},
		{"host2.example.internal", "user2", "db2"},
		{"host4.example.internal", "user4", "db4"},
	} {
		in := creator.inputs[i]
		if in.Kind != "postgres" {
			t.Errorf("row %d: Kind = %q, want postgres", i, in.Kind)
		}
		if in.Host == nil || *in.Host != want.host {
			t.Errorf("row %d: Host = %v, want %q", i, in.Host, want.host)
		}
		if in.Port == nil || *in.Port != 5432 {
			t.Errorf("row %d: Port = %v, want 5432", i, in.Port)
		}
		if in.Username == nil || *in.Username != want.username {
			t.Errorf("row %d: Username = %v, want %q", i, in.Username, want.username)
		}
		if in.Database == nil || *in.Database != want.database {
			t.Errorf("row %d: Database = %v, want %q", i, in.Database, want.database)
		}
		// secret-storage is "forget" for every row here, so no password is ever attempted.
		if in.Password != nil {
			t.Errorf("row %d: Password = %v, want nil (secret-storage: forget)", i, in.Password)
		}
	}
}

// TestApplyPartialFailureLeavesOtherRowsCreated is case 24's second half: a Creator error on row
// 2 of 4 leaves rows 1, 3, 4 created and the report marking row 2 failed.
func TestApplyPartialFailureLeavesOtherRowsCreated(t *testing.T) {
	dir, uuids := fourSourceProject(t)
	creator := &recordingCreator{failOn: 2}

	report, err := Apply(dir, uuids, true, creator)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(report.Rows) != 4 {
		t.Fatalf("got %d report rows, want 4", len(report.Rows))
	}
	if len(creator.inputs) != 4 {
		t.Fatalf("got %d Create calls, want 4 (row 2's failure must not stop the rest)", len(creator.inputs))
	}
	for i, row := range report.Rows {
		if i == 1 { // row 2, 0-indexed
			if row.Created {
				t.Errorf("row 2: Created = true, want false")
			}
			if row.Error == "" {
				t.Errorf("row 2: Error is empty, want the Creator's failure message")
			}
			continue
		}
		if !row.Created {
			t.Errorf("row %d: Created = false, want true (its own Create call did not fail)", i+1)
		}
	}
}

// TestLookupPasswordKeepassConfiguredIsUnsupported is the regression case: a project whose
// security.xml says PROVIDER=KEEPASS now has no backend left to try (the file-based PasswordSafe
// store was removed), and must get a named ReasonCredentialStoreUnsupported refusal rather than
// being silently folded into ReasonPasswordNotSaved (which would misreport a real,
// readable-by-DataGrip store as "no password saved") or a bare miss.
func TestLookupPasswordKeepassConfiguredIsUnsupported(t *testing.T) {
	ds := DataSource{UUID: "dddddddd-0000-0000-0000-000000000001"}
	_, _, err := lookupPassword(ds, SecurityConfig{Provider: "KEEPASS"})
	if err == nil {
		t.Fatalf("expected an error")
	}
	re, ok := err.(*RefusalError)
	if !ok {
		t.Fatalf("error is not a *RefusalError: %v", err)
	}
	if re.Code != ReasonCredentialStoreUnsupported {
		t.Errorf("Code = %q, want %q", re.Code, ReasonCredentialStoreUnsupported)
	}
}

// TestApplySecretsUnavailableImportsEveryRowWithNoPassword is case 25: with
// secrets.Status.Available == false, every Input.Password is nil and every row carries
// secret-storage-unavailable — but every row still creates.
func TestApplySecretsUnavailableImportsEveryRowWithNoPassword(t *testing.T) {
	uuid := "cccccccc-0000-0000-0000-000000000001"
	shared := sharedSourceXML(uuid, "s", "postgresql", "jdbc:postgresql://host:5432/db", false)
	// master_key (a real saved password) so the only thing suppressing the password is
	// secretsAvailable=false below, not an upstream password-not-saved skip.
	local := `<?xml version="1.0" encoding="UTF-8"?><project version="4"><component name="dataSourceStorageLocal" created-in="DB-243.22562.220"><data-source name="s" uuid="` + uuid + `"><secret-storage>master_key</secret-storage><user-name>u</user-name></data-source></component></project>`
	dir := writeSyntheticProject(t, shared, local)

	creator := &recordingCreator{}
	report, err := Apply(dir, []string{uuid}, false, creator)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(creator.inputs) != 1 {
		t.Fatalf("got %d Create calls, want 1", len(creator.inputs))
	}
	if creator.inputs[0].Password != nil {
		t.Errorf("Password = %v, want nil", creator.inputs[0].Password)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("got %d report rows, want 1", len(report.Rows))
	}
	if report.Rows[0].Error != ReasonSecretStorageUnavailable {
		t.Errorf("Error = %q, want %q", report.Rows[0].Error, ReasonSecretStorageUnavailable)
	}
	if !report.Rows[0].Created {
		t.Errorf("Created = false, want true (only the password is missing, per D8)")
	}
	if report.Rows[0].PasswordImported {
		t.Errorf("PasswordImported = true, want false")
	}
}

// TestApplySecretsUnavailableNeverLooksUpPassword: the Keychain lookup (a
// real macOS authorization panel per row) must never run for a row whose password would only be
// discarded afterward by the secretsAvailable check — checking secretsAvailable first, before
// attempting the lookup at all, must skip lookupPasswordFn entirely rather than call it and throw
// the result away.
func TestApplySecretsUnavailableNeverLooksUpPassword(t *testing.T) {
	uuid := "cccccccc-0000-0000-0000-000000000002"
	shared := sharedSourceXML(uuid, "s", "postgresql", "jdbc:postgresql://host:5432/db", false)
	// master_key: a real saved password DataGrip claims to hold, so the only thing that could ever
	// suppress the lookup is secretsAvailable=false below — not an upstream memory/forget skip.
	local := `<?xml version="1.0" encoding="UTF-8"?><project version="4"><component name="dataSourceStorageLocal" created-in="DB-243.22562.220"><data-source name="s" uuid="` + uuid + `"><secret-storage>master_key</secret-storage><user-name>u</user-name></data-source></component></project>`
	dir := writeSyntheticProject(t, shared, local)

	calls := 0
	original := lookupPasswordFn
	lookupPasswordFn = func(ds DataSource, cfg SecurityConfig) (string, string, error) {
		calls++
		return original(ds, cfg)
	}
	t.Cleanup(func() { lookupPasswordFn = original })

	creator := &recordingCreator{}
	report, err := Apply(dir, []string{uuid}, false, creator)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if calls != 0 {
		t.Errorf("lookupPasswordFn was called %d time(s), want 0 — secretsAvailable=false must skip the Keychain lookup entirely", calls)
	}
	if len(report.Rows) != 1 || report.Rows[0].Error != ReasonSecretStorageUnavailable {
		t.Fatalf("report = %+v, want one row with Error %q", report.Rows, ReasonSecretStorageUnavailable)
	}
}
