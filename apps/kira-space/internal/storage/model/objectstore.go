package model

// ObjectDownloadRequest is domain/objectStore.ts's ObjectDownloadRequest — adapter-side, built by
// the data-op dispatcher from a decoded NodePath plus the wire's own destPath, never parsed whole
// at a boundary. DestPath is an absolute local path chosen through the save dialog.
type ObjectDownloadRequest struct {
	Path     NodePath
	DestPath string
}

// ObjectTransferResult is domain/objectStore.ts's ObjectTransferResult.
type ObjectTransferResult struct {
	Bytes int64 `json:"bytes"`
}
