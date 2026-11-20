//go:build darwin && cgo

package fswatch

import (
	"sync"

	"github.com/fsnotify/fsevents"
)

// droppedFlags mean "something changed and we don't know what": coalesced beyond recognition,
// dropped in the kernel or in our client buffer, or the watched root moved, was deleted or mounted over.
const droppedFlags = fsevents.MustScanSubDirs | fsevents.KernelDropped | fsevents.UserDropped |
	fsevents.RootChanged | fsevents.Mount | fsevents.Unmount

const metaFlags = fsevents.ItemInodeMetaMod | fsevents.ItemChangeOwner | fsevents.ItemXattrMod | fsevents.ItemFinderInfoMod

// fseventsBackend watches whole trees through one stream: no state per directory and no file
// descriptor per file, unlike kqueue. The scope's add and remove are no-ops here: the verdicts alone
// filter. Cannot be built outside a macOS toolchain.
type fseventsBackend struct {
	es *fsevents.EventStream

	stopping chan struct{} // closed first: run discards batches (D10 step 1)
	done     chan struct{} // closed second: run returns (D10 step 3)
	once     sync.Once
}

func startFSEvents(o backendOptions) (backend, error) {
	es := &fsevents.EventStream{
		Paths: o.roots,
		// FileEvents: without it FSEvents reports only the containing directory. WatchRoot delivers
		// RootChanged if a root is moved or deleted, mapped to Rescan.
		Flags: fsevents.FileEvents | fsevents.WatchRoot,
		// NoDefer and IgnoreSelf stay unset: NoDefer is a kernel-side leading-window debounce that
		// would stack on the consumers' own; IgnoreSelf would hide this process's own writes.
		Latency: o.latency,
		// Device MUST stay zero: nonzero switches to device-relative paths with no leading slash,
		// which would match no root, silently, forever.
		Device: 0,
		// Resume false: replaying history would fire spurious events at every start.
		Resume: false,
	}
	if err := es.Start(); err != nil {
		return nil, err
	}
	return &fseventsBackend{es: es, stopping: make(chan struct{}), done: make(chan struct{})}, nil
}

func (b *fseventsBackend) add(string) error { return nil }
func (b *fseventsBackend) remove(string)    {}

// run is the sole reader of es.Events. The C callback blocks on that unbuffered channel, so run
// keeps receiving until done closes, even while stopping: a callback parked mid-send then always
// completes, and es.Stop (which waits on the callback's queue) cannot deadlock.
func (b *fseventsBackend) run(w *Watcher) {
	for {
		select {
		case batch, ok := <-b.es.Events:
			if !ok {
				return
			}
			select {
			case <-b.stopping:
				continue // draining for stop: discard
			default:
			}
			raws := make([]rawEvent, 0, len(batch))
			for _, ev := range batch {
				if ev.Flags&droppedFlags != 0 {
					raws = append(raws, rawEvent{rescan: true})
					continue
				}
				if r, ok := fromFSEvents(ev); ok {
					raws = append(raws, r)
				}
			}
			w.handle(raws)
		case w.sink.pendingC() <- Event{Rescan: true}:
			w.sink.flushed()
		case <-b.done:
			return
		}
	}
}

// stop runs the exact order: drain, stop the stream, let run return, join.
func (b *fseventsBackend) stop(done <-chan struct{}) error {
	b.once.Do(func() {
		close(b.stopping) // 1: run discards what arrives and keeps unblocking the callback
		b.es.Stop()       // 2: safe now, no callback can be parked mid-send
		close(b.done)     // 3: run returns
	})
	<-done // 4: join
	return nil
}

func fromFSEvents(ev fsevents.Event) (rawEvent, bool) {
	var op Op
	if ev.Flags&fsevents.ItemCreated != 0 {
		op |= Create
	}
	if ev.Flags&fsevents.ItemModified != 0 {
		op |= Write
	}
	if ev.Flags&fsevents.ItemRemoved != 0 {
		op |= Remove
	}
	if ev.Flags&fsevents.ItemRenamed != 0 {
		op |= Rename
	}
	if ev.Flags&metaFlags != 0 {
		op |= Meta
	}
	if op == 0 { // HistoryDone, EventIDsWrapped, bare type flags
		return rawEvent{}, false
	}
	return rawEvent{path: ev.Path, op: op, dir: ev.Flags&fsevents.ItemIsDir != 0}, true
}
