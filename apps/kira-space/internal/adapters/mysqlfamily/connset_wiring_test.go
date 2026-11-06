package mysqlfamily

import (
	"context"
	"database/sql/driver"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// TestAcquire_SingleFlightPerDatabase is white-box (package mysqlfamily) coverage for NewConnSet's
// own wiring onto adapters.ConnSet: the LRU/single-flight behaviour itself is covered
// once, generically, by internal/adapters's own connset_test.go — this confirms Acquire (via
// the real key-normalization/Dial closure NewConnSet builds) still serializes concurrent dials for
// the same database with no real MySQL server needed.
func TestAcquire_SingleFlightPerDatabase(t *testing.T) {
	old := mysqlNewConnector
	t.Cleanup(func() { mysqlNewConnector = old })

	var inFlight, maxInFlight, totalDials int32
	mysqlNewConnector = func(_ *mysql.Config) (driver.Connector, error) {
		n := atomic.AddInt32(&inFlight, 1)
		atomic.AddInt32(&totalDials, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if n <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil, context.DeadlineExceeded
	}

	s := NewConnSet(model.ResolvedConnectionConfig{}, Profile{
		Kind: "test", ServerLabel: "Test",
		ApplyEngineOptions: func(*mysql.Config, model.ResolvedConnectionConfig, LogFunc) {},
	}, func(string, string) {})
	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = s.Acquire(context.Background(), "")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got > 1 {
		t.Errorf("max concurrent dials for the primary database = %d, want at most 1", got)
	}
	if atomic.LoadInt32(&totalDials) == 0 {
		t.Fatal("mysqlNewConnector was never called at all")
	}
}
