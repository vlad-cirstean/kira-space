package mongo

import "testing"

func TestParseStatement_CollectionForms(t *testing.T) {
	cases := map[string]string{
		`db.orders.find()`:                      "orders",
		`db.orders.archive.find({})`:            "orders.archive",
		`db["my-coll"].find()`:                  "my-coll",
		`db.getCollection("système").find()`:    "système",
		`db.getCollection("a.b").deleteOne({})`: "a.b",
	}
	for text, want := range cases {
		stmt, err := parseStatement(text)
		if err != nil {
			t.Errorf("%s: %v", text, err)
			continue
		}
		if stmt.collection != want {
			t.Errorf("%s: collection = %q, want %q", text, stmt.collection, want)
		}
	}
	for _, text := range []string{`db[1].find()`, `db[""].find()`, `db.getCollection(x).find()`, `db.orders.drop()`, `db.orders.find`} {
		if _, err := parseStatement(text); err == nil {
			t.Errorf("%s: want error", text)
		}
	}
}
