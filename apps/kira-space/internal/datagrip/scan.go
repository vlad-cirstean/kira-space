package datagrip

// PasswordOutlook is D9's four-value classification a Scan can make with file reads alone, before
// any credential store is actually opened.
type PasswordOutlook string

const (
	// OutlookWillAttempt means Apply will try to fetch and decrypt a password for this row.
	OutlookWillAttempt PasswordOutlook = "will-attempt"
	// OutlookNotSaved means DataGrip itself never wrote a password for this row (secret-storage
	// memory/forget, or the resolved provider is MEMORY_ONLY/DO_NOT_STORE) — not a failure.
	OutlookNotSaved PasswordOutlook = "not-saved"
	// OutlookFromURL means D7's configured-by-url exception already supplied the password —
	// no credential store will be touched for this row at all.
	OutlookFromURL PasswordOutlook = "from-url"
	// OutlookStoreUnsupported means neither backend Apply would try is reachable on this
	// platform/configuration (D1) — file reads alone are enough to know this in advance.
	OutlookStoreUnsupported PasswordOutlook = "store-unsupported"
)

// PreviewRow is one line of D10's review dialog. JSON tags: this struct crosses the bridge
// verbatim as DataGripService.Scan's return value (no separate bridge-side mirror type — the same
// way connections.Service.List returns model.ConnectionSummary directly).
type PreviewRow struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	Importable bool   `json:"importable"`

	// Populated only when Importable.
	Kind            string          `json:"kind,omitempty"`
	Host            *string         `json:"host,omitempty"`
	Port            *int            `json:"port,omitempty"`
	Database        *string         `json:"database,omitempty"`
	Username        *string         `json:"username,omitempty"`
	PasswordOutlook PasswordOutlook `json:"passwordOutlook,omitempty"`

	// Populated only when !Importable — one of D11's four "no row at all" reasons.
	SkipReason string `json:"skipReason,omitempty"`
	SkipDetail string `json:"skipDetail,omitempty"`

	// Warnings that do not block import (name-truncated, ssh-tunnel-dropped).
	Warnings []string `json:"warnings"`
}

// Preview is one Scan's answer — D9: file reads only, no credential store touched (F11: touching
// the macOS Keychain here would double the authorization panels a real import shows).
type Preview struct {
	ProjectDir string       `json:"projectDir"`
	Rows       []PreviewRow `json:"rows"`
}

// Scan parses projectDir's two data-source files, maps engines and fields, locates the IntelliJ
// Platform config directory, and reads security.xml — all file reads, no keychain touched.
// The config-directory lookup exists only to read security.xml's PROVIDER, which lets a
// KEEPASS-configured project (the file-based PasswordSafe store this app used to
// also read is gone) get OutlookStoreUnsupported instead of a misleading OutlookWillAttempt.
func Scan(projectDir string) (*Preview, error) {
	project, err := ParseProject(projectDir)
	if err != nil {
		return nil, err
	}

	candidates := discoverCandidates(project.CreatedIn)
	var cfg SecurityConfig
	if len(candidates) > 0 {
		cfg = ReadSecurityXML(candidates[0].Dir)
	}

	// project.Dir (not the projectDir parameter): ParseProject already normalized a .idea-direct
	// pick down to the real project root (D13/datasources.go), and macro expansion below must
	// resolve against that root, not the .idea folder itself.
	preview := &Preview{ProjectDir: project.Dir, Rows: []PreviewRow{}}
	for _, ds := range project.DataSources {
		preview.Rows = append(preview.Rows, previewRowFor(ds, project.Dir, cfg))
	}
	return preview, nil
}

func discoverCandidates(createdIn string) []ConfigCandidate {
	root, err := jetBrainsConfigRoot()
	if err != nil {
		return nil
	}
	candidates, _ := DiscoverConfigDirs(root, createdIn)
	return candidates
}

func previewRowFor(ds DataSource, projectDir string, cfg SecurityConfig) PreviewRow {
	name, truncated := truncateName(ds.Name)
	row := PreviewRow{UUID: ds.UUID, Name: name, Warnings: []string{}}
	if truncated {
		row.Warnings = append(row.Warnings, WarnNameTruncated)
	}
	if ds.SSHEnabled {
		row.Warnings = append(row.Warnings, WarnSSHTunnelDropped)
	}

	fields, ok, skipCode, skipDetail := resolveFields(ds, projectDir)
	if !ok {
		row.SkipReason = skipCode
		row.SkipDetail = skipDetail
		return row
	}

	row.Importable = true
	row.Kind = fields.Kind
	row.Host = fields.Host
	row.Port = fields.Port
	row.Database = fields.Database
	row.Username = fields.Username
	row.PasswordOutlook = outlookFor(ds, fields, cfg)
	return row
}

// outlookFor is the classification — file reads and in-memory facts only. cfg.Provider ==
// "KEEPASS" is checked ahead of LookupOrder for the same reason lookupPassword checks it directly
// (apply.go): it is a real, readable-by-DataGrip store this app does not support, which is a
// different outlook than "DataGrip never saved a password" (OutlookNotSaved).
func outlookFor(ds DataSource, fields ResolvedFields, cfg SecurityConfig) PasswordOutlook {
	if fields.FromURL {
		return OutlookFromURL
	}
	if ds.HasLocal && (ds.SecretStorage == "memory" || ds.SecretStorage == "forget") {
		return OutlookNotSaved
	}
	if cfg.Provider == "MEMORY_ONLY" || cfg.Provider == "DO_NOT_STORE" {
		return OutlookNotSaved
	}
	if cfg.Provider == "KEEPASS" {
		return OutlookStoreUnsupported
	}

	order := LookupOrder(cfg.Provider)
	if len(order) == 0 {
		return OutlookNotSaved
	}
	for _, backend := range order {
		if backend == "keychain" && keychainSupported {
			return OutlookWillAttempt
		}
	}
	return OutlookStoreUnsupported
}
