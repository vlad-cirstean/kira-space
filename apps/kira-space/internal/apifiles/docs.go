package apifiles

import (
	_ "embed"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

//go:embed agents_intro.md
var agentsIntro string

const (
	collectionIntro  = "A collection holds variables and a tree of folders and requests. Requests are HTTP or gRPC."
	environmentIntro = "An environment holds variables that override collection variables while it is the active environment."
)

// DocModel is the AGENTS.md model of the api folder: one section per file kind.
func DocModel() (configfs.DocModel, error) {
	d, err := newDecoders()
	if err != nil {
		return configfs.DocModel{}, err
	}
	c, e := d.collectionDoc, d.environmentDoc
	c.App, c.Pattern, c.Format, c.Intro = "Studio", "<id>.collection.yaml", "YAML", collectionIntro
	e.App, e.Pattern, e.Format, e.Intro = "Studio", "<id>.environment.yaml", "YAML", environmentIntro
	return configfs.DocModel{App: "Studio", Title: "Api", Intro: agentsIntro, Sections: []configfs.DocModel{c, e}}, nil
}
