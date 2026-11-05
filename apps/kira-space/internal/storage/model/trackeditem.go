package model

// AdeTrackedItem is one row of ade_tracked_items: what the user last saw of a tracked item of a
// task and the latest observation. Times are Unix milliseconds; ChangedAt is 0 while the two match.
type AdeTrackedItem struct {
	TaskID      string
	ItemKey     string
	Kind        string
	SeenJSON    string
	CurrentJSON string
	SeenAt      int64
	ObservedAt  int64
	ChangedAt   int64
}

// AdeTrackedKey identifies one tracked item.
type AdeTrackedKey struct {
	TaskID  string
	ItemKey string
}
