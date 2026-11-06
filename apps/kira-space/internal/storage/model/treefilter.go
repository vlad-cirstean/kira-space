package model

// TreeVisibility mirrors packages/shared/domain/treeFilter.ts's treeVisibilitySchema — a set of
// hidden node kinds and hidden paths, not a rule list (P28 D12).
type TreeVisibility struct {
	HiddenKinds []string `json:"hiddenKinds"`
	HiddenPaths []string `json:"hiddenPaths"`
}

// EmptyVisibility mirrors packages/shared/domain/treeFilter.ts's EMPTY_VISIBILITY.
func EmptyVisibility() TreeVisibility {
	return TreeVisibility{HiddenKinds: []string{}, HiddenPaths: []string{}}
}
