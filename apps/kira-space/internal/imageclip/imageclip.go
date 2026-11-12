// Package imageclip puts several images on the system clipboard as separate items. The
// web Async Clipboard API takes one image and Wails' clipboard is text only, so this is native.
// Only macOS can: NSPasteboard holds an ordered array of items, each with its own types.
package imageclip

// Clipboard writes image batches. invoke runs a func on the AppKit main thread.
type Clipboard struct {
	invoke func(func())
}

// New returns a Clipboard. Production passes application.InvokeSync; a direct call works in tests.
func New(invoke func(func())) *Clipboard {
	return &Clipboard{invoke: invoke}
}
