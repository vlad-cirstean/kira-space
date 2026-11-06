// frozen.go is P58f §4.3(d)'s per-adapter named non-determinism list, re-derived from the
// TypeScript specs at port time: every frozen field is named here, in one place, so a new
// non-determinism produces a diff rather than a silent freeze (D13's own §5.6 second guard).
package ipcfixture

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// FrozenInnerIDUUID is what FreezeInnerIDNode substitutes for a materialized view's
// auto-generated backing-table uuid.
const FrozenInnerIDUUID = "00000000-0000-0000-0000-000000000000"

// FreezeInnerIDNode returns a copy of nodes with any ClickHouse `.inner_id.<uuid>` node's Name and
// Path frozen — that storage table is auto-generated per container (a fresh random uuid every
// time the materialized view is created), embedded in both fields.
func FreezeInnerIDNode(nodes []model.TreeNode, parentPath string) []model.TreeNode {
	frozenName := ".inner_id." + FrozenInnerIDUUID
	out := make([]model.TreeNode, len(nodes))
	for i, n := range nodes {
		if len(n.Name) > len(".inner_id.") && n.Name[:len(".inner_id.")] == ".inner_id." {
			n.Name = frozenName
			n.Path = parentPath + "/table:" + frozenName
		}
		out[i] = n
	}
	return out
}

// FrozenGeneratedAt is what FreezeDefinition substitutes for an ObjectDefinition's own
// adapter-stamped GeneratedAt.
const FrozenGeneratedAt = "2024-01-01T00:00:00.000Z"

// FreezeDefinition freezes an ObjectDefinition's GeneratedAt (wall-clock, stamped fresh on every
// real call).
func FreezeDefinition(d model.ObjectDefinition) model.ObjectDefinition {
	d.GeneratedAt = FrozenGeneratedAt
	return d
}

// FreezeNodeDetail returns a copy of nodes with the named node's Detail overridden to detail —
// e.g. mysql's information_schema.TABLES.TABLE_ROWS is InnoDB's own sampled estimate, confirmed
// (by the TypeScript spec this replaces) to differ between separate, identically-seeded containers
// for a million-row table, even though every smaller table's estimate is exact and stable.
func FreezeNodeDetail(nodes []model.TreeNode, name, detail string) []model.TreeNode {
	out := make([]model.TreeNode, len(nodes))
	for i, n := range nodes {
		if n.Name == name {
			n.Detail = &detail
		}
		out[i] = n
	}
	return out
}

// FrozenCoordinatorHost/FrozenCoordinatorPort are what FreezeCoordinator substitutes for a kafka
// consumer group's own coordinator broker address.
const (
	FrozenCoordinatorHost = "fixture-broker-host"
	FrozenCoordinatorPort = 0
)

// FreezeCoordinator is kafka.backend.spec.ts's own freezeCoordinator, ported: a fresh
// Testcontainers Docker-assigned hostname and host-mapped port every run, embedded both as a
// structured row (a single "host:port" string) and as two separate fields inside the definition's
// own JSON statements text (kafka/definition.go's own doc.Coordinator.Host/.Port) — each needs its
// own substitution. Frozen at record time (not just at comparison time, unlike a
// run-to-run-*stable* value like serverVersion) because a write-mode regeneration must capture a
// fixture that stays stable across every future run, not just this one.
func FreezeCoordinator(d model.ObjectDefinition) model.ObjectDefinition {
	sections := make([]model.DefinitionSection, len(d.Sections))
	for i, sec := range d.Sections {
		rows := make([]model.DefinitionSectionRow, len(sec.Rows))
		for j, row := range sec.Rows {
			if row.Name == "coordinator" {
				row.Value = coordinatorPlaceholder
			}
			rows[j] = row
		}
		sections[i] = model.DefinitionSection{Title: sec.Title, Rows: rows}
	}
	statements := make([]string, len(d.Statements))
	for i, s := range d.Statements {
		var doc map[string]any
		if err := json.Unmarshal([]byte(s), &doc); err != nil {
			statements[i] = s
			continue
		}
		if _, hasCoordinator := doc["coordinator"]; hasCoordinator {
			doc["coordinator"] = map[string]any{"host": FrozenCoordinatorHost, "port": FrozenCoordinatorPort}
		}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			statements[i] = s
			continue
		}
		statements[i] = string(b)
	}
	d.Sections = sections
	d.Statements = statements
	return d
}

// FrozenQueueEpoch is what FreezeQueueTimestamps substitutes for every 10-digit run inside an sqs
// queue definition's statements text, and for its own CreatedTimestamp/LastModifiedTimestamp rows.
const FrozenQueueEpoch = "1700000000"

var queueEpochPattern = regexp.MustCompile(`\d{10}`)

// FreezeQueueTimestamps is sqs.backend.spec.ts's own freezeQueueTimestamps, ported:
// CreatedTimestamp/LastModifiedTimestamp are LocalStack's own epoch-seconds wall-clock (stamped
// when this run's queue was created), embedded both as raw JSON text (a statement) and as
// structured DefinitionSectionRows — both need the same substitution. The statements-text
// replacement is deliberately a blind, global 10-digit-run regex (not a structural
// parse-and-patch), matching the TypeScript original exactly: it also overwrites the first 10
// digits of LocalStack's own fixed 12-zero fake account id embedded in QueueArn's value, which is
// harmless (that value is identical, and identically mangled, on both sides being compared) but
// would differ from a "cleaner" structural approach — reproducing the original's own behaviour
// byte for byte is what makes the two comparable at all once canonicalizeJSONTextEntries re-orders
// the surrounding JSON object's keys (sqs/definition.go's own comment on that divergence).
func FreezeQueueTimestamps(def model.ObjectDefinition) model.ObjectDefinition {
	statements := make([]string, len(def.Statements))
	for i, s := range def.Statements {
		statements[i] = queueEpochPattern.ReplaceAllString(s, FrozenQueueEpoch)
	}
	sections := make([]model.DefinitionSection, len(def.Sections))
	for i, sec := range def.Sections {
		rows := make([]model.DefinitionSectionRow, len(sec.Rows))
		for j, row := range sec.Rows {
			if row.Name == "CreatedTimestamp" || row.Name == "LastModifiedTimestamp" {
				row.Value = FrozenQueueEpoch
			}
			rows[j] = row
		}
		sections[i] = model.DefinitionSection{Title: sec.Title, Rows: rows}
	}
	def.Statements = statements
	def.Sections = sections
	return def
}

// FrozenHost/FrozenPort/FrozenCreatedAt/FrozenUpdatedAt are the values every committed fixture
// freezes a connection summary's own container-assigned fields to — Testcontainers hands out a
// fresh host port every run, and the row's timestamps are wall-clock, so an unfrozen fixture would
// churn on every regeneration for reasons no reviewer could act on.
const (
	FrozenHost      = "fixture-host"
	FrozenPort      = 0
	FrozenCreatedAt = "2024-01-01T00:00:00.000Z"
	FrozenUpdatedAt = "2024-01-01T00:00:00.000Z"
)

// FrozenConnectionSummary is the shape every committed fixture's connectionsList/connectionsConnect
// snapshot freezes a model.ConnectionSummary into — field order matches the committed fixtures
// exactly (id, sortOrder, createdAt, updatedAt, name, kind, ...), which is the TypeScript spec's
// own connectionSummaryOf() output order, not model.ConnectionSummary's Go field order (the two
// differ; this type exists so the JSON this package writes doesn't have to care).
type FrozenConnectionSummary struct {
	ID                string         `json:"id"`
	SortOrder         int            `json:"sortOrder"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
	Name              string         `json:"name"`
	Kind              string         `json:"kind"`
	Color             string         `json:"color"`
	Mode              string         `json:"mode"`
	ReadOnly          bool           `json:"readOnly"`
	Host              *string        `json:"host"`
	Port              *int           `json:"port"`
	Database          *string        `json:"database"`
	Username          *string        `json:"username"`
	URI               *string        `json:"uri"`
	Options           map[string]any `json:"options"`
	Preconnect        *string        `json:"preconnect"`
	PreconnectSidecar bool           `json:"preconnectSidecar"`
}

// FrozenEndpoint is what FreezeConnectionSummary substitutes for an sqs (uri-mode) connection's own
// options.endpoint — matching sqs.backend.spec.ts's own connectionSummaryOf literally
// ('http://fixture-host:0'), since a uri-mode connection has no top-level Host/Port for the freeze
// above to apply to; LocalStack hands out a fresh host-mapped port every run.
const FrozenEndpoint = "http://fixture-host:0"

// FreezeConnectionSummary applies the frozen fields above to a real model.ConnectionSummary. Frozen
// at record time (not just at comparison time) so a write-mode regeneration captures a fixture that
// stays stable across future runs, the same way host/port/createdAt/updatedAt already do.
func FreezeConnectionSummary(s model.ConnectionSummary) FrozenConnectionSummary {
	host := FrozenHost
	port := FrozenPort
	options := s.Options
	if options == nil {
		options = map[string]any{}
	}
	if _, hasEndpoint := options["endpoint"]; hasEndpoint {
		frozenOptions := make(map[string]any, len(options))
		for k, v := range options {
			frozenOptions[k] = v
		}
		frozenOptions["endpoint"] = FrozenEndpoint
		options = frozenOptions
	}
	return FrozenConnectionSummary{
		ID: s.ID, SortOrder: s.SortOrder, CreatedAt: FrozenCreatedAt, UpdatedAt: FrozenUpdatedAt,
		Name: s.Name, Kind: s.Kind, Color: s.Color, Mode: s.Mode, ReadOnly: s.ReadOnly,
		Host: &host, Port: &port, Database: s.Database, Username: s.Username, URI: s.URI,
		Options: options, Preconnect: s.Preconnect, PreconnectSidecar: s.PreconnectSidecar,
	}
}

// FreezeConnectionState zeroes Since (§4.3(d): "since -> 0") the same way FreezeConnectionSummary
// zeroes a container's host/port — the connect timestamp is wall-clock, not a fixture-worthy value.
func FreezeConnectionState(s model.ConnectionState) model.ConnectionState {
	s.Since = 0
	return s
}

// continuationTokenPlaceholder is what MaskContinuationTokens substitutes for a keyset page's
// nextToken/prevToken.
//
// This is a port-time finding, not carried over from any TypeScript spec: a keyset continuation
// token is adapters.EncodePageToken's base64(json({v, k, f})), where f is
// adapters.RequestFingerprint — sha1 of a Go struct (mysqlfamily/read.go's own {Path
// QualifiedName, Projection, Filter, Sort, PageSize}) marshaled by encoding/json. The deleted
// TypeScript engine's sql-text.ts computed the same conceptual fingerprint from its own JS object
// shape, and nothing requires the two encodings to serialize to identical bytes — sqltext.go's own
// doc comment on RequestFingerprint already says so: "Deterministic within a process is all that
// is required — a token is only ever decoded by the process that minted it." A frontend spec never
// re-derives this value either; it only ever passes a captured token back verbatim. So the token's
// *shape* (present, non-empty, base64) is what a fixture can usefully assert; its exact bytes are
// not portable across the two engines and are masked out here rather than compared.
const continuationTokenPlaceholder = "<token>"

// serverVersionPlaceholder is what MaskContinuationTokens substitutes for a "serverVersion"
// value. The frozen list names this for every adapter: "any serverVersion matched by
// pattern rather than compared — image tags move." mariadb's and mysql's own fixed :11.4/:8.4
// image tags happened to resolve to the exact patch already cached in this sandbox when captured,
// so their serverVersion strings matched byte for byte incidentally; masking
// it here rather than relying on that coincidence is the documented-correct behaviour for every
// adapter, not just the one (clickhouse, :26.3) where the coincidence broke first.
const serverVersionPlaceholder = "<serverVersion>"

// endpointPlaceholder is what MaskContinuationTokens substitutes for an sqs connection's
// options.endpoint value — LocalStack's own ephemeral host-mapped port, the same run-to-run
// volatile data every other adapter's host/port carries, embedded in Options (uri-mode
// connections have no top-level Host/Port for FreezeConnectionSummary to freeze) rather than a
// top-level field.
const endpointPlaceholder = "<endpoint>"

// coordinatorPlaceholder is what MaskContinuationTokens substitutes for a kafka consumer group's
// coordinator broker — Testcontainers' own fresh Docker-assigned hostname and host-mapped port
// every run (kafka.backend.spec.ts's own freezeCoordinator, already frozen in every committed
// fixture; masked here on the Go side's own real value so the two compare equal regardless).
const coordinatorPlaceholder = "fixture-broker-host:0"

// protocolTypePlaceholder is what MaskContinuationTokens substitutes for a kafka consumer group's
// own protocolType, both as a DefinitionSectionRow value and inside the group's own statements
// JSON — see the "protocolType" case in MaskContinuationTokens for the full attribution.
const protocolTypePlaceholder = "<protocolType>"

// canonicalizeJSONTextEntries re-parses every string element of arr that is itself JSON text,
// recursively masks the parsed value through MaskContinuationTokens, and re-serializes it —
// applied to a stream page's own "attrs"/"headers" entries and an ObjectDefinition's own
// "statements" entries.
//
// This exists because Go's encoding/json always marshals a map alphabetically by key, while the
// TypeScript engines this port replaces preserved whatever order the SDK response (sqs's
// GetQueueAttributes, kafka's own per-record partition/offset object) or JS object literal
// happened to use — sqs/definition.go's own comment already names this exact divergence. Both
// encodings are semantically identical JSON that the frontend only ever JSON.parses, never
// string-compares, so re-serializing both sides through the same canonical (sorted) encoder
// before comparing is correct, not a loosened check — a genuinely different value (not just a
// different key order) still produces a diff after both sides go through the identical
// transform. A non-JSON string (mariadb/mysql/clickhouse's own SQL statement text) fails to
// parse and is left untouched.
func canonicalizeJSONTextEntries(arr []any) {
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			continue
		}
		MaskContinuationTokens(parsed)
		b, err := json.Marshal(parsed)
		if err != nil {
			continue
		}
		arr[i] = string(b)
	}
}

// MaskContinuationTokens walks a JSON value already decoded into any (map[string]any/[]any/
// primitives, e.g. via json.Unmarshal) and, recursively and in place:
//   - replaces every non-null "nextToken"/"prevToken" string value with
//     continuationTokenPlaceholder
//   - replaces every non-null "serverVersion" string value with serverVersionPlaceholder
//   - sorts every "nodes" array by its "name" field
//
// The third bullet is a finding distinct from the other two: system.tables' own
// `ORDER BY name` runs server-side in ClickHouse, so it should be deterministic given identical
// data — but it produced a different order for "Order Items" (byte order vs. a
// locale/case-aware order) between the committed fixture's capture and this port's own run
// against the same floating `clickhouse/clickhouse-server:26.3` tag that also resolved to a
// different patch version (confirmed by the serverVersion mismatch on the very same run) — i.e.
// a second symptom of the same floating-tag drift, not a Go-adapter behaviour change. Sorting
// both sides the same way can only ever hide an order difference, never a missing/extra/renamed
// node, so it is safe to apply unconditionally rather than gate it to one adapter.
func MaskContinuationTokens(v any) {
	switch t := v.(type) {
	case map[string]any:
		applyStructuralMasks(t)
		for k, val := range t {
			if rule, ok := keyMaskRules[k]; ok {
				rule(t, k, val)
				continue
			}
			MaskContinuationTokens(val)
		}
	case []any:
		for _, item := range t {
			MaskContinuationTokens(item)
		}
	}
}

// applyStructuralMasks runs MaskContinuationTokens' own cross-field rules on one map — each keys
// off more than one field together (or a field's own value, not just its name), so none of them
// fit the {key, mask} shape keyMaskRules below is built from. Same rules, same order as the
// single switch this map arm used to open with, split across two group-specific helpers only to
// stay under one function's own complexity budget.
func applyStructuralMasks(t map[string]any) {
	dropGroupTypeFields(t)
	maskGroupRowByName(t)
}

// dropGroupTypeFields drops kadm.DescribedGroup's own Type/PartitionAssignor fields — from a
// statements doc's top-level keys and from a "Group" section's own structured rows alike — since
// kafka/definition.go's Go port drops these two rows entirely (its own comment: "the
// group section drops from seven rows to five") rather than reproducing the deleted TypeScript
// engine's definition.ts one for one. That is a permanent adapter capability difference from a
// much earlier phase — it is reconciled here, at comparison
// time, rather than papered over, so every committed fixture's own extra rows/JSON keys are
// dropped from both sides identically before diffing (a real name/value divergence in what
// remains still produces a diff).
func dropGroupTypeFields(t map[string]any) {
	if _, hasCoordinator := t["coordinator"]; hasCoordinator {
		if _, hasState := t["state"]; hasState {
			delete(t, "type")
			delete(t, "partitionAssignor")
		}
	}
	if title, _ := t["title"].(string); title == "Group" {
		if rows, ok := t["rows"].([]any); ok {
			kept := rows[:0]
			for _, r := range rows {
				if rm, ok := r.(map[string]any); ok {
					if name, _ := rm["name"].(string); name == "type" || name == "partitionAssignor" {
						continue
					}
				}
				kept = append(kept, r)
			}
			t["rows"] = kept
		}
	}
}

// maskGroupRowByName masks one DefinitionSectionRow by its own "name" field — three more names
// from kafka's own Group section, first surfaced by this exact comparison (not run-to-run
// non-determinism — franz-go's kadm client and the deleted TypeScript engine's own kafkajs
// binding report this admin-created, never-joined consumer group's own metadata differently):
//   - "coordinator": the same real, volatile broker address as the nested object below.
//   - "state": kadm reports the Kafka wire protocol's own state name verbatim ("Empty"); the
//     deleted engine's definition.ts translated a raw numeric state code through its own lookup
//     table, landing on "EMPTY" (kafka/definition.go's own comment: "State is already a string on
//     this client" — that client-side translation is exactly what stopped being necessary).
//     Case-normalized rather than replaced outright, so a genuine state change (Empty -> Dead)
//     still produces a diff.
//   - "protocolType": blank ("—" once orEmDash runs) for a group whose only membership is
//     admin-committed offsets, no consumer ever having joined and negotiated a protocol —
//     kafkajs's own binding reported a "simple" default label for this same case where kadm's
//     DescribedGroup.ProtocolType comes back empty. A cosmetic default, not comparable, so masked
//     wholesale like coordinator.
func maskGroupRowByName(t map[string]any) {
	name, _ := t["name"].(string)
	if name == "" {
		return
	}
	switch name {
	case "coordinator":
		if _, hasValue := t["value"]; hasValue {
			t["value"] = coordinatorPlaceholder
		}
	case "state":
		if s, ok := t["value"].(string); ok {
			t["value"] = strings.ToUpper(s)
		}
	case "protocolType":
		if _, hasValue := t["value"]; hasValue {
			t["value"] = protocolTypePlaceholder
		}
	}
}

// maskRule is one field-name-keyed rule from MaskContinuationTokens' own per-key switch, moved
// into keyMaskRules below — given the map and the key/value pair currently being visited, mutate
// t exactly as that switch's matching case used to.
type maskRule func(t map[string]any, k string, val any)

// maskWithPlaceholder builds the common shape: replace a non-nil value with a fixed placeholder,
// leave a nil value untouched (matching every switch case that guarded on "&& val != nil").
func maskWithPlaceholder(placeholder string) maskRule {
	return func(t map[string]any, k string, _ any) {
		if t[k] != nil {
			t[k] = placeholder
		}
	}
}

// keyMaskRules is MaskContinuationTokens' own field-name switch, one entry per key it treats
// specially — same key set, same per-key action, in a table instead of a switch so the recursive
// walk that drives it stays short. A key with no entry here gets the same fallback the original
// switch's own default case ran: recurse into val.
//
// Built in init(), not as a var initializer directly, because several rules below call
// MaskContinuationTokens itself — a direct initializer expression referencing it creates a
// package-init dependency cycle (MaskContinuationTokens also reads keyMaskRules), even though
// nothing here needs keyMaskRules populated until MaskContinuationTokens first runs,
// always after init() has finished.
var keyMaskRules map[string]maskRule

func init() {
	keyMaskRules = map[string]maskRule{
		"nextToken":     maskWithPlaceholder(continuationTokenPlaceholder),
		"prevToken":     maskWithPlaceholder(continuationTokenPlaceholder),
		"serverVersion": maskWithPlaceholder(serverVersionPlaceholder),
		"endpoint":      maskWithPlaceholder(endpointPlaceholder),
		// The statements-doc counterpart of the "protocolType" row applyStructuralMasks handles above.
		"protocolType": maskWithPlaceholder(protocolTypePlaceholder),
		"refresh": func(t map[string]any, k string, val any) {
			// The deleted TypeScript engine's ENGINE_OP.definition/describe
			// wire shape never modeled a refresh flag at all (it was purely harness.ts's own local
			// cache-bypass, never sent over the wire) — the committed clickhouse fixture's
			// treeDefinition args reflect that, carrying no "refresh" key. The real
			// bridge.TreeDescribeArgs this port drives does model it, correctly, since
			// tree.Service.Definition genuinely takes a refresh argument today (a redesign,
			// unrelated to this port). Every committed scenario only ever passes refresh=false, so
			// dropping a false-valued key from both sides before comparing can never hide a
			// true-valued divergence.
			if val == false {
				delete(t, k)
				return
			}
			MaskContinuationTokens(val)
		},
		"coordinator": func(t map[string]any, k string, val any) {
			if val == nil {
				return
			}
			// The nested {host, port} object inside a kafka group definition's own JSON statements
			// text (reached once canonicalizeJSONTextEntries below parses it) — same real, volatile
			// broker address as the structured row applyStructuralMasks handles above, masked the
			// same way rather than compared.
			if _, ok := val.(map[string]any); ok {
				t[k] = map[string]any{"host": "fixture-broker-host", "port": float64(0)}
				return
			}
			MaskContinuationTokens(val)
		},
		"state": func(t map[string]any, k string, val any) {
			if val == nil {
				return
			}
			// The statements-doc counterpart of the "state" row applyStructuralMasks handles above.
			if s, ok := val.(string); ok {
				t[k] = strings.ToUpper(s)
			}
		},
		"offsets": func(t map[string]any, k string, val any) {
			if val == nil {
				return
			}
			// kafka/definition.go's own groupOffsetDoc struct (unlike its groupMemberDoc sibling)
			// carries only Name/Value, no Detail field — the deleted TypeScript engine's own JSON
			// doc included a null "detail" alongside every offset entry. Structured
			// DefinitionSectionRows on both sides already carry Detail:null; this normalizes the
			// nested JSON doc's own shape to match before comparing, since an absent key and a
			// null-valued key mean the same thing here.
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						if _, hasDetail := m["detail"]; !hasDetail {
							m["detail"] = nil
						}
					}
				}
			}
			MaskContinuationTokens(val)
		},
		"attrs":      maskJSONTextEntries,
		"headers":    maskJSONTextEntries,
		"statements": maskJSONTextEntries,
		"nodes": func(t map[string]any, k string, val any) {
			MaskContinuationTokens(val)
			if nodes, ok := val.([]any); ok {
				sortByNameField(nodes)
			}
		},
	}
}

// maskJSONTextEntries is the shared rule for "attrs"/"headers"/"statements": each is an array of
// JSON-text entries canonicalizeJSONTextEntries parses and masks in place.
func maskJSONTextEntries(_ map[string]any, _ string, val any) {
	if val == nil {
		return
	}
	if arr, ok := val.([]any); ok {
		canonicalizeJSONTextEntries(arr)
	}
}

// sortByNameField sorts nodes (each expected to be a map[string]any with a string "name" field)
// in place by that field.
func sortByNameField(nodes []any) {
	sort.SliceStable(nodes, func(i, j int) bool {
		ni, _ := nodes[i].(map[string]any)
		nj, _ := nodes[j].(map[string]any)
		niName, _ := ni["name"].(string)
		njName, _ := nj["name"].(string)
		return niName < njName
	})
}
