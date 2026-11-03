// Package palette holds the storable colour names shared by every Go store. It mirrors
// packages/shared/domain/color.ts's paletteColorSchema.
package palette

import "sort"

var valid = map[string]bool{
	"none": true, "red": true, "orange": true, "amber": true, "olive": true, "green": true,
	"teal": true, "cyan": true, "blue": true, "indigo": true, "violet": true, "magenta": true,
	"grey": true,
}

// autoRepoColors rotates through offered, non-grey hues in adjacent-contrast order.
var autoRepoColors = [...]string{"blue", "amber", "magenta", "green", "red", "cyan"}

// Valid reports whether name is a storable palette colour.
func Valid(name string) bool { return valid[name] }

// AutoRepoColor returns the default colour for the repo at sort position n.
func AutoRepoColor(n int) string {
	if n < 0 {
		n = -n
	}
	return autoRepoColors[n%len(autoRepoColors)]
}

// Names lists every storable colour, sorted.
func Names() []string {
	out := make([]string, 0, len(valid))
	for n := range valid {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
