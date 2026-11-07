package memory

import (
	"regexp"
	"strconv"

	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

// FactIDPattern matches a fact id inside text (word-boundary grep).
var FactIDPattern = regexp.MustCompile(`\b` + memfmt.FactID + `\b`)

// versionID is the id of one version of a fact: "<fact id>.<version>".
func versionID(factID string, version int) string { return factID + "." + strconv.Itoa(version) }
