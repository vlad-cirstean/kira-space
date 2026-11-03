package bridge

// StreamSession is the whole of what gitgraph.go's ServeGitGraph needs from a renderer
// connection — Kira Studio's own bridge.StreamSession (stream.go), copied here rather than
// imported: that file also declares ServeEngineStream, which imports internal/adapterhost, a
// package this app has no use for and does not carry. *application.StreamConn satisfies this
// structurally (Send([]byte) error, Receive() ([]byte, error)), so this package still imports no
// Wails.
type StreamSession interface {
	Send(frame []byte) error
	Receive() ([]byte, error)
}
