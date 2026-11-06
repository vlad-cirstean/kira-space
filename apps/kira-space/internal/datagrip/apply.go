package datagrip

import (
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// CreatorInput mirrors connections.Input's exact shape (this package never imports
// internal/connections, so it cannot reference that type directly — the bridge service adapts this
// struct into the real connections.Input when it wires the real *connections.Service in as a
// Creator, a one-line, field-for-field conversion).
type CreatorInput struct {
	model.ConnectionFields
	Password *string
}

// Creator is Apply's one-method seam onto connections.Service.Create — the same per-consumer
// interface discipline connections.Backend and tree.Backend already follow (D3).
type Creator interface {
	Create(CreatorInput) (model.ConnectionSummary, error)
}

// ReportRow is one row of D9's Report — Error is a D11 reason code on a password-only failure, or
// a Creator error's message when the row's Create call itself failed. JSON tags: this struct
// crosses the bridge verbatim as DataGripService.Import's return value.
type ReportRow struct {
	UUID             string `json:"uuid"`
	Name             string `json:"name"`
	Created          bool   `json:"created"`
	PasswordImported bool   `json:"passwordImported"`
	Error            string `json:"error,omitempty"`
}

// Report is what Apply answers with (D9). It never carries a password.
type Report struct {
	Rows []ReportRow `json:"rows"`
}

// Apply is the second step. It re-parses the project (Scan's own Preview is never trusted as
// still current — the files could have changed, and Preview never carried a password anyway),
// then for each selected uuid resolves fields, resolves the password exactly once, and calls
// creator.Create. The plaintext exists only inside this call's stack: it is never written into
// Report, a log line, or any bridge payload. Each row is independent: a failure on
// one row never rolls back another, and the report says per row what happened.
func Apply(projectDir string, selectedUUIDs []string, secretsAvailable bool, creator Creator) (Report, error) {
	project, err := ParseProject(projectDir)
	if err != nil {
		return Report{}, err
	}
	selected := make(map[string]bool, len(selectedUUIDs))
	for _, id := range selectedUUIDs {
		selected[id] = true
	}

	// security.xml's PROVIDER is still read here (never for a lookup this package no longer makes,
	// only so a KEEPASS-configured project gets ReasonCredentialStoreUnsupported instead of a bare
	// "not found" — see lookupPassword).
	candidates := discoverCandidates(project.CreatedIn)
	var cfg SecurityConfig
	if len(candidates) > 0 {
		cfg = ReadSecurityXML(candidates[0].Dir)
	}

	report := Report{Rows: []ReportRow{}}
	for _, ds := range project.DataSources {
		if !selected[ds.UUID] {
			continue
		}
		// project.Dir (not the projectDir parameter): see Scan's identical comment in scan.go.
		report.Rows = append(report.Rows, applyOne(ds, project.Dir, cfg, secretsAvailable, creator))
	}
	return report, nil
}

func applyOne(ds DataSource, projectDir string, cfg SecurityConfig, secretsAvailable bool, creator Creator) ReportRow {
	name, _ := truncateName(ds.Name)
	row := ReportRow{UUID: ds.UUID, Name: name}

	fields, ok, skipCode, skipDetail := resolveFields(ds, projectDir)
	if !ok {
		row.Error = skipCode
		if skipDetail != "" {
			row.Error = fmt.Sprintf("%s: %s", skipCode, skipDetail)
		}
		return row
	}

	password := fields.Password
	passwordImported := fields.FromURL && password != nil

	if password == nil {
		switch {
		// Checked before ds.HasLocal below, and before any lookup: when this app's own secret
		// storage is unavailable, the final block below discards the password regardless of where
		// it came from, so a Keychain lookup here would only fire a real macOS authorization panel
		// (lookupPasswordFn -> readKeychainPassword) for a result nobody will ever see.
		case !secretsAvailable:
			row.Error = ReasonSecretStorageUnavailable
		case ds.HasLocal && (ds.SecretStorage == "memory" || ds.SecretStorage == "forget"):
			row.Error = ReasonPasswordNotSaved
		default:
			pw, un, lookupErr := lookupPasswordFn(ds, cfg)
			if lookupErr == nil {
				password = &pw
				passwordImported = true
				if fields.Username == nil && un != "" {
					fields.Username = &un
				}
			} else if re, isRefusal := lookupErr.(*RefusalError); isRefusal {
				row.Error = re.Code
			} else {
				row.Error = lookupErr.Error()
			}
		}
	}

	// D8: a Linux box with no KIRA_INSECURE_SECRETS set, or a Mac whose Keychain probe failed —
	// this app's own secret storage, not DataGrip's. Every selected row still creates, with no
	// password, rather than the whole import failing.
	if !secretsAvailable {
		password = nil
		passwordImported = false
		row.Error = ReasonSecretStorageUnavailable
	}

	color := kindAccent[fields.Kind]
	if color == "" {
		color = "none"
	}
	in := CreatorInput{
		ConnectionFields: model.ConnectionFields{
			Name: name, Kind: fields.Kind, Color: color, Mode: "fields",
			Host: fields.Host, Port: fields.Port, Database: fields.Database,
			Username: fields.Username, Options: map[string]any{},
			// The connection dialog's defaults; Create rejects an empty permission mode.
			McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny", McpMaskedMode: "inherit", McpAutoExplain: true,
			McpReview: model.McpReview{Read: "inherit", Write: "inherit", Ddl: "inherit", Masked: "inherit"}, McpStatementTimeoutSec: model.McpStatementTimeoutDefault,
		},
		Password: password,
	}
	if _, createErr := creator.Create(in); createErr != nil {
		row.Error = createErr.Error()
		return row
	}
	row.Created = true
	row.PasswordImported = passwordImported
	return row
}

// lookupPasswordFn is lookupPassword through an indirection tests can swap out (t.Cleanup-restored)
// to spy on or count calls — applyOne's only route to a real Keychain prompt, and the seam the
// review finding's "never invoked when secretsAvailable is false" regression test hangs off.
var lookupPasswordFn = lookupPassword

// lookupPassword is the (now single-backend) credential fetch for one data source, run once, at
// Apply time. cfg.Provider is checked directly rather than through LookupOrder, because
// KEEPASS and MEMORY_ONLY/DO_NOT_STORE both resolve to "no backend to try" but need different
// refusals: KEEPASS means a real store exists that this app cannot read (file-based
// PasswordSafe was scoped out), while MEMORY_ONLY/
// DO_NOT_STORE means DataGrip itself never saved anything.
func lookupPassword(ds DataSource, cfg SecurityConfig) (password, username string, err error) {
	if cfg.Provider == "KEEPASS" {
		return "", "", &RefusalError{
			Code: ReasonCredentialStoreUnsupported,
			Message: "DataGrip is configured to use its own local password store instead of the system " +
				"keychain, which Kira Space does not support",
		}
	}
	order := LookupOrder(cfg.Provider)
	if len(order) == 0 {
		return "", "", &RefusalError{
			Code:    ReasonPasswordNotSaved,
			Message: "DataGrip is configured not to save passwords",
		}
	}
	return readKeychainPassword(ds.UUID)
}
