package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Repos is every storage repo that needs no cipher, constructed once at startup. Variables and
// MaskKeys need one, so they are constructed separately via NewVariables and NewMaskKeys once the
// cipher is wired.
type Repos struct {
	Settings        *SettingsRepo
	Layout          *LayoutRepo
	Tabs            *TabsRepo
	Windows         *WindowsRepo
	Connections     *ConnectionsRepo
	Ops             *OpsRepo
	SavedQueries    *SavedQueriesRepo
	FilterHistory   *FilterHistoryRepo
	Metadata        *MetadataCacheRepo
	Collections     *CollectionsRepo
	Variables       *VariablesRepo
	ResponseHistory *ResponseHistoryRepo
	GrpcHistory     *GrpcHistoryRepo
	// MaskRules is M5's rule store (connection_mask_rules) — needs no cipher, unlike MaskKeys just
	// below, so it belongs in this aggregate the same as every other cipher-free repo.
	MaskRules *MaskRulesRepo
	// CustomScripts is P85's own launch-target store (custom_scripts).
	CustomScripts *scripts.Repo
	ScriptRuns    *scriptruns.Repo
	// DockerRestore is P334's last-known-running store for Docker engine restarts.
	DockerRestore *DockerRestoreRepo
	// KubeClusters is P439's cluster identity store.
	KubeClusters *KubeClustersRepo
	// Proof is P420's validation-proof session store.
	Proof *ProofRepo

	stmts []*sql.Stmt // every prepared statement below, for Close.
}

// New prepares P52 §5.4's five hot statements once (settings read, layout read, tabs list,
// op-log append/finish) and constructs every repo that needs no cipher.
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
	opsInsert, err := db.Prepare(opsInsertSQL)
	if err != nil {
		return nil, fmt.Errorf("repos: prepare ops insert: %w", err)
	}
	opsUpdate, err := db.Prepare(opsUpdateSQL)
	if err != nil {
		return nil, fmt.Errorf("repos: prepare ops update: %w", err)
	}

	return &Repos{
		Settings:        &SettingsRepo{DB: db, selectAll: settingsSelectAll},
		Layout:          &LayoutRepo{DB: db, selectAll: layoutSelectAll},
		Tabs:            &TabsRepo{DB: db, selectAll: tabsSelectAll},
		Windows:         &WindowsRepo{DB: db},
		Connections:     &ConnectionsRepo{DB: db},
		Ops:             &OpsRepo{DB: db, insert: opsInsert, update: opsUpdate},
		SavedQueries:    &SavedQueriesRepo{DB: db},
		FilterHistory:   &FilterHistoryRepo{DB: db},
		Metadata:        &MetadataCacheRepo{DB: db},
		Collections:     &CollectionsRepo{DB: db},
		ResponseHistory: &ResponseHistoryRepo{DB: db},
		GrpcHistory:     &GrpcHistoryRepo{DB: db},
		MaskRules:       &MaskRulesRepo{DB: db},
		CustomScripts:   &scripts.Repo{DB: db},
		ScriptRuns:      &scriptruns.Repo{DB: db},
		DockerRestore:   &DockerRestoreRepo{DB: db},
		KubeClusters:    &KubeClustersRepo{DB: db},
		Proof:           &ProofRepo{DB: db},
		stmts:           []*sql.Stmt{settingsSelectAll, layoutSelectAll, tabsSelectAll, opsInsert, opsUpdate},
	}, nil
}

// Close releases every prepared statement. It does not close the underlying *sql.DB, which the
// caller (main.go) owns.
func (r *Repos) Close() error {
	return appstorage.CloseStmts(r.stmts)
}
