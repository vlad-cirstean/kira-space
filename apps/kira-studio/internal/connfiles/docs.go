package connfiles

import (
	_ "embed"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

//go:embed connections_intro.md
var introText string

// DocModel is the AGENTS.md model of the connections folder.
func DocModel() (configfs.DocModel, error) {
	d, err := newDecoder()
	if err != nil {
		return configfs.DocModel{}, err
	}
	m := d.doc
	m.App, m.Title, m.Pattern, m.Format, m.Intro = "Studio", "Connections", "<id>.yaml", "YAML", introText
	return m, nil
}
