package model

// GitHostSchemaVersion is the layout of the persisted change request index; a lower stored value
// drops the repo's rows and syncs them again.
const GitHostSchemaVersion = 1

// GitHostChangeRequest is one cached change request of a code host repository. State is
// open|closed|merged; a draft is an open one with Draft set. Times are Unix milliseconds.
type GitHostChangeRequest struct {
	Number         int
	NodeID         string
	HeadRef        string
	HeadOwner      string
	HeadSha        string
	BaseRef        string
	State          string
	Draft          bool
	Title          string
	URL            string
	ReviewDecision string
	ChecksState    string
	UpdatedAt      int64
	SyncedAt       int64
}

// GitHostSyncState is where one repository's sync stands. Times are Unix milliseconds.
type GitHostSyncState struct {
	// Account is the host account the rows were read as.
	Account string
	// WatermarkUpdatedAt is the newest updatedAt fully ingested.
	WatermarkUpdatedAt int64
	ETag               string
	LastFullSyncAt     int64
	LastDeltaAt        int64
	LastChecksAt       int64
	// BackfillCursor resumes an interrupted full load.
	BackfillCursor   string
	BackfillComplete bool
	// DeltaFloor is the watermark before a delta that overflowed the page cap; the resumed walk
	// completes on reaching it. 0 for a first backfill, which walks the whole history.
	DeltaFloor    int64
	SchemaVersion int
}
