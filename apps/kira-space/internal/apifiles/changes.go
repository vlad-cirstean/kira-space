package apifiles

// Owner scopes of a variable change.
const (
	OwnerCollection  = "collection"
	OwnerEnvironment = "environment"
)

// OwnerKey names one variable owner.
type OwnerKey struct{ Scope, ID string }

// Changes lists what a load or a write made stale in the renderer's caches.
type Changes struct {
	Tree         bool
	Environments bool
	// Config is set when a file's problem set changed.
	Config    bool
	Requests  map[string]struct{}
	Variables map[OwnerKey]struct{}
}

// Empty reports whether nothing changed.
func (c Changes) Empty() bool {
	return !c.Tree && !c.Environments && !c.Config && len(c.Requests) == 0 && len(c.Variables) == 0
}

func (c *Changes) request(id string) {
	if c.Requests == nil {
		c.Requests = map[string]struct{}{}
	}
	c.Requests[id] = struct{}{}
}

func (c *Changes) variables(scope, id string) {
	if c.Variables == nil {
		c.Variables = map[OwnerKey]struct{}{}
	}
	c.Variables[OwnerKey{scope, id}] = struct{}{}
}

func (c *Changes) merge(o Changes) {
	c.Tree = c.Tree || o.Tree
	c.Environments = c.Environments || o.Environments
	c.Config = c.Config || o.Config
	for id := range o.Requests {
		c.request(id)
	}
	for k := range o.Variables {
		c.variables(k.Scope, k.ID)
	}
}
