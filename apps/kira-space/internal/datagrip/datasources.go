package datagrip

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxProjectFileBytes bounds the read of each project XML file — copied from postman.Parse's own
// reasoning (parse.go's maxCollectionBytes): a user-chosen file read whole into memory should have
// a ceiling well above anything real, so a pathological file fails loudly instead of by OOM. A
// DataGrip project file is a few KB in the real world; 8 MiB is generous.
const maxProjectFileBytes = 8 << 20

// DataSource is one <data-source> correlated across dataSources.xml (F1, always present) and
// dataSources.local.xml (F2, optional — .gitignore'd in most real projects).
type DataSource struct {
	UUID            string
	Name            string
	DriverRef       string
	JDBCURL         string
	ConfiguredByURL bool

	// From dataSources.local.xml, by uuid (F2). HasLocal is false when the local file is absent
	// or has no entry for this uuid — the zero values below must never be mistaken for "DataGrip
	// says there is no username"/"DataGrip says do not save the password" in that case.
	HasLocal      bool
	Username      string
	SecretStorage string // "master_key" | "memory" | "forget" | "" (absent/unrecognised)
	DBMS          string // <database-info dbms="…">, e.g. "POSTGRES"; "" if not yet connected once
	SSHEnabled    bool
}

// Project is one Scan's worth of parsed input — every data source the shared file names, in
// document order, correlated with whatever the local file (if any) says about it. CreatedIn is
// the local file's own <component created-in="…">, used to rank candidate IDE config directories;
// "" when the local file is absent.
type Project struct {
	Dir         string
	DataSources []DataSource
	CreatedIn   string
}

// xmlSharedDoc / xmlLocalDoc each match both real root shapes: <project><component
// name="DataSourceManagerImpl">… and <application><component name="dataSourceStorage">… wrap an
// identical <component> directly under the document root, so a struct keyed on the root's
// children (not the root's own element name) parses either one without a second code path. The
// shared and local files get their own struct trees — encoding/xml refuses two fields on one
// struct both tagged `xml:"data-source"` ("conflicts with field") — rather than one shared
// xmlComponent trying to decode both shapes of <data-source> from the same elements.
type xmlSharedDoc struct {
	Components []xmlSharedComponent `xml:"component"`
}

type xmlSharedComponent struct {
	Name        string            `xml:"name,attr"`
	DataSources []xmlSharedSource `xml:"data-source"`
}

// xmlSharedSource is one <data-source> in dataSources.xml (F1).
type xmlSharedSource struct {
	Name            string `xml:"name,attr"`
	UUID            string `xml:"uuid,attr"`
	DriverRef       string `xml:"driver-ref"`
	JDBCURL         string `xml:"jdbc-url"`
	ConfiguredByURL bool   `xml:"configured-by-url"`
}

type xmlLocalDoc struct {
	Components []xmlLocalComponent `xml:"component"`
}

type xmlLocalComponent struct {
	Name        string           `xml:"name,attr"`
	CreatedIn   string           `xml:"created-in,attr"`
	DataSources []xmlLocalSource `xml:"data-source"`
}

// xmlLocalSource is one <data-source> in dataSources.local.xml (F2).
type xmlLocalSource struct {
	Name          string `xml:"name,attr"`
	UUID          string `xml:"uuid,attr"`
	UserName      string `xml:"user-name"`
	SecretStorage string `xml:"secret-storage"`
	DatabaseInfo  struct {
		DBMS string `xml:"dbms,attr"`
	} `xml:"database-info"`
	SSHProperties *struct {
		Enabled bool `xml:"enabled"`
	} `xml:"ssh-properties"`
}

// ParseProject reads and correlates a DataGrip (or other JetBrains IDE) project's two data-source
// files. dir may be the project folder (containing .idea/dataSources.xml) or the .idea folder
// itself (D13: both are accepted, since users routinely navigate one level too deep).
func ParseProject(dir string) (*Project, error) {
	ideaDir, err := resolveIdeaDir(dir)
	if err != nil {
		return nil, err
	}

	shared, err := readSharedDoc(ideaDir)
	if err != nil {
		return nil, err
	}

	localByUUID, createdIn, err := readLocalDoc(ideaDir)
	if err != nil {
		return nil, err
	}

	// Dir is always ideaDir's own parent, never the raw dir the caller passed in: resolveIdeaDir
	// above already normalized a .idea-direct pick down to the real .idea folder, so its parent is
	// always the actual project root regardless of which of the two the caller/picker handed in.
	// Getting this wrong would make expandMacros (jdbc.go) resolve $PROJECT_DIR$/$ProjectFileDir$
	// to the .idea folder itself for a .idea-direct pick — e.g. "<project>/.idea/db.sqlite" instead
	// of "<project>/db.sqlite" for a relative SQLite path.
	project := &Project{Dir: filepath.Dir(ideaDir), CreatedIn: createdIn}
	project.DataSources = dataSourcesFrom(shared, localByUUID)
	return project, nil
}

// readSharedDoc reads and parses dataSources.xml (F1, always required).
func readSharedDoc(ideaDir string) (xmlSharedDoc, error) {
	sharedData, err := readBoundedXML(filepath.Join(ideaDir, "dataSources.xml"))
	if err != nil {
		return xmlSharedDoc{}, err
	}
	if sharedData == nil {
		return xmlSharedDoc{}, fmt.Errorf("datagrip: %s has no dataSources.xml", ideaDir)
	}
	var shared xmlSharedDoc
	if err := xml.Unmarshal(sharedData, &shared); err != nil {
		return xmlSharedDoc{}, fmt.Errorf("datagrip: dataSources.xml is not valid XML: %w", err)
	}
	return shared, nil
}

// readLocalDoc reads and parses dataSources.local.xml (F2, optional), returning the per-uuid
// correlation map and the project's own createdIn — both zero-valued (an empty, non-nil map and
// "") when the file is absent.
func readLocalDoc(ideaDir string) (map[string]xmlLocalSource, string, error) {
	localData, err := readBoundedXML(filepath.Join(ideaDir, "dataSources.local.xml"))
	if err != nil {
		return nil, "", err
	}
	localByUUID := map[string]xmlLocalSource{}
	if localData == nil {
		return localByUUID, "", nil
	}
	var local xmlLocalDoc
	if err := xml.Unmarshal(localData, &local); err != nil {
		return nil, "", fmt.Errorf("datagrip: dataSources.local.xml is not valid XML: %w", err)
	}
	createdIn := ""
	for _, c := range local.Components {
		if c.Name != "dataSourceStorageLocal" {
			continue
		}
		if c.CreatedIn != "" {
			createdIn = c.CreatedIn
		}
		for _, ds := range c.DataSources {
			if ds.UUID != "" {
				localByUUID[ds.UUID] = ds
			}
		}
	}
	return localByUUID, createdIn, nil
}

// dataSourcesFrom correlates shared's own <data-source> entries, in document order, against
// localByUUID (F2's per-uuid overlay).
func dataSourcesFrom(shared xmlSharedDoc, localByUUID map[string]xmlLocalSource) []DataSource {
	var out []DataSource
	for _, c := range shared.Components {
		if c.Name != "DataSourceManagerImpl" && c.Name != "dataSourceStorage" {
			continue
		}
		for _, ds := range c.DataSources {
			one := DataSource{
				UUID: ds.UUID, Name: ds.Name, DriverRef: ds.DriverRef,
				JDBCURL: ds.JDBCURL, ConfiguredByURL: ds.ConfiguredByURL,
			}
			if l, ok := localByUUID[ds.UUID]; ok {
				one.HasLocal = true
				one.Username = l.UserName
				one.SecretStorage = l.SecretStorage
				one.DBMS = l.DatabaseInfo.DBMS
				one.SSHEnabled = l.SSHProperties != nil && l.SSHProperties.Enabled
			}
			out = append(out, one)
		}
	}
	return out
}

// resolveIdeaDir implements D13's "accepts either a project folder or its .idea folder directly".
func resolveIdeaDir(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("datagrip: %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("datagrip: %s is not a directory", dir)
	}
	if filepath.Base(dir) == ".idea" {
		if _, err := os.Stat(filepath.Join(dir, "dataSources.xml")); err == nil {
			return dir, nil
		}
	}
	ideaDir := filepath.Join(dir, ".idea")
	if _, err := os.Stat(filepath.Join(ideaDir, "dataSources.xml")); err != nil {
		return "", fmt.Errorf("datagrip: no .idea/dataSources.xml found under %s", dir)
	}
	return ideaDir, nil
}

// readBoundedXML returns (nil, nil) when path does not exist (F2: dataSources.local.xml is
// routinely .gitignore'd and legitimately absent) — any other read failure is a real error. The
// bytes are returned unparsed so the two callers can unmarshal into their own root shape.
func readBoundedXML(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("datagrip: %w", err)
	}
	defer f.Close()

	limited := io.LimitReader(f, maxProjectFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("datagrip: read %s: %w", path, err)
	}
	if len(data) > maxProjectFileBytes {
		return nil, fmt.Errorf("datagrip: %s is larger than %d bytes", path, maxProjectFileBytes)
	}
	return data, nil
}

// productPrefixFromCreatedIn extracts the build-code prefix DataGrip/IntelliJ writes into
// created-in (F2), e.g. "DB" from "DB-243.22562.220" — used by D5 to rank config directories.
func productPrefixFromCreatedIn(createdIn string) string {
	code, _, _ := strings.Cut(createdIn, "-")
	return code
}
