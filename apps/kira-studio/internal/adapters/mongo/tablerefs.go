package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// ParseTables is adapters.TableReferencer. A statement targets one collection; aggregation stages that
// read or write another ($lookup and the like) are found by scanning every argument, nested pipelines included.
func (a *Adapter) ParseTables(statement string, path model.NodePath) adapters.TableParse {
	db := ""
	if len(path.Segments) > 0 && path.Segments[0].Kind == "database" {
		db = path.Segments[0].Name
	} else if d := a.state.Load().defaultDatabase; d != nil {
		db = *d
	}
	return parseTablesMongo(statement, db)
}

// ResolveTables is never called: ParseTables never asks for a probe.
func (a *Adapter) ResolveTables(context.Context, adapters.TableParse, model.NodePath, *adapters.OpCtx) ([]tableref.Ref, string, error) {
	return nil, "", nil
}

func parseTablesMongo(text, db string) adapters.TableParse {
	stmt, err := parseStatement(text)
	if err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	if db == "" {
		return adapters.TableParse{Refusal: "parse error: no database"}
	}
	s := &mongoScan{db: db, refs: []tableref.Ref{{Database: db, Name: stmt.collection}}}
	for _, arg := range stmt.args {
		s.walk(arg)
	}
	if s.refusal != "" {
		return adapters.TableParse{Refusal: s.refusal}
	}
	return adapters.TableParse{Refs: s.refs}
}

type mongoScan struct {
	db      string
	refs    []tableref.Ref
	refusal string
}

func (s *mongoScan) refuse(msg string) {
	if s.refusal == "" {
		s.refusal = msg
	}
}

func (s *mongoScan) add(db, coll string) {
	if coll == "" {
		s.refuse("stage names no collection")
		return
	}
	if db == "" {
		db = s.db
	}
	s.refs = append(s.refs, tableref.Ref{Database: db, Name: coll})
}

// target reads a collection given as a string or as {db, coll}.
func (s *mongoScan) target(v any, stage string) {
	switch t := v.(type) {
	case string:
		s.add("", t)
	case bson.D:
		db, _ := lookupField(t, "db")
		coll, _ := lookupField(t, "coll")
		dbs, dok := db.(string)
		cs, cok := coll.(string)
		if !dok || !cok {
			s.refuse(stage + " target is malformed")
			return
		}
		s.add(dbs, cs)
	default:
		s.refuse(stage + " target is malformed")
	}
}

func (s *mongoScan) walk(v any) {
	switch t := v.(type) {
	case bson.A:
		for _, e := range t {
			s.walk(e)
		}
	case bson.D:
		for _, e := range t {
			s.stage(e.Key, e.Value)
			s.walk(e.Value)
		}
	}
}

func (s *mongoScan) stage(key string, val any) {
	switch key {
	case "$lookup":
		d, ok := val.(bson.D)
		if !ok {
			s.refuse("$lookup is malformed")
			return
		}
		if from, ok := lookupField(d, "from"); ok {
			s.target(from, "$lookup")
		}
	case "$graphLookup":
		d, ok := val.(bson.D)
		if !ok {
			s.refuse("$graphLookup is malformed")
			return
		}
		from, ok := lookupField(d, "from")
		if !ok {
			s.refuse("$graphLookup is malformed")
			return
		}
		s.target(from, "$graphLookup")
	case "$unionWith":
		if d, ok := val.(bson.D); ok {
			if coll, ok := lookupField(d, "coll"); ok {
				s.target(coll, "$unionWith")
			}
			return
		}
		s.target(val, "$unionWith")
	case "$out":
		s.target(val, "$out")
	case "$merge":
		if d, ok := val.(bson.D); ok {
			into, ok := lookupField(d, "into")
			if !ok {
				s.refuse("$merge is malformed")
				return
			}
			s.target(into, "$merge")
			return
		}
		s.target(val, "$merge")
	}
}
