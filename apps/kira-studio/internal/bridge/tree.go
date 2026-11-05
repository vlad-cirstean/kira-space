package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tree"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type TreeService struct {
	Deps appcore.Deps
}

type TreeChildrenArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
	Refresh      bool   `json:"refresh"`
}

func (s *TreeService) Children(args TreeChildrenArgs) (tree.ChildrenResult, error) {
	if args.ConnectionID == "" {
		return tree.ChildrenResult{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Tree.Children(context.Background(), args.ConnectionID, args.Path, args.Refresh)
}

// TreeDescribeArgs is shared by Describe and Definition — both take the same four arguments.
type TreeDescribeArgs struct {
	ConnectionID string  `json:"connectionId"`
	Path         string  `json:"path"`
	Refresh      bool    `json:"refresh"`
	TabID        *string `json:"tabId"`
}

func (s *TreeService) Describe(args TreeDescribeArgs) (tree.DescribeResult, error) {
	if args.ConnectionID == "" {
		return tree.DescribeResult{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Tree.Describe(context.Background(), args.ConnectionID, args.Path, args.Refresh, args.TabID)
}

func (s *TreeService) Definition(args TreeDescribeArgs) (tree.DefinitionResult, error) {
	if args.ConnectionID == "" {
		return tree.DefinitionResult{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Tree.Definition(context.Background(), args.ConnectionID, args.Path, args.Refresh, args.TabID)
}

// SchemaColumns is P22c D3 — Args reuses TreeDescribeArgs (TabID unused: a schema-wide fetch is
// not tagged to one tab's op-log row).
func (s *TreeService) SchemaColumns(args TreeDescribeArgs) (tree.SchemaColumnsResult, error) {
	if args.ConnectionID == "" {
		return tree.SchemaColumnsResult{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Tree.SchemaColumns(context.Background(), args.ConnectionID, args.Path, args.Refresh)
}

// TreeKeyTypesArgs is P63 §4.3's per-key type batch — Paths is capped client-side at 200
// (BrowseView.vue's own visible-range window), never enforced here: the adapter pipelines
// whatever it's given, and the renderer alone owns the batching discipline.
type TreeKeyTypesArgs struct {
	ConnectionID string   `json:"connectionId"`
	Paths        []string `json:"paths"`
}

func (s *TreeService) KeyTypes(args TreeKeyTypesArgs) ([]string, error) {
	if args.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	return s.Deps.Tree.KeyTypes(context.Background(), args.ConnectionID, args.Paths)
}

// TreeInvalidateArgs's Path nil drops the whole connection; non-nil drops one node.
type TreeInvalidateArgs struct {
	ConnectionID string  `json:"connectionId"`
	Path         *string `json:"path"`
}

func (s *TreeService) Invalidate(args TreeInvalidateArgs) error {
	if args.ConnectionID == "" {
		return ipcerr.Required("connectionId")
	}
	if err := s.Deps.Tree.Invalidate(args.ConnectionID, args.Path); err != nil {
		return err
	}
	if s.Deps.MaskReview != nil {
		s.Deps.MaskReview.Refresh(args.ConnectionID)
	}
	return nil
}
