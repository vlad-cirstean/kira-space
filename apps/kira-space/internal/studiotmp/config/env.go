package config

import "github.com/kirathecat/kira-studio/internal/kirapaths"

// IsDev is P103 Part 3 §6.4's one-line wrapper — the build-tag-gated logic itself
// (isProductionBuild, KIRA_DEV) is repo-root internal/kirapaths, shared with Kira Space, since it
// was byte-identical in both apps.
func IsDev() bool {
	return kirapaths.IsDev()
}
