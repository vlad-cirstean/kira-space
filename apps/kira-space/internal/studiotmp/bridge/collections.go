package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// CollectionsService is the QueriesService shape (a typed-struct wrapper per method, with an
// explicit guard and an ipcerr translation). Reads go to Deps.Repos.Collections; every write is an
// edit of a collection file through Files, which projects the file back into the rows and pushes the
// change event itself. Import and Export are the two file methods.
//
// **Go reads and writes the file.** CHUNK_THRESHOLD is 512 KiB with a serial fetch per chunk and
// an outright refusal above a 64 MiB assembled body, so a 10-50 MB collection through the
// renderer would be 20-100 serial round trips and, above the ceiling, an unattributable
// "assembled body too large". The renderer's whole involvement is a path string from
// FilesService.ChooseOpen/ChooseSave — the identical shape established for a request body's file.
//
// **No op-log row and no new op kind.** The ring-and-Stop machinery ARCHITECTURE.md's ~150 ms
// invariant refers to is ViewChrome + useRunState(tabId), which is per tab; an import initiated
// from the left panel has no tab, so joining the op log would buy an Operations-panel row and
// nothing the user is looking at. Cancelling a single all-or-nothing local transaction has no
// useful semantics either. The panel's own header action carries a spinner instead, and Wails
// handles each bound call in its own goroutine, so a long import blocks that call and
// nothing else.
type CollectionsService struct {
	Deps  appcore.Deps
	Files *apifiles.Store
}

// CollectionSummary and ItemSummary are the two flat row shapes the tree renders from — the
// renderer builds the tree, mirroring how TreeService.Children returns flat nodes.
type CollectionSummary = model.Collection

// ItemSummary is model.CollectionItem under the name the renderer's own mirror uses.
type ItemSummary = model.CollectionItem

// CollectionsTree is one List() answer: flat arrays, one call per panel mount, no N+1.
type CollectionsTree struct {
	Collections []CollectionSummary `json:"collections"`
	Items       []ItemSummary       `json:"items"`
}

func (s *CollectionsService) List() (CollectionsTree, error) {
	collections, items, err := s.Deps.Repos.Collections.List()
	if err != nil {
		return CollectionsTree{}, ipcerr.InternalErr(err)
	}
	return CollectionsTree{Collections: collections, Items: items}, nil
}

type CollectionsItemArgs struct {
	ItemID string `json:"itemId"`
}

// readErr maps a saved-request read failure: a missing item is E_NOT_FOUND, a folder or
// other-protocol item is E_BAD_REQUEST, everything else (including corrupt stored JSON) E_INTERNAL.
func readErr[T any](v T, err error) (T, error) {
	var zero T
	switch {
	case err == nil:
		return v, nil
	case errors.Is(err, repos.ErrItemNotFound):
		return zero, ipcerr.NotFound(err.Error())
	case errors.Is(err, repos.ErrNotARequest):
		return zero, ipcerr.BadRequest(err.Error())
	}
	return zero, ipcerr.Internal(err.Error())
}

func (s *CollectionsService) GetRequest(args CollectionsItemArgs) (model.SavedRequest, error) {
	if args.ItemID == "" {
		return model.SavedRequest{}, ipcerr.Required("itemId")
	}
	return readErr(s.Deps.Repos.Collections.GetRequest(args.ItemID))
}

// GetGrpcRequest is GetRequest's own gRPC sibling (P11 D12).
func (s *CollectionsService) GetGrpcRequest(args CollectionsItemArgs) (model.SavedGrpcRequest, error) {
	if args.ItemID == "" {
		return model.SavedGrpcRequest{}, ipcerr.Required("itemId")
	}
	return readErr(s.Deps.Repos.Collections.GetGrpcRequest(args.ItemID))
}

type CollectionsSaveRequestArgs struct {
	ItemID  string             `json:"itemId"`
	Name    string             `json:"name"`
	Request model.SavedRequest `json:"request"`
}

// SaveRequest replaces the request's node in its collection file and answers with the item row the
// file now projects to.
func (s *CollectionsService) SaveRequest(args CollectionsSaveRequestArgs) (ItemSummary, error) {
	if args.ItemID == "" {
		return ItemSummary{}, ipcerr.Required("itemId")
	}
	if args.Name == "" {
		return ItemSummary{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	if err := args.Request.Validate(); err != nil {
		return ItemSummary{}, ipcerr.BadRequest(err.Error())
	}
	if err := s.Files.SaveRequest(args.ItemID, args.Name, apifiles.HTTPOf(args.Request)); err != nil {
		return ItemSummary{}, apiFilesErr(err)
	}
	return readErr(s.Deps.Repos.Collections.GetItem(args.ItemID))
}

type CollectionsSaveGrpcRequestArgs struct {
	ItemID  string                 `json:"itemId"`
	Name    string                 `json:"name"`
	Request model.SavedGrpcRequest `json:"request"`
}

// SaveGrpcRequest is SaveRequest's own gRPC sibling (P11 D12).
func (s *CollectionsService) SaveGrpcRequest(args CollectionsSaveGrpcRequestArgs) (ItemSummary, error) {
	if args.ItemID == "" {
		return ItemSummary{}, ipcerr.Required("itemId")
	}
	if args.Name == "" {
		return ItemSummary{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	if err := args.Request.Validate(); err != nil {
		return ItemSummary{}, ipcerr.BadRequest(err.Error())
	}
	if err := s.Files.SaveGrpcRequest(args.ItemID, args.Name, apifiles.GRPCOf(args.Request)); err != nil {
		return ItemSummary{}, apiFilesErr(err)
	}
	return readErr(s.Deps.Repos.Collections.GetItem(args.ItemID))
}

type CollectionsCreateCollectionArgs struct {
	Name string `json:"name"`
}

// CreateCollection writes a new empty collection file.
func (s *CollectionsService) CreateCollection(args CollectionsCreateCollectionArgs) (CollectionSummary, error) {
	if args.Name == "" {
		return CollectionSummary{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	id, err := s.Files.CreateCollection(args.Name)
	if err != nil {
		return CollectionSummary{}, apiFilesErr(err)
	}
	c, err := s.Deps.Repos.Collections.GetCollection(id)
	if err != nil {
		return CollectionSummary{}, ipcerr.InternalErr(err)
	}
	return c, nil
}

type CollectionsCreateItemArgs struct {
	CollectionID string              `json:"collectionId"`
	ParentID     *string             `json:"parentId"`
	Kind         string              `json:"kind"`
	Name         string              `json:"name"`
	Request      *model.SavedRequest `json:"request"`
}

// CreateItem appends a folder or a request to the collection file.
func (s *CollectionsService) CreateItem(args CollectionsCreateItemArgs) (ItemSummary, error) {
	if args.CollectionID == "" {
		return ItemSummary{}, ipcerr.Required("collectionId")
	}
	if !model.IsCollectionItemKind(args.Kind) {
		return ItemSummary{}, ipcerr.Contract("kind must be 'folder' or 'request'")
	}
	if args.Name == "" {
		return ItemSummary{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	n := apifiles.NewItem{Name: args.Name, Folder: args.Kind == model.CollectionItemFolder}
	if !n.Folder {
		req := model.SavedRequest{Method: "GET", Headers: []model.SavedHeader{}, BodyMode: "none", CodeLanguage: "json", ParamDescriptions: map[string]string{}}
		if args.Request != nil {
			req = *args.Request
		}
		if err := req.Validate(); err != nil {
			return ItemSummary{}, ipcerr.BadRequest(err.Error())
		}
		h := apifiles.HTTPOf(req)
		n.HTTP = &h
	}
	return s.createItem(args.CollectionID, args.ParentID, n)
}

type CollectionsCreateGrpcItemArgs struct {
	CollectionID string                  `json:"collectionId"`
	ParentID     *string                 `json:"parentId"`
	Name         string                  `json:"name"`
	Request      *model.SavedGrpcRequest `json:"request"`
}

// CreateGrpcItem is CreateItem's own gRPC sibling (P11 D12) — always a request, never a folder (a
// gRPC request has no folder distinction of its own).
func (s *CollectionsService) CreateGrpcItem(args CollectionsCreateGrpcItemArgs) (ItemSummary, error) {
	if args.CollectionID == "" {
		return ItemSummary{}, ipcerr.Required("collectionId")
	}
	if args.Name == "" {
		return ItemSummary{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	req := model.SavedGrpcRequest{TLSMode: "tls", DescriptorMode: "reflection", ImportPaths: []string{}, Metadata: []model.SavedGrpcMetaRow{}}
	if args.Request != nil {
		req = *args.Request
	}
	if err := req.Validate(); err != nil {
		return ItemSummary{}, ipcerr.BadRequest(err.Error())
	}
	g := apifiles.GRPCOf(req)
	return s.createItem(args.CollectionID, args.ParentID, apifiles.NewItem{Name: args.Name, GRPC: &g})
}

func (s *CollectionsService) createItem(collectionID string, parentID *string, n apifiles.NewItem) (ItemSummary, error) {
	parent := ""
	if parentID != nil {
		parent = *parentID
	}
	id, err := s.Files.CreateItem(collectionID, parent, n)
	if err != nil {
		return ItemSummary{}, apiFilesErr(err)
	}
	return readErr(s.Deps.Repos.Collections.GetItem(id))
}

// CollectionsTargetArgs names a row in either table, 'collection' or 'item', so the renderer never has
// to know which one a tree row lives in.
type CollectionsTargetArgs struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Name   string `json:"name,omitempty"`
}

// Rename sets the name of a collection or an item in its file; the file stem stays.
func (s *CollectionsService) Rename(args CollectionsTargetArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if args.Target != "collection" && args.Target != "item" {
		return ipcerr.Contract("target must be 'collection' or 'item'")
	}
	if args.Name == "" {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.validation.nameRequired.body", nil)
	}
	var err error
	if args.Target == "collection" {
		err = s.Files.RenameCollection(args.ID, args.Name)
	} else {
		err = s.Files.RenameItem(args.ID, args.Name)
	}
	return apiFilesErr(err)
}

type CollectionsDeleteManyArgs struct {
	CollectionIDs []string `json:"collectionIds"`
	ItemIDs       []string `json:"itemIds"`
}

// DeleteMany deletes collection files and items, each with its subtree.
func (s *CollectionsService) DeleteMany(args CollectionsDeleteManyArgs) error {
	if len(args.CollectionIDs) == 0 && len(args.ItemIDs) == 0 {
		return ipcerr.Required("collectionIds or itemIds")
	}
	return apiFilesErr(s.Files.DeleteItems(args.CollectionIDs, args.ItemIDs))
}

type CollectionsMoveItemsArgs struct {
	ItemIDs      []string `json:"itemIds"`
	CollectionID string   `json:"collectionId"`
	ParentID     *string  `json:"parentId"`
	BeforeID     *string  `json:"beforeId"`
}

// MoveItems moves requests and folders, each with its subtree, under parentId (null is the collection
// root) of collectionId, before beforeId or last when null. It answers with each moved row id and the
// row id it has now: a move into another collection file makes new rows.
func (s *CollectionsService) MoveItems(args CollectionsMoveItemsArgs) (map[string]string, error) {
	if len(args.ItemIDs) == 0 {
		return nil, ipcerr.Required("itemIds")
	}
	if args.CollectionID == "" {
		return nil, ipcerr.Required("collectionId")
	}
	moved, err := s.Files.MoveItems(args.ItemIDs, args.CollectionID, args.ParentID, args.BeforeID)
	if err != nil {
		return nil, apiFilesErr(err)
	}
	return moved, nil
}

type CollectionsReorderCollectionsArgs struct {
	IDs []string `json:"ids"`
}

// ReorderCollections puts the listed collections first, in that order; ids left out keep their order after.
func (s *CollectionsService) ReorderCollections(args CollectionsReorderCollectionsArgs) error {
	if len(args.IDs) == 0 {
		return ipcerr.Required("ids")
	}
	if err := s.Files.ReorderCollections(args.IDs); err != nil {
		return apiFilesErr(err)
	}
	return nil
}

// apiFilesErr maps a config file write failure: a missing row or item is E_NOT_FOUND, a broken or
// refused file E_BAD_REQUEST, a file changed twice during the write E_CONFLICT.
func apiFilesErr(err error) error {
	var broken *apifiles.BrokenError
	var invalid *apifiles.InvalidError
	switch {
	case errors.As(err, &broken):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.apiFiles.fileBroken.body",
			map[string]string{"file": broken.File, "line": strconv.Itoa(broken.Line)})
	case errors.As(err, &invalid):
		return ipcerr.BadRequest(invalid.Problem.Message)
	case errors.Is(err, apifiles.ErrNotFound), errors.Is(err, apifiles.ErrItemGone):
		return ipcerr.T(ipcerr.CodeNotFound, "backend.studio.apiFiles.notFound.body", nil)
	case errors.Is(err, apifiles.ErrInvalidMove):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.collection.invalidMove.body", nil)
	case errors.Is(err, apifiles.ErrInvalidOrder):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.collection.invalidOrder.body", nil)
	case errors.Is(err, apifiles.ErrNotFolder), errors.Is(err, apifiles.ErrProtocol), errors.Is(err, configfs.ErrInvalid):
		return ipcerr.BadRequest(err.Error())
	case errors.Is(err, configfs.ErrConflict):
		return ipcerr.T(ipcerr.CodeConflict, "backend.studio.apiFiles.changed.body", nil)
	case errors.Is(err, apifiles.ErrSecretStoreOff):
		return ipcerr.T(ipcerr.CodeSecretStore, "backend.studio.apiFiles.noStore.body", nil)
	}
	return ipcerr.Wrap(err)
}

// ImportWarning is one counted class of "the app quietly did something other than what the file
// said" (D12). Counted rather than listed per-item: the point is that the user learns it at import
// time rather than from a confusing 401 or an E_BAD_REQUEST minutes later.
type ImportWarning struct {
	Kind   string `json:"kind"`
	Count  int    `json:"count"`
	Detail string `json:"detail"`
}

// ImportReport is what Import answers with — part of the feature, not decoration.
type ImportReport struct {
	CollectionID string          `json:"collectionId"`
	Name         string          `json:"name"`
	Folders      int             `json:"folders"`
	Requests     int             `json:"requests"`
	Warnings     []ImportWarning `json:"warnings"`
}

// importWarningKinds is D12's order: warnings are reported in this order.
var importWarningKinds = []string{
	postman.WarnScriptsInert, postman.WarnAuthInert, postman.WarnVariablesInert, postman.WarnVariablesImported,
	postman.WarnEmptyValues, postman.WarnDisabledVariables, postman.WarnUnnamedVariables,
	postman.WarnGraphQLBody, postman.WarnUnsupportedMethod, postman.WarnUnresolvedFile,
	postman.WarnInlineFileContent, postman.WarnDisabledBody, postman.WarnMissingRequest, postman.WarnMalformedItem,
}

// importWarningDetail is D12's table: one sentence per warning kind. The messages live here rather
// than in internal/postman because they are UI text — that package only knows what it did.
func importWarningDetail(kind string, n int) string {
	c := strconv.Itoa(n)
	switch kind {
	case postman.WarnScriptsInert:
		return i18n.T("backend.studio.collection.import.scriptsInert.body", map[string]string{"count": c})
	case postman.WarnAuthInert:
		return i18n.T("backend.studio.collection.import.authInert.body", map[string]string{"count": c})
	case postman.WarnVariablesInert:
		return i18n.T("backend.studio.collection.import.variablesInert.body", map[string]string{"count": c})
	case postman.WarnVariablesImported:
		return i18n.T("backend.studio.collection.import.variablesImported.body", map[string]string{"count": c})
	case postman.WarnGraphQLBody:
		return i18n.T("backend.studio.collection.import.graphqlBody.body", map[string]string{"count": c, "format": "GraphQL"})
	case postman.WarnUnsupportedMethod:
		return i18n.T("backend.studio.collection.import.unsupportedMethod.body", map[string]string{"count": c, "method": "GET"})
	case postman.WarnUnresolvedFile:
		return i18n.T("backend.studio.collection.import.unresolvedFile.body", map[string]string{"count": c})
	case postman.WarnInlineFileContent:
		return i18n.T("backend.studio.collection.import.inlineFileContent.body", map[string]string{"count": c})
	case postman.WarnDisabledBody:
		return i18n.T("backend.studio.collection.import.disabledBody.body", map[string]string{"count": c})
	case postman.WarnMalformedItem:
		return i18n.T("backend.studio.collection.import.malformedItem.body", map[string]string{"count": c})
	case postman.WarnEmptyValues:
		return i18n.T("backend.studio.collection.import.emptyValues.body", map[string]string{"count": c})
	case postman.WarnDisabledVariables:
		return i18n.T("backend.studio.collection.import.disabledVariables.body", map[string]string{"count": c})
	case postman.WarnUnnamedVariables:
		return i18n.T("backend.studio.collection.import.unnamedVariables.body", map[string]string{"count": c})
	case postman.WarnMissingRequest:
		return i18n.T("backend.studio.collection.import.missingRequest.body", map[string]string{"count": c})
	}
	return ""
}

type CollectionsImportArgs struct {
	Paths []string `json:"paths"`
}

// EnvironmentImportReport is what one imported environment file answers with.
type EnvironmentImportReport struct {
	EnvironmentID string          `json:"environmentId"`
	Name          string          `json:"name"`
	Variables     int             `json:"variables"`
	Warnings      []ImportWarning `json:"warnings"`
}

// ImportFileResult is one picked file's outcome. Kind is "collection" or "environment", "" when the
// file failed (Error then carries the reason).
type ImportFileResult struct {
	Path        string                   `json:"path"`
	FileName    string                   `json:"fileName"`
	Kind        string                   `json:"kind"`
	Collection  *ImportReport            `json:"collection"`
	Environment *EnvironmentImportReport `json:"environment"`
	Error       *string                  `json:"error"`
}

// ImportResult lists one entry per path, in the order given.
type ImportResult struct {
	Files []ImportFileResult `json:"files"`
}

// Import parses each file and writes it: a collection as rows in one transaction, an environment as
// one environment row. Files are independent — a failed one is reported in its own entry and the
// rest still import. Only paths cross the bridge, never file bytes (F16).
func (s *CollectionsService) Import(args CollectionsImportArgs) (ImportResult, error) {
	if len(args.Paths) == 0 {
		return ImportResult{}, ipcerr.Required("paths")
	}
	result := ImportResult{Files: make([]ImportFileResult, 0, len(args.Paths))}
	var changes []ApiDataChange
	treeChanged, envsChanged := false, false
	for _, path := range args.Paths {
		entry := ImportFileResult{Path: path, FileName: filepath.Base(path)}
		file, err := s.importFile(path)
		switch {
		case err != nil:
			msg := importErrorMessage(err)
			entry.Error = &msg
		case file.collection != nil:
			entry.Kind, entry.Collection = "collection", file.collection
		default:
			entry.Kind, entry.Environment = "environment", file.environment
		}
		treeChanged = treeChanged || file.treeCommitted
		envsChanged = envsChanged || file.environment != nil
		result.Files = append(result.Files, entry)
	}
	if treeChanged {
		changes = append(changes, treeChange())
	}
	if envsChanged {
		changes = append(changes, environmentsChange())
	}
	if len(changes) > 0 {
		emitApiData(s.Deps.Events, changes...)
	}
	return result, nil
}

// importErrorMessage is the user-facing text of a coded error, or the plain error text.
func importErrorMessage(err error) string {
	var coded *ipcerr.Error
	if errors.As(err, &coded) {
		return coded.Message
	}
	return err.Error()
}

// importedFile is what one importFile call wrote. treeCommitted stays true when a collection
// committed and a later step failed, so other windows still see the partial state (P112).
type importedFile struct {
	collection    *ImportReport
	environment   *EnvironmentImportReport
	treeCommitted bool
}

func (s *CollectionsService) importFile(path string) (importedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return importedFile{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.file.readFailed.error",
			map[string]string{"path": path, "detail": err.Error()})
	}
	defer f.Close()

	parsed, err := postman.ParseFile(f)
	if err != nil {
		// A refusal here is about the file's own content (D10's version gate, or JSON that is not
		// a Postman export at all), which is the user's to fix — not an internal failure.
		return importedFile{}, ipcerr.BadRequest(err.Error())
	}
	if parsed.Environment != nil {
		report, err := s.importEnvironment(parsed.Environment)
		return importedFile{environment: report}, err
	}
	return s.importCollection(parsed.Collection)
}

func (s *CollectionsService) importEnvironment(env *postman.Environment) (*EnvironmentImportReport, error) {
	stem, err := s.Files.ImportEnvironment(env)
	if err != nil {
		return nil, apiFilesErr(err)
	}
	var id string
	if err := s.Deps.DB.QueryRow(`SELECT id FROM api_environments WHERE stem = ?`, stem).Scan(&id); err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	report := &EnvironmentImportReport{
		EnvironmentID: id, Name: env.Name, Variables: len(env.Variables),
		Warnings: []ImportWarning{},
	}
	for _, w := range []struct {
		kind  string
		count int
	}{
		{postman.WarnDisabledVariables, env.Disabled},
		{postman.WarnEmptyValues, env.Empty},
		{postman.WarnUnnamedVariables, env.Unnamed},
	} {
		if w.count > 0 {
			report.Warnings = append(report.Warnings, ImportWarning{
				Kind: w.kind, Count: w.count, Detail: importWarningDetail(w.kind, w.count),
			})
		}
	}
	return report, nil
}

func (s *CollectionsService) importCollection(tree *postman.Tree) (importedFile, error) {
	stem, err := s.Files.ImportCollection(tree)
	if err != nil {
		return importedFile{}, apiFilesErr(err)
	}
	out := importedFile{treeCommitted: true}
	var id string
	if err := s.Deps.DB.QueryRow(`SELECT id FROM api_collections WHERE stem = ?`, stem).Scan(&id); err != nil {
		return out, ipcerr.InternalErr(err)
	}
	report := &ImportReport{
		CollectionID: id, Name: tree.Name,
		Folders: tree.Report.Folders, Requests: tree.Report.Requests,
		Warnings: []ImportWarning{},
	}
	for _, kind := range importWarningKinds {
		count := tree.Report.Warnings[kind]
		if count == 0 {
			continue
		}
		report.Warnings = append(report.Warnings, ImportWarning{
			Kind: kind, Count: count, Detail: importWarningDetail(kind, count),
		})
	}
	out.collection = report
	return out, nil
}

type CollectionsExportArgs struct {
	CollectionID string `json:"collectionId"`
	Path         string `json:"path"`
}

// writeExportFile writes to a renameio temp file next to the target and renames it over the target
// only once write has fully succeeded, so a failed re-export never destroys a good file.
//
// The target is path with any symlink resolved first (EvalSymlinks errors when path doesn't exist
// yet, in which case path is used as-is), so the rename writes through a link instead of replacing
// it. An existing target keeps its mode; a fresh one is private (0600).
func writeExportFile(path string, write func(*os.File) error) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	f, err := renameio.NewPendingFile(target, renameio.WithExistingPermissions())
	if err != nil {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.collection.writeFailed.error",
			map[string]string{"path": path, "detail": err.Error()})
	}
	defer func() { _ = f.Cleanup() }()

	if err := write(f.File); err != nil {
		return ipcerr.InternalErr(err)
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		return ipcerr.T(ipcerr.CodeInternal, "backend.studio.collection.finishWriteFailed.error",
			map[string]string{"path": path, "detail": err.Error()})
	}
	return nil
}

// ExportReport is Export's answer — a secret is written valueless, and SecretCount is what lets
// the renderer say so once, rather than that being a fact only discoverable by opening the file.
// SkippedGrpc: Postman Collection v2.1 has no representation for a gRPC request, so every
// protocol='grpc' item is skipped and the skip is reported — silently dropping requests from an
// export would be the worst possible reading of "faithful round-trip".
type ExportReport struct {
	SecretCount int `json:"secretCount"`
	SkippedGrpc int `json:"skippedGrpc"`
}

// Export writes the collection at path as Collection v2.1 JSON. The file is written whole rather
// than streamed: a collection is rows in a local table and the writer needs the tree in memory to
// rebuild the nested arrays anyway.
//
// This used to os.Create(args.Path) directly — truncate-or-create — and
// stream postman.Write straight into it, so a marshal error, a full disk, or the process dying
// mid-write left the destination empty or half-written with no cleanup, silently destroying
// whatever was there before (a re-export is exactly the kind of save people make over last week's
// file). Now writeExportFile writes a temp file and renames it onto the destination — the
// destination is only ever replaced by a complete, fully-flushed file.
func (s *CollectionsService) Export(args CollectionsExportArgs) (ExportReport, error) {
	if args.CollectionID == "" {
		return ExportReport{}, ipcerr.Required("collectionId")
	}
	if args.Path == "" {
		return ExportReport{}, ipcerr.Required("path")
	}
	tree, err := s.Deps.Repos.Collections.LoadTree(args.CollectionID)
	if err != nil {
		return ExportReport{}, ipcerr.InternalErr(err)
	}

	if err := writeExportFile(args.Path, func(f *os.File) error { return postman.Write(f, tree) }); err != nil {
		return ExportReport{}, err
	}

	secretCount := 0
	for _, v := range tree.Variables {
		if v.Secret {
			secretCount++
		}
	}
	skippedGrpc := 0
	for _, item := range tree.Items {
		if item.Protocol == "grpc" {
			skippedGrpc++
		}
	}
	return ExportReport{SecretCount: secretCount, SkippedGrpc: skippedGrpc}, nil
}
