package datagrip

import (
	"testing"
)

// findByUUID is a small test helper shared by this file and jdbc_test.go.
func findByUUID(sources []DataSource, uuid string) (DataSource, bool) {
	for _, s := range sources {
		if s.UUID == uuid {
			return s, true
		}
	}
	return DataSource{}, false
}

// TestParseProjectSixSourcesRoundTrip is §4.2 case 1: uuid, name, driver-ref, jdbc-url, and the
// local file's user-name/secret-storage/dbms all pull across correctly, correlated by uuid.
func TestParseProjectSixSourcesRoundTrip(t *testing.T) {
	project, err := ParseProject("testdata/project-six")
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	if len(project.DataSources) != 6 {
		t.Fatalf("got %d data sources, want 6", len(project.DataSources))
	}
	if project.CreatedIn != "DB-243.22562.220" {
		t.Errorf("CreatedIn = %q, want DB-243.22562.220", project.CreatedIn)
	}

	pg, ok := findByUUID(project.DataSources, "11111111-0000-0000-0000-000000000001")
	if !ok {
		t.Fatalf("pg-main not found")
	}
	if pg.Name != "pg-main" || pg.DriverRef != "postgresql" ||
		pg.JDBCURL != "jdbc:postgresql://pg.example.internal:5432/appdb" {
		t.Errorf("pg-main shared fields wrong: %+v", pg)
	}
	if !pg.HasLocal || pg.Username != "app" || pg.SecretStorage != "master_key" || pg.DBMS != "POSTGRES" {
		t.Errorf("pg-main local fields wrong: %+v", pg)
	}

	sqlite, ok := findByUUID(project.DataSources, "11111111-0000-0000-0000-000000000004")
	if !ok {
		t.Fatalf("app-coverage not found")
	}
	if sqlite.SecretStorage != "forget" {
		t.Errorf("app-coverage secret-storage = %q, want forget", sqlite.SecretStorage)
	}
}

// TestParseProjectAbsentLocalFile is case 2: dataSources.local.xml absent → every source still
// parses, with no username, an unknown secret-storage, and no error.
func TestParseProjectAbsentLocalFile(t *testing.T) {
	project, err := ParseProject("testdata/project-no-local")
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	if len(project.DataSources) != 6 {
		t.Fatalf("got %d data sources, want 6", len(project.DataSources))
	}
	for _, ds := range project.DataSources {
		if ds.HasLocal {
			t.Errorf("%s: HasLocal = true with no local file present", ds.Name)
		}
		if ds.Username != "" || ds.SecretStorage != "" {
			t.Errorf("%s: local-only fields leaked with no local file: %+v", ds.Name, ds)
		}
	}
}

// TestParseProjectIgnoresLocalOnlyEntry is case 3: "ghost" exists only in dataSources.local.xml
// (project-six's own fixture) and must never produce a phantom row.
func TestParseProjectIgnoresLocalOnlyEntry(t *testing.T) {
	project, err := ParseProject("testdata/project-six")
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	if _, ok := findByUUID(project.DataSources, "11111111-0000-0000-0000-00000000dead"); ok {
		t.Errorf("local-only entry 'ghost' produced a row")
	}
}

// TestParseProjectIDELevelRootShape is case 4: the <application><component
// name="dataSourceStorage"> root shape parses identically to the per-project one.
func TestParseProjectIDELevelRootShape(t *testing.T) {
	project, err := ParseProject("testdata/project-ide-level")
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	if len(project.DataSources) != 1 {
		t.Fatalf("got %d data sources, want 1", len(project.DataSources))
	}
	ds := project.DataSources[0]
	if ds.Name != "ide-level-pg" || ds.DriverRef != "postgresql" {
		t.Errorf("ide-level-pg parsed wrong: %+v", ds)
	}
}

// TestParseProjectAcceptsIdeaDirDirectly is D13: a caller may point at the .idea folder itself.
func TestParseProjectAcceptsIdeaDirDirectly(t *testing.T) {
	project, err := ParseProject("testdata/project-six/.idea")
	if err != nil {
		t.Fatalf("ParseProject(.idea dir): %v", err)
	}
	if len(project.DataSources) != 6 {
		t.Fatalf("got %d data sources, want 6", len(project.DataSources))
	}
}

// TestParseProjectDirResolvesToParentOfIdea: whichever of the project folder or its .idea folder
// the caller passed in, Project.Dir must always be the project's own root (the .idea folder's
// parent) — never the .idea folder itself — since jdbc.go's expandMacros resolves
// $PROJECT_DIR$/$ProjectFileDir$ against it directly.
func TestParseProjectDirResolvesToParentOfIdea(t *testing.T) {
	viaRoot, err := ParseProject("testdata/project-six")
	if err != nil {
		t.Fatalf("ParseProject(root): %v", err)
	}
	viaIdea, err := ParseProject("testdata/project-six/.idea")
	if err != nil {
		t.Fatalf("ParseProject(.idea dir): %v", err)
	}
	if viaRoot.Dir != "testdata/project-six" {
		t.Errorf("Dir via project root = %q, want %q", viaRoot.Dir, "testdata/project-six")
	}
	if viaIdea.Dir != viaRoot.Dir {
		t.Errorf("Dir via .idea pick = %q, want it to resolve to the same project root %q", viaIdea.Dir, viaRoot.Dir)
	}
}

// TestParseProjectRejectsFolderWithNoIdea is the negative case for D13's validation.
func TestParseProjectRejectsFolderWithNoIdea(t *testing.T) {
	if _, err := ParseProject(t.TempDir()); err == nil {
		t.Fatalf("expected an error for a folder with no .idea/dataSources.xml")
	}
}
