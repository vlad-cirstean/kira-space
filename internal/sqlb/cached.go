package sqlb

import "sync"

// Cached holds the text of a statement whose shape never varies by call, built on first use.
// Place it beside a repo's DB; the zero value is ready, so `&Repo{DB: db}` literals keep working.
// Bind values still come from the caller per call.
type Cached struct {
	once sync.Once
	sql  string
}

// SQL returns the statement text, building it with build on the first call only.
func (c *Cached) SQL(build func() Builder) string {
	c.once.Do(func() { c.sql, _ = build().Build() })
	return c.sql
}
