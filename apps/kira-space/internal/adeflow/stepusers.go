package adeflow

import (
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// StepUsers maps a library step id to the ids of the workflows whose files reference it. It reads the
// files' text, so a workflow that is broken right now still counts; a file that does not parse falls
// back to its last valid version.
func (r *Reader) StepUsers() map[string][]string {
	out := map[string][]string{}
	for _, e := range r.folder().List() {
		id := strings.TrimSuffix(e.File, ".yaml")
		ids := map[string]bool{}
		if src, err := configfs.ReadCapped(e.Path, configfs.MaxFileBytes); err == nil {
			for _, s := range referencedIDs(src) {
				ids[s] = true
			}
		}
		if e.Value != nil {
			for _, st := range e.Value.Steps {
				if st.Use != "" {
					ids[st.Use] = true
				}
			}
		}
		for s := range ids {
			out[s] = append(out[s], id)
		}
	}
	return out
}

// referencedIDs lists the `step:` values of a workflow file's items; nil when the file does not parse.
func referencedIDs(src []byte) []string {
	var doc yaml.Node
	if yaml.Unmarshal(src, &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	_, steps := find(doc.Content[0], "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, item := range steps.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if _, v := find(item, "step"); v != nil && v.Kind == yaml.ScalarNode {
			out = append(out, strings.TrimSpace(v.Value))
		}
	}
	return out
}
