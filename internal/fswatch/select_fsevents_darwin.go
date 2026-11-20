//go:build darwin && cgo

package fswatch

const backendName = "fsevents"

var defaultBackend startFunc = startFSEvents
