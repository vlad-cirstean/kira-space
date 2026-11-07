package sqlb

// Sub embeds q as a parenthesised subquery in a hand-written fragment of b (a builder's own Var).
// go-sqlbuilder renders a builder passed to Var or Equal without parentheses, which is invalid SQL
// inside a comparison or function call; wrap it here instead of by hand.
func Sub(b interface{ Var(arg any) string }, q Builder) string {
	return "(" + b.Var(q) + ")"
}
