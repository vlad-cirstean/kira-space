package jsengine

import (
	"os"
	"runtime"
	"runtime/metrics"
	"time"

	"github.com/dop251/goja"
)

const (
	watchTick  = 5 * time.Millisecond
	minGCSpace = 50 * time.Millisecond
)

// startWatchdog runs in the worker. Soft: heap objects over baseline above budget, force a GC,
// recheck, then interrupt the VM (uncatchable; positioned KindMemory). Hard: runtime total above
// hardTotal means one built-in allocated in a single call; exit at once. A large allocation is
// accounted before its copy runs, so the exit lands within a tick or two.
func startWatchdog(vm *goja.Runtime, baseline, budget, hardTotal uint64) (stop func()) {
	done := make(chan struct{})
	go func() {
		samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/total:bytes"}}
		read := func() (heap, total uint64) {
			metrics.Read(samples)
			return samples[0].Value.Uint64(), samples[1].Value.Uint64()
		}
		tick := time.NewTicker(watchTick)
		defer tick.Stop()
		var lastGC time.Time
		interrupted := false
		for {
			select {
			case <-done:
				return
			case <-tick.C:
			}
			heap, total := read()
			if total > hardTotal {
				os.Exit(exitMemory)
			}
			if interrupted || heap <= baseline+budget || time.Since(lastGC) < minGCSpace {
				continue
			}
			runtime.GC()
			lastGC = time.Now()
			if heap, _ = read(); heap > baseline+budget {
				vm.Interrupt(interruptMemory)
				interrupted = true // keep watching the hard ceiling
			}
		}
	}()
	return func() { close(done) }
}

// heapStats returns heap object bytes and runtime total bytes after a GC.
func heapStats() (heap, total uint64) {
	runtime.GC()
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/total:bytes"}}
	metrics.Read(samples)
	return samples[0].Value.Uint64(), samples[1].Value.Uint64()
}
