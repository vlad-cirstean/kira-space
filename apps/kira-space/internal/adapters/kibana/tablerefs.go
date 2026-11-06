package kibana

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// ParseTables is adapters.TableReferencer. A request reads one data view, named by the request or the
// path; it decodes exactly as Execute does so both agree on the view.
func (a *Adapter) ParseTables(statement string, path model.NodePath) adapters.TableParse {
	return parseTablesKibana(statement, path)
}

// ResolveTables is never called: ParseTables never asks for a probe.
func (a *Adapter) ResolveTables(context.Context, adapters.TableParse, model.NodePath, *adapters.OpCtx) ([]tableref.Ref, string, error) {
	return nil, "", nil
}

func parseTablesKibana(statement string, path model.NodePath) adapters.TableParse {
	var dr model.KibanaDiscoverRequest
	dec := json.NewDecoder(strings.NewReader(statement))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dr); err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	if dr.DataView == "" && len(path.Segments) == 1 {
		dr.DataView = path.Segments[0].Name
	}
	if dr.DataView == "" {
		return adapters.TableParse{Refusal: "parse error: no data view"}
	}
	return adapters.TableParse{Refs: []tableref.Ref{{Name: dr.DataView}}}
}
