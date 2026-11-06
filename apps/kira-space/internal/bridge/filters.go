package bridge

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// FiltersService reads and edits the left panel visibility in a connection's file: the file is the
// source, the tree section its node.
type FiltersService struct {
	Deps appcore.Deps
}

type FiltersListArgs struct {
	ConnectionID string `json:"connectionId"`
}

func (s *FiltersService) List(args FiltersListArgs) (model.TreeVisibility, error) {
	if args.ConnectionID == "" {
		return model.TreeVisibility{}, ipcerr.Required("connectionId")
	}
	rec, ok := s.Deps.ConnectionFiles.RecordOf(args.ConnectionID)
	if !ok {
		return model.TreeVisibility{}, connfiles.IPCError(connfiles.ErrNotFound)
	}
	return rec.File.Visibility(), nil
}

type FiltersReplaceArgs struct {
	ConnectionID string               `json:"connectionId"`
	Visibility   model.TreeVisibility `json:"visibility"`
}

// Replace writes the whole visibility set into the file's tree section. A repeated or blank entry is
// dropped, as the file refuses both.
func (s *FiltersService) Replace(args FiltersReplaceArgs) (model.TreeVisibility, error) {
	if args.ConnectionID == "" {
		return model.TreeVisibility{}, ipcerr.Required("connectionId")
	}
	tree := connfiles.Tree{HiddenKinds: uniqueNonBlank(args.Visibility.HiddenKinds), HiddenPaths: uniqueNonBlank(args.Visibility.HiddenPaths)}
	if err := s.Deps.ConnectionFiles.Patch(args.ConnectionID, configfs.NodeSet{Path: []string{"tree"}, Value: tree}); err != nil {
		return model.TreeVisibility{}, connfiles.IPCError(err)
	}
	return s.List(FiltersListArgs{ConnectionID: args.ConnectionID})
}

func uniqueNonBlank(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if strings.TrimSpace(v) == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
