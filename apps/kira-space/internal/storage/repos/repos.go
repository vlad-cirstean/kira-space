package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Repos is every storage repo this app needs that takes no cipher, constructed once at startup.
// Variables and MaskKeys need one, so they come from NewVariables and NewMaskKeys once the
// cipher is wired.
type Repos struct {
	Settings              *SettingsRepo
	Windows               *WindowsRepo
	MobileDevices         *MobileDevicesRepo
	MobileNetwork         *MobileTrustedNetworkRepo
	GitHostChangeRequests *GitHostChangeRequestsRepo
	FileHandlerPrevious   *FileHandlerPreviousRepo
	CodeRepos             *CodeReposRepo
	Layout                *LayoutRepo
	Tabs                  *TabsRepo
	AdeSessions           *AdeSessionsRepo
	AdeReview             *AdeReviewWindowsRepo
	AdeTasks              *AdeTaskRepo
	AdeStatuses           *AdeStatusRepo
	AdeRepoConfig         *AdeRepoConfigRepo
	AdeFacts              *AdeFactsRepo
	AdeLogs               *AdeLogsRepo
	AdeRunSandbox         *AdeRunSandboxRepo
	AdeSessionSandbox     *AdeSessionSandboxRepo
	AdeTrackedItems       *AdeTrackedItemsRepo
	OpLog                 *OpLogRepo
	DbAudit               *DbAuditRepo
	Connections           *ConnectionsRepo
	SavedQueries          *SavedQueriesRepo
	FilterHistory         *FilterHistoryRepo
	Metadata              *MetadataCacheRepo
	Collections           *CollectionsRepo
	Variables             *VariablesRepo
	ResponseHistory       *ResponseHistoryRepo
	GrpcHistory           *GrpcHistoryRepo
	MaskRules             *MaskRulesRepo
	MaskReview            *MaskReviewRepo
	DockerRestore         *DockerRestoreRepo
	KubeClusters          *KubeClustersRepo
	Proof                 *ProofRepo
	CustomScripts         *scripts.Repo
	ScriptRuns            *scriptruns.Repo

	stmts []*sql.Stmt // every prepared statement below, for Close.
}

// New prepares the settings and layout reads (this app's own hot boot-path reads, the same
// "prepare it once" treatment Kira Studio's own repos.New gives its settings/layout/tabs reads)
// and constructs every repo.
func New(db *sql.DB) (*Repos, error) {
	settingsSelectAll, err := db.Prepare(settingsSelectAllSQL)
	if err != nil {
		return nil, fmt.Errorf("repos: prepare settings select: %w", err)
	}
	layoutSelectAll, err := db.Prepare(appstorage.LayoutSelectAllSQL)
	if err != nil {
		return nil, fmt.Errorf("repos: prepare layout select: %w", err)
	}
	tabsSelectAll, err := db.Prepare(tabsSelectAllSQL)
	if err != nil {
		return nil, fmt.Errorf("repos: prepare tabs select: %w", err)
	}

	return &Repos{
		Settings:              &SettingsRepo{DB: db, selectAll: settingsSelectAll},
		Windows:               &WindowsRepo{DB: db},
		MobileDevices:         &MobileDevicesRepo{DB: db},
		MobileNetwork:         &MobileTrustedNetworkRepo{DB: db},
		GitHostChangeRequests: &GitHostChangeRequestsRepo{DB: db},
		FileHandlerPrevious:   &FileHandlerPreviousRepo{DB: db},
		CodeRepos:             &CodeReposRepo{DB: db},
		Layout:                &LayoutRepo{DB: db, selectAll: layoutSelectAll},
		Tabs:                  &TabsRepo{DB: db, selectAll: tabsSelectAll},
		AdeSessions:           &AdeSessionsRepo{DB: db},
		AdeReview:             &AdeReviewWindowsRepo{DB: db},
		AdeTasks:              &AdeTaskRepo{DB: db},
		AdeStatuses:           &AdeStatusRepo{DB: db},
		AdeRepoConfig:         &AdeRepoConfigRepo{DB: db},
		AdeFacts:              &AdeFactsRepo{DB: db},
		AdeLogs:               &AdeLogsRepo{DB: db},
		AdeRunSandbox:         &AdeRunSandboxRepo{DB: db},
		AdeSessionSandbox:     &AdeSessionSandboxRepo{DB: db},
		AdeTrackedItems:       &AdeTrackedItemsRepo{DB: db},
		OpLog:                 &OpLogRepo{DB: db},
		DbAudit:               &DbAuditRepo{DB: db},
		Connections:           &ConnectionsRepo{DB: db},
		SavedQueries:          &SavedQueriesRepo{DB: db},
		FilterHistory:         &FilterHistoryRepo{DB: db},
		Metadata:              &MetadataCacheRepo{DB: db},
		Collections:           &CollectionsRepo{DB: db},
		ResponseHistory:       &ResponseHistoryRepo{DB: db},
		GrpcHistory:           &GrpcHistoryRepo{DB: db},
		MaskRules:             &MaskRulesRepo{DB: db},
		MaskReview:            &MaskReviewRepo{DB: db},
		DockerRestore:         &DockerRestoreRepo{DB: db},
		KubeClusters:          &KubeClustersRepo{DB: db},
		Proof:                 &ProofRepo{DB: db},
		CustomScripts:         &scripts.Repo{DB: db},
		ScriptRuns:            &scriptruns.Repo{DB: db},
		stmts:                 []*sql.Stmt{settingsSelectAll, layoutSelectAll, tabsSelectAll},
	}, nil
}

// Close releases every prepared statement. It does not close the underlying *sql.DB, which the
// caller (main.go) owns.
func (r *Repos) Close() error {
	return appstorage.CloseStmts(r.stmts)
}
