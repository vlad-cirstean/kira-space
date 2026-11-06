package datagrip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSyntheticProject builds a one-off .idea/dataSources.xml (+ optional dataSources.local.xml)
// under a fresh temp dir, using the same element/attribute shapes as testdata/project-six's real,
// citation-backed fixture — a temp file rather than another static testdata/ directory per case,
// since many small, one-field-different variants are better expressed inline than as a dozen
// near-duplicate checked-in files. Returns the project directory (the picker's own
// return value) to pass to resolveFields/Scan.
func writeSyntheticProject(t *testing.T, sharedXML, localXML string) string {
	t.Helper()
	dir := t.TempDir()
	ideaDir := filepath.Join(dir, ".idea")
	if err := os.MkdirAll(ideaDir, 0o755); err != nil {
		t.Fatalf("mkdir .idea: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ideaDir, "dataSources.xml"), []byte(sharedXML), 0o644); err != nil {
		t.Fatalf("write dataSources.xml: %v", err)
	}
	if localXML != "" {
		if err := os.WriteFile(filepath.Join(ideaDir, "dataSources.local.xml"), []byte(localXML), 0o644); err != nil {
			t.Fatalf("write dataSources.local.xml: %v", err)
		}
	}
	return dir
}

func sharedSourceXML(uuid, name, driverRef, jdbcURL string, configuredByURL bool) string {
	cbu := ""
	if configuredByURL {
		cbu = "<configured-by-url>true</configured-by-url>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="DataSourceManagerImpl" format="xml" multifile-model="true">
    <data-source source="LOCAL" name="` + name + `" uuid="` + uuid + `">
      <driver-ref>` + driverRef + `</driver-ref>
      <jdbc-driver>x</jdbc-driver>
      <jdbc-url>` + jdbcURL + `</jdbc-url>
      ` + cbu + `
    </data-source>
  </component>
</project>`
}

func localSourceXML(uuid, name, dbms, sshEnabled string) string {
	dbInfo := ""
	if dbms != "" {
		dbInfo = `<database-info product="x" dbms="` + dbms + `" exact-version="0" />`
	}
	ssh := ""
	if sshEnabled != "" {
		ssh = `<ssh-properties><enabled>` + sshEnabled + `</enabled></ssh-properties>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="dataSourceStorageLocal" created-in="DB-243.22562.220">
    <data-source name="` + name + `" uuid="` + uuid + `">
      ` + dbInfo + `
      ` + ssh + `
    </data-source>
  </component>
</project>`
}

func oneSource(t *testing.T, dir string) DataSource {
	t.Helper()
	project, err := ParseProject(dir)
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	if len(project.DataSources) != 1 {
		t.Fatalf("got %d data sources, want 1", len(project.DataSources))
	}
	return project.DataSources[0]
}

// TestEngineCascade is case 5: dbms wins over driver-ref wins over URL scheme; family suffixes
// strip; an unmapped driver-ref reports unsupported-engine with the driver-ref quoted.
func TestEngineCascade(t *testing.T) {
	tests := []struct {
		name      string
		dbms      string
		driverRef string
		jdbcURL   string
		wantKind  string
		wantOK    bool
		wantLabel string
	}{
		{"dbms wins over conflicting driver-ref/url", "MYSQL", "postgresql", "jdbc:postgresql://h/d", "mysql", true, ""},
		{"driver-ref wins over conflicting url", "", "mariadb", "jdbc:mysql://h/d", "mariadb", true, ""},
		{"mysql.8 family strips to mysql", "", "mysql.8", "jdbc:mysql://h/d", "mysql", true, ""},
		{"sqlite.xerial family strips to sqlite", "", "sqlite.xerial", "jdbc:sqlite:/abs/db", "sqlite", true, ""},
		{"mongo.4 family strips to mongodb", "", "mongo.4", "jdbc:mongodb://h/d", "mongodb", true, ""},
		{"url scheme used when no dbms/driver-ref hit", "", "", "jdbc:postgresql://h/d", "postgres", true, ""},
		{"oracle is unsupported, driver-ref quoted", "", "oracle", "jdbc:oracle:thin:@h:1521:orcl", "", false, "oracle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uuid := "aaaaaaaa-0000-0000-0000-000000000001"
			shared := sharedSourceXML(uuid, "s", tt.driverRef, tt.jdbcURL, false)
			var local string
			if tt.dbms != "" {
				local = localSourceXML(uuid, "s", tt.dbms, "")
			}
			dir := writeSyntheticProject(t, shared, local)
			ds := oneSource(t, dir)
			kind, ok, label := mapEngine(ds)
			if kind != tt.wantKind || ok != tt.wantOK || label != tt.wantLabel {
				t.Errorf("mapEngine(%+v) = (%q, %v, %q), want (%q, %v, %q)",
					ds, kind, ok, label, tt.wantKind, tt.wantOK, tt.wantLabel)
			}
		})
	}
}

// TestSQLiteMacroExpansion is case 6: a $PROJECT_DIR$ macro expands against the picked folder and
// is absolute; a bare relative path is a skip.
func TestSQLiteMacroExpansion(t *testing.T) {
	uuid := "aaaaaaaa-0000-0000-0000-000000000002"
	dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", "sqlite.xerial", "jdbc:sqlite:$PROJECT_DIR$/db/app.sqlite", false), "")
	ds := oneSource(t, dir)
	fields, ok, skipCode, _ := resolveFields(ds, dir)
	if !ok {
		t.Fatalf("resolveFields failed: skipCode=%s", skipCode)
	}
	want := filepath.Join(dir, "db", "app.sqlite")
	if fields.Database == nil || *fields.Database != want {
		t.Errorf("Database = %v, want %q", fields.Database, want)
	}

	dir2 := writeSyntheticProject(t, sharedSourceXML(uuid, "s", "sqlite.xerial", "jdbc:sqlite:relative.db", false), "")
	ds2 := oneSource(t, dir2)
	_, ok2, skipCode2, _ := resolveFields(ds2, dir2)
	if ok2 || skipCode2 != ReasonSQLitePathNotAbsolute {
		t.Errorf("relative sqlite path: ok=%v skipCode=%q, want ok=false skipCode=%q", ok2, skipCode2, ReasonSQLitePathNotAbsolute)
	}
}

// TestSQLiteMacroExpansionWhenPickingIdeaDirDirectly: a $PROJECT_DIR$-relative
// SQLite path must resolve against the real project root even when the caller/picker pointed at the
// .idea folder itself rather than the project folder — never against .idea, which would put
// the resolved path a level too deep (e.g. "<project>/.idea/db.sqlite" instead of
// "<project>/db.sqlite").
func TestSQLiteMacroExpansionWhenPickingIdeaDirDirectly(t *testing.T) {
	uuid := "aaaaaaaa-0000-0000-0000-00000000000a"
	dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", "sqlite.xerial", "jdbc:sqlite:$PROJECT_DIR$/db/app.sqlite", false), "")

	preview, err := Scan(filepath.Join(dir, ".idea"))
	if err != nil {
		t.Fatalf("Scan(.idea dir): %v", err)
	}
	if len(preview.Rows) != 1 || !preview.Rows[0].Importable || preview.Rows[0].Database == nil {
		t.Fatalf("Scan(.idea dir) rows = %+v, want one importable row with a database path", preview.Rows)
	}
	got := *preview.Rows[0].Database
	want := filepath.Join(dir, "db", "app.sqlite")
	if got != want {
		t.Errorf("Database = %q, want %q", got, want)
	}
	if strings.Contains(got, ".idea") {
		t.Errorf("Database = %q contains .idea — $PROJECT_DIR$ resolved against the .idea folder itself", got)
	}
}

// TestPostgresDefaultPort is case 7.
func TestPostgresDefaultPort(t *testing.T) {
	uuid := "aaaaaaaa-0000-0000-0000-000000000003"
	dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", "postgresql", "jdbc:postgresql://host/db", false), "")
	ds := oneSource(t, dir)
	fields, ok, _, _ := resolveFields(ds, dir)
	if !ok {
		t.Fatalf("resolveFields failed")
	}
	if fields.Port == nil || *fields.Port != 5432 {
		t.Errorf("Port = %v, want 5432", fields.Port)
	}
}

// TestUnrepresentableURLs is case 8: mongodb+srv and a comma-separated multi-host URL are both
// unrepresentable-url, never a half-import.
func TestUnrepresentableURLs(t *testing.T) {
	tests := []struct {
		name    string
		jdbcURL string
	}{
		{"mongodb+srv", "mongodb+srv://cluster/db"},
		{"comma-separated multi-host", "jdbc:postgresql://host1:5432,host2:5432/db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uuid := "aaaaaaaa-0000-0000-0000-000000000004"
			driverRef := "postgresql"
			if strings.Contains(tt.jdbcURL, "mongodb") {
				driverRef = "mongo"
			}
			dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", driverRef, tt.jdbcURL, false), "")
			ds := oneSource(t, dir)
			_, ok, skipCode, _ := resolveFields(ds, dir)
			if ok || skipCode != ReasonUnrepresentableURL {
				t.Errorf("ok=%v skipCode=%q, want ok=false skipCode=%q", ok, skipCode, ReasonUnrepresentableURL)
			}
		})
	}
}

// TestConfiguredByURLPassword is case 9: a configured-by-url source with user:pw@ in the URL
// yields the password directly and badges from-url, with no credential-store lookup at all.
func TestConfiguredByURLPassword(t *testing.T) {
	uuid := "aaaaaaaa-0000-0000-0000-000000000005"
	dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", "postgresql", "jdbc:postgresql://scott:tiger@host:5432/db", true), "")
	ds := oneSource(t, dir)
	if !ds.ConfiguredByURL {
		t.Fatalf("ConfiguredByURL not parsed")
	}
	fields, ok, _, _ := resolveFields(ds, dir)
	if !ok {
		t.Fatalf("resolveFields failed")
	}
	if !fields.FromURL || fields.Password == nil || *fields.Password != "tiger" {
		t.Errorf("Password/FromURL = %v/%v, want tiger/true", fields.Password, fields.FromURL)
	}
	if fields.Username == nil || *fields.Username != "scott" {
		t.Errorf("Username = %v, want scott", fields.Username)
	}
}

// TestRedactURLCredentials covers redactURLCredentials's own rules directly.
func TestRedactURLCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no at sign", "jdbc:postgresql://host:5432/db", "jdbc:postgresql://host:5432/db"},
		{"user and password", "jdbc:postgresql://scott:tiger@host:5432/db", "jdbc:postgresql://REDACTED@host:5432/db"},
		{"bare user, no password", "jdbc:postgresql://scott@host:5432/db", "jdbc:postgresql://REDACTED@host:5432/db"},
		{"mongodb+srv scheme", "mongodb+srv://scott:tiger@cluster.example/db", "mongodb+srv://REDACTED@cluster.example/db"},
		{"no scheme at all", "scott:tiger@host:5432/db", "REDACTED@host:5432/db"},
		{"at sign only in path, not userinfo", "jdbc:sqlite:/db/user@host.sqlite", "jdbc:sqlite:/db/user@host.sqlite"},
		// F6 (P108 Part 3): a raw '/' in the password, once "://" itself was found, used to fall
		// through to returning raw unredacted — reproduced with the exact multi-host shape a real
		// JDBC URL can carry.
		{"raw slash in password, multi-host authority", "jdbc:postgresql://u:pa/ss@h1,h2/db", "jdbc:postgresql://REDACTED@h1,h2/db"},
		{"raw question mark in password", "jdbc:postgresql://u:pa?ss@h/db", "jdbc:postgresql://REDACTED@h/db"},
		{"raw hash in password", "jdbc:postgresql://u:pa#ss@h/db", "jdbc:postgresql://REDACTED@h/db"},
		// F6: query-string credentials were never masked at all before this fix.
		{"password in query string", "jdbc:postgresql://h/db?user=u&password=secret", "jdbc:postgresql://h/db?user=u&password=REDACTED"},
		{"token in query string, case-insensitive key", "jdbc:mongodb://h/db?apiToken=abc123", "jdbc:mongodb://h/db?apiToken=REDACTED"},
		{"non-credential query param untouched", "jdbc:postgresql://h/db?sslmode=require", "jdbc:postgresql://h/db?sslmode=require"},
		{"both userinfo and query credentials", "jdbc:postgresql://u:pa/ss@h/db?token=xyz", "jdbc:postgresql://REDACTED@h/db?token=REDACTED"},
		{"query credential before a fragment", "jdbc:postgresql://h/db?pwd=secret#frag", "jdbc:postgresql://h/db?pwd=REDACTED#frag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURLCredentials(tt.in); got != tt.want {
				t.Errorf("redactURLCredentials(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSkipDetailNeverLeaksURLCredentials: a JDBC URL's own embedded
// "user:password@host" userinfo (the configured-by-url exception, or any other URL DataGrip wrote
// credentials into) must never appear verbatim in a skip/error string that crosses the bridge —
// the "a password never crosses the bridge" guarantee applies to skip/error text exactly as much
// as to a successfully-resolved row's own fields.
func TestSkipDetailNeverLeaksURLCredentials(t *testing.T) {
	const secret = "hunter2secret"
	tests := []struct {
		name      string
		driverRef string
		jdbcURL   string
	}{
		{"configured-by-url mongodb+srv is unrepresentable", "mongo", "mongodb+srv://scott:" + secret + "@cluster.example/db"},
		{"multi-host url with credentials is unrepresentable", "postgresql", "jdbc:postgresql://scott:" + secret + "@host1:5432,host2:5432/db"},
		{"unsupported engine with no other signal quotes the url", "", "unknownscheme://scott:" + secret + "@host/db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uuid := "aaaaaaaa-0000-0000-0000-0000000000f1"
			dir := writeSyntheticProject(t, sharedSourceXML(uuid, "s", tt.driverRef, tt.jdbcURL, true), "")
			ds := oneSource(t, dir)
			_, ok, skipCode, skipDetail := resolveFields(ds, dir)
			if ok {
				t.Fatalf("resolveFields(%+v) succeeded, want a skip", ds)
			}
			if strings.Contains(skipDetail, secret) {
				t.Errorf("skipCode=%q skipDetail=%q leaks the URL's password", skipCode, skipDetail)
			}

			// Full round trip through Scan/Apply — what actually crosses the bridge as
			// PreviewRow.SkipDetail / ReportRow.Error.
			preview, err := Scan(dir)
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			for _, row := range preview.Rows {
				if strings.Contains(row.SkipDetail, secret) {
					t.Errorf("PreviewRow.SkipDetail leaks the password: %q", row.SkipDetail)
				}
			}
			report, err := Apply(dir, []string{uuid}, true, &recordingCreator{})
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			for _, row := range report.Rows {
				if strings.Contains(row.Error, secret) {
					t.Errorf("ReportRow.Error leaks the password: %q", row.Error)
				}
			}
		})
	}
}

// TestNameTruncation is case 10: a 400-character name truncates to 120 and is reported.
func TestNameTruncation(t *testing.T) {
	longName := strings.Repeat("a", 400)
	name, truncated := truncateName(longName)
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
	if len(name) != 120 {
		t.Errorf("len(name) = %d, want 120", len(name))
	}
}

// TestSSHTunnelDropped is case 11: a source with <ssh-properties><enabled>true still imports
// (Scan marks it importable) and carries the ssh-tunnel-dropped warning.
func TestSSHTunnelDropped(t *testing.T) {
	uuid := "aaaaaaaa-0000-0000-0000-000000000006"
	shared := sharedSourceXML(uuid, "s", "postgresql", "jdbc:postgresql://host:5432/db", false)
	local := localSourceXML(uuid, "s", "", "true")
	dir := writeSyntheticProject(t, shared, local)
	ds := oneSource(t, dir)
	if !ds.SSHEnabled {
		t.Fatalf("SSHEnabled not parsed")
	}
	preview, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(preview.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(preview.Rows))
	}
	row := preview.Rows[0]
	if !row.Importable {
		t.Fatalf("row not importable: %+v", row)
	}
	found := false
	for _, w := range row.Warnings {
		if w == WarnSSHTunnelDropped {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %v, want to contain %q", row.Warnings, WarnSSHTunnelDropped)
	}
}
