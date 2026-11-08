package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// ItemKind discriminates a plain-struct menu item (D14: Wails-free, so a template can be asserted
// without a running app). P103 Part 3: only the type vocabulary is shared — each app's own
// BuildTemplate (its own menu content) lives in its own internal/appshell.
type ItemKind int

const (
	ItemSeparator ItemKind = iota
	ItemRole               // a Wails role, optionally re-accelerated
	ItemEmit               // a custom item calling MenuDeps.OnEmit with Channel
	ItemQuit               // the custom, role-free Quit item (§1.3)
	ItemNewWindow          // opens a second window (P8 D8) — handled in Go, not a renderer emit (§0.4)
	ItemAction             // runs MenuDeps.Actions[Action] in Go: works with no window focused, unlike ItemEmit
)

// Item.Role is application.Role, so this file is not literally Wails-free — but Role is a bare
// uint constant in roles.go with no cgo behind it, and the package as a whole imports Wails for
// menu.go anyway. Stated plainly rather than pretending the split buys a build-tag win it
// does not.
type Item struct {
	Kind        ItemKind
	Label       string           // ItemEmit / ItemQuit only
	Role        application.Role // ItemRole only
	Accelerator string           // "" for none; also used to re-accelerate an ItemRole
	Channel     string           // ItemEmit only, an app-defined channel string
	Action      string           // ItemAction only, a key of MenuDeps.Actions
}

type Section struct {
	Label string
	Items []Item
}
