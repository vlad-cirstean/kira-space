package sqlitex

import (
	"io/fs"
	"sort"
	"strings"
)

// MigrationSource names one embedded migration file in application order — the shared shape each
// app's own migrations/embed.go walked separately (storage/migrations for both apps,
// gitreview/migrations) before P107 I2-4 pulled the read-and-sort loop out from under all three.
type MigrationSource struct {
	Version int
	Name    string
	File    string
}

// LoadMigrations reads every source's file from fsys and returns them as Migration, sorted by
// Version. Each caller keeps its own //go:embed directive and its own MigrationSource table; this
// owns only the loop the three copies shared.
func LoadMigrations(fsys fs.FS, sources []MigrationSource) ([]Migration, error) {
	out := make([]Migration, 0, len(sources))
	for _, s := range sources {
		b, err := fs.ReadFile(fsys, s.File)
		if err != nil {
			return nil, err
		}
		sqlText := string(b)
		first, _, _ := strings.Cut(sqlText, "\n")
		out = append(out, Migration{
			Version: s.Version, Name: s.Name, SQL: sqlText,
			ForeignKeysOff: strings.TrimSpace(first) == ForeignKeysOffMarker,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
