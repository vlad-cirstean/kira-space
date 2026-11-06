package adapters

// Constructor builds one Adapter for a connection kind.
type Constructor func(deps Deps) (Adapter, error)

// Unlike the TypeScript, which deferred each adapter's own dynamic import to avoid loading eleven
// drivers into one process at boot, Go has no such cost — every driver is already linked into the
// binary regardless of whether a kind is ever used, so this is a plain constructor table, not a
// lazy-import map.
var loaders = map[string]Constructor{}

// Register adds kind's constructor to the registry. Called once per adapter package's own init(),
// so adding a new adapter never requires editing this file.
func Register(kind string, ctor Constructor) {
	loaders[kind] = ctor
}

// CreateAdapter is registry.ts's createAdapter.
func CreateAdapter(kind string, deps Deps) (Adapter, error) {
	ctor, ok := loaders[kind]
	if !ok {
		return nil, New(CodeUnsupported, kind+" connections are not supported yet", nil)
	}
	return ctor(deps)
}
