package harness

// ContextKeeper is the optional role of a provider whose resumed conversation keeps the context it
// started with instead of the current file. It feeds the editor's note only.
type ContextKeeper interface {
	KeepsContextOnResume() bool
}

// KeepsContextOnResume reports whether p implements ContextKeeper and says so.
func KeepsContextOnResume(p Provider) bool {
	k, ok := p.(ContextKeeper)
	return ok && k.KeepsContextOnResume()
}
