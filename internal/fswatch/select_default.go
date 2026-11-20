//go:build !darwin || !cgo

package fswatch

const backendName = "fsnotify"

var defaultBackend startFunc = startFSNotify
