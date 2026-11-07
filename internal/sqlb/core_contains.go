package sqlb

// ContainsExpr is the position of arg inside col: 0 when absent, case-sensitive, no LIKE wildcards.
// SQLite: instr; Postgres: strpos.
func ContainsExpr(d Dialect, col, arg string) string {
	if d.sqlite() {
		return "instr(" + col + ", " + arg + ")"
	}
	return "strpos(" + col + ", " + arg + ")"
}
