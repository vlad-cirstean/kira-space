package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	tableAccept = "application/json;as=Table;v=v1;g=meta.k8s.io"
	jsonAccept  = "application/json"
)

// resourcePath is the API path of a kind, namespaced when namespace is set, and of one object when
// name is set.
func resourcePath(k Kind, namespace, name string) string {
	p := "/api/" + k.Version
	if k.Group != "" {
		p = "/apis/" + k.Group + "/" + k.Version
	}
	if k.Namespaced && namespace != "" {
		p += "/namespaces/" + url.PathEscape(namespace)
	}
	p += "/" + k.Resource
	if name != "" {
		p += "/" + url.PathEscape(name)
	}
	return p
}

func cellString(v any) string {
	switch c := v.(type) {
	case nil:
		return ""
	case string:
		return c
	case float64:
		return strconv.FormatFloat(c, 'f', -1, 64)
	default:
		return fmt.Sprint(c)
	}
}

// List returns a kubectl-style table of one kind, paged server side and capped at the row limit.
func (s *Service) List(ctx context.Context, args ListArgs) (ListResult, error) {
	return s.list(ctx, args, s.listMax)
}

func (s *Service) list(ctx context.Context, args ListArgs, max int) (ListResult, error) {
	if args.LabelSelector != "" {
		if _, err := labels.Parse(args.LabelSelector); err != nil {
			return ListResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.kubernetes.badSelector.error", map[string]string{"message": err.Error()})
		}
	}
	var res ListResult
	err := s.record(ctx, args.ClusterID, args.Background, func(n string) string { return ListCommand(n, args) }, func(ctx context.Context, note *OpNote) error {
		var err error
		if res, err = s.listRows(ctx, args, max); err == nil {
			note.SetRows(len(res.Rows))
		}
		return err
	})
	if err != nil {
		return ListResult{}, err
	}
	return res, nil
}

func (s *Service) listRows(ctx context.Context, args ListArgs, max int) (ListResult, error) {
	cs, err := s.client(args.ClusterID)
	if err != nil {
		return ListResult{}, mapErr(err)
	}
	kind, err := s.findKind(ctx, cs, args.Kind)
	if err != nil {
		return ListResult{}, mapErr(err)
	}
	allNamespaces := kind.Namespaced && args.Namespace == ""
	path := resourcePath(kind, args.Namespace, "")
	res := ListResult{Columns: []Column{}, Rows: []Row{}}
	cont := ""
	for first := true; ; first = false {
		page, err := s.listPageOf(ctx, cs, path, args.LabelSelector, cont)
		if err != nil {
			return ListResult{}, mapErr(err)
		}
		if first {
			if allNamespaces {
				res.Columns = append(res.Columns, Column{Name: "Namespace", Type: "string"})
			}
			for _, c := range page.ColumnDefinitions {
				if c.Priority == 0 {
					res.Columns = append(res.Columns, Column{Name: c.Name, Type: c.Type})
				}
			}
		}
		for _, r := range page.Rows {
			if len(res.Rows) == max {
				res.Truncated = true
				return res, nil
			}
			res.Rows = append(res.Rows, tableRow(page, r, allNamespaces))
		}
		cont = page.Continue
		if cont == "" {
			return res, nil
		}
		if len(res.Rows) >= max {
			res.Truncated = true
			return res, nil
		}
	}
}

func (s *Service) listPageOf(ctx context.Context, cs *clusterClient, path, selector, cont string) (*metav1.Table, error) {
	cctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	req := cs.rest.Get().AbsPath(path).SetHeader("Accept", tableAccept).
		Param("limit", strconv.Itoa(s.listPage)).Param("includeObject", string(metav1.IncludeMetadata))
	if selector != "" {
		req = req.Param("labelSelector", selector)
	}
	if cont != "" {
		req = req.Param("continue", cont)
	}
	raw, err := req.Do(cctx).Raw()
	if err != nil {
		return nil, err
	}
	var t metav1.Table
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func tableRow(t *metav1.Table, r metav1.TableRow, withNamespace bool) Row {
	var meta metav1.PartialObjectMetadata
	if len(r.Object.Raw) > 0 {
		_ = json.Unmarshal(r.Object.Raw, &meta)
	}
	row := Row{Key: string(meta.UID), Name: meta.Name, Namespace: meta.Namespace, Cells: []string{}}
	if row.Key == "" {
		row.Key = meta.Namespace + "/" + meta.Name
	}
	if withNamespace {
		row.Cells = append(row.Cells, meta.Namespace)
	}
	for i, def := range t.ColumnDefinitions {
		if def.Priority != 0 {
			continue
		}
		cell := ""
		if i < len(r.Cells) {
			cell = cellString(r.Cells[i])
		}
		row.Cells = append(row.Cells, cell)
	}
	return row
}
