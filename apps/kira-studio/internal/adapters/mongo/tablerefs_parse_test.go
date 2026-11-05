package mongo

import "testing"

func TestParseTablesMongo(t *testing.T) {
	cases := []struct {
		name, in string
		refs     []string
		refusal  string
	}{
		{"find", `db.orders.find({})`, []string{"app.orders"}, ""},
		{"getCollection", `db.getCollection("a.b").find({})`, []string{"app.a.b"}, ""},
		{"lookup", `db.orders.aggregate([{$lookup:{from:"secret",localField:"a",foreignField:"b",as:"x"}}])`, []string{"app.orders", "app.secret"}, ""},
		{"lookup other db", `db.orders.aggregate([{$lookup:{from:{db:"d2",coll:"c"},as:"x"}}])`, []string{"app.orders", "d2.c"}, ""},
		{"lookup pipeline nested", `db.orders.aggregate([{$lookup:{from:"a",pipeline:[{$lookup:{from:"deep",as:"y"}}],as:"x"}}])`, []string{"app.orders", "app.a", "app.deep"}, ""},
		{"facet nested", `db.orders.aggregate([{$facet:{f:[{$unionWith:"u"}]}}])`, []string{"app.orders", "app.u"}, ""},
		{"graphLookup", `db.orders.aggregate([{$graphLookup:{from:"g",startWith:"$a",connectFromField:"a",connectToField:"b",as:"x"}}])`, []string{"app.orders", "app.g"}, ""},
		{"unionWith doc", `db.orders.aggregate([{$unionWith:{coll:"u",pipeline:[]}}])`, []string{"app.orders", "app.u"}, ""},
		{"unionWith pipeline only", `db.orders.aggregate([{$unionWith:{pipeline:[{$documents:[{a:1}]}]}}])`, []string{"app.orders"}, ""},
		{"out", `db.orders.aggregate([{$out:"dst"}])`, []string{"app.orders", "app.dst"}, ""},
		{"merge into doc", `db.orders.aggregate([{$merge:{into:{db:"d2",coll:"c"}}}])`, []string{"app.orders", "d2.c"}, ""},
		{"merge string", `db.orders.aggregate([{$merge:"m"}])`, []string{"app.orders", "app.m"}, ""},
		{"lookup bad from", `db.orders.aggregate([{$lookup:{from:5,as:"x"}}])`, nil, "$lookup target is malformed"},
		{"merge no into", `db.orders.aggregate([{$merge:{on:"_id"}}])`, nil, "$merge is malformed"},
		{"unparsable", `db.orders.find(`, nil, "parse error"},
		{"unsupported method", `db.orders.drop()`, nil, "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parseTablesMongo(c.in, "app")
			if c.refusal != "" {
				if len(p.Refusal) < len(c.refusal) || p.Refusal[:len(c.refusal)] != c.refusal {
					t.Fatalf("refusal = %q, want prefix %q", p.Refusal, c.refusal)
				}
				return
			}
			if p.Refusal != "" {
				t.Fatalf("refused: %s", p.Refusal)
			}
			if len(p.Refs) != len(c.refs) {
				t.Fatalf("refs = %v, want %v", p.Refs, c.refs)
			}
			for i, r := range p.Refs {
				if got := r.Database + "." + r.Name; got != c.refs[i] {
					t.Fatalf("ref %d = %s, want %s", i, got, c.refs[i])
				}
			}
		})
	}
}
