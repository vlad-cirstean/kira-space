package adapterhost

import "context"

// OpSource says who started an op: the user, through the UI, or an MCP client.
const (
	OpSourceUser = "user"
	OpSourceMCP  = "mcp"
)

type opSourceKey struct{}

// WithOpSource tags ctx so every op RunOp starts under it records source.
func WithOpSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, opSourceKey{}, source)
}

// OpSourceOf returns the source set by WithOpSource, OpSourceUser when unset.
func OpSourceOf(ctx context.Context) string {
	if s, ok := ctx.Value(opSourceKey{}).(string); ok && s != "" {
		return s
	}
	return OpSourceUser
}
