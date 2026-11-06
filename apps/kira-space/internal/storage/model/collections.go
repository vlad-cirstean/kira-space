package model

import "fmt"

// Collection is one api_collections row, minus origin_json — the summary the tree renders from.
// P4 D2: `List` never selects origin_json (or request_json), which is why neither is here.
type Collection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	// File is the collection file's name in the api folder, "" before the file is projected.
	File string `json:"file"`
}

// CollectionItem is one api_items row, minus request_json and origin_json. Method and URL are
// denormalized out of the saved request so the tree renders a method chip and searches URLs
// without reading potentially large bodies; both are "" for a folder. Protocol is
// structural sibling data, not a third Kind value — see the two protocol constants below.
type CollectionItem struct {
	ID           string  `json:"id"`
	CollectionID string  `json:"collectionId"`
	ParentID     *string `json:"parentId"`
	Kind         string  `json:"kind"`
	Name         string  `json:"name"`
	SortOrder    int     `json:"sortOrder"`
	Method       string  `json:"method"`
	URL          string  `json:"url"`
	Protocol     string  `json:"protocol"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

// P11 D12: the two protocols a request item's request_json can hold — 'http' -> SavedRequest,
// 'grpc' -> SavedGrpcRequest. Meaningless for a folder, which always reads back 'http' (the
// column's own SQL default) but is never consulted.
const (
	ItemProtocolHTTP = "http"
	ItemProtocolGrpc = "grpc"
)

// IsCollectionItemProtocol reports whether protocol is one of the two.
func IsCollectionItemProtocol(protocol string) bool {
	return protocol == ItemProtocolHTTP || protocol == ItemProtocolGrpc
}

// The two item kinds. Postman carries no `type` field — a folder is "has an `item` member", a
// request is "has a `request` member" (P4 F1) — so this vocabulary is this app's own.
const (
	CollectionItemFolder  = "folder"
	CollectionItemRequest = "request"
)

// IsCollectionItemKind reports whether kind is one of the two.
func IsCollectionItemKind(kind string) bool {
	return kind == CollectionItemFolder || kind == CollectionItemRequest
}

// model.SavedRequest is what a saved collection request *is* — deliberately field-identical
// to the request half of packages/shared/domain/http.ts's httpRequestTabStateSchema, so the
// renderer can spread it straight into tab state. It is NOT tabs.state_json (the four UI-only
// pane fields stay out — saving them would make scrolling a pane mark a request dirty) and it is
// NOT httpclient.Body (the wire body drops disabled rows, which is the one thing a saved request
// must keep).
//
// Method is a plain string, not a closed enum: Postman's own method list is 15 values plus any
// custom string, and a value this app's builder cannot show is coerced at exactly one
// renderer boundary, not silently rejected at the storage layer.
type SavedRequest struct {
	Method       string           `json:"method"`
	URL          string           `json:"url"`
	Headers      []SavedHeader    `json:"headers"`
	BodyMode     string           `json:"bodyMode"`
	Body         string           `json:"body"` // the `raw` mode's own buffer
	Code         string           `json:"code"` // the `code` mode's own buffer
	CodeLanguage string           `json:"codeLanguage"`
	URLEncoded   []SavedField     `json:"urlEncoded"`
	FormData     []SavedFormField `json:"formData"`
	BinaryFile   *SavedFile       `json:"binaryFile"`
	// ParamDescriptions is a query param's own description, keyed by name. A side-car
	// map rather than a row on a `params` array: there is no such array (the URL is the single
	// source of truth for the query string), so this is annotation only, never input to what
	// gets sent.
	ParamDescriptions map[string]string `json:"paramDescriptions"`
}

// SavedHeader is one header row. `enabled` has no wire counterpart — it exists so the builder can
// keep an unchecked row instead of deleting it, which is the whole point of the checkbox.
// Description is app-local free text, no wire counterpart, the same shape
// model/variables.go's own Description field already carries.
type SavedHeader struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

// SavedField is one urlencoded row.
type SavedField struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

// SavedFormField is one form-data row. Kind == "file" means Path is a local path (never bytes)
// and Value is ignored; FileName/FileSize let the builder render `report.csv (1.2 MB)` with no
// round trip back to disk.
type SavedFormField struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	Path        string `json:"path"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ContentType string `json:"contentType"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

// SavedFile is the binary (Postman `file`) body's one chosen file — path only, never bytes.
type SavedFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// savedBodyModes mirrors HTTP_BODY_MODES (packages/shared/domain/http.ts) and
// httpclient.validBodyModes. Kept here rather than imported so this package keeps its
// standard-library-only dependency set (internal/postman imports it, and a cycle through
// httpclient would be gratuitous).
var savedBodyModes = map[string]bool{
	"none": true, "raw": true, "code": true, "urlencoded": true, "formdata": true, "file": true,
}

// savedCodeLanguages mirrors CODE_LANGUAGES (packages/shared/domain/http.ts).
var savedCodeLanguages = map[string]bool{
	"javascript": true, "json": true, "html": true, "xml": true,
}

// Validate checks what SQL cannot. A row failing it is dropped and logged on read
// (repos/saved_queries.go's posture) and refused on write (repos/tabs.go's).
func (r SavedRequest) Validate() error {
	if r.Method == "" {
		return fmt.Errorf("model: saved request: method is required")
	}
	if !savedBodyModes[r.BodyMode] {
		return fmt.Errorf("model: saved request: unrecognised body mode %q", r.BodyMode)
	}
	if !savedCodeLanguages[r.CodeLanguage] {
		return fmt.Errorf("model: saved request: unrecognised code language %q", r.CodeLanguage)
	}
	return nil
}
