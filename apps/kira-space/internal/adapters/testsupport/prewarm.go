package testsupport

import "sync"

// prewarmers maps a fixture kind's name to a closure over its existing memo (fixture.go's
// fixture[T].get) and its own start<Kind> function. One entry per fixture kind in this package —
// adding a fixture here does not change what Start<Kind> does, only what Prewarm can be asked to
// warm ahead of it.
var prewarmers = map[string]func() error{
	"postgres":   func() error { _, err := pgMemo.get(startPostgres); return err },
	"mariadb":    func() error { _, err := mariaMemo.get(startMariadb); return err },
	"mysql":      func() error { _, err := mysqlMemo.get(startMysql); return err },
	"clickhouse": func() error { _, err := clickhouseMemo.get(startClickHouse); return err },
	"redis":      func() error { _, err := redisMemo.get(startRedis); return err },
	"sqs":        func() error { _, err := sqsMemo.get(startSqs); return err },
	"s3":         func() error { _, err := s3Memo.get(startS3); return err },
	"mongo":      func() error { _, err := mongoMemo.get(startMongo); return err },
	"sqlite":     func() error { _, err := sqliteMemo.get(startSqlite); return err },
	"kafka":      func() error { _, err := kafkaMemo.get(startKafka); return err },
	"kafkasasl":  func() error { _, err := kafkaSaslMemo.get(startKafkaSasl); return err },
}

// Prewarm starts the named fixtures concurrently, so a package that needs several containers pays
// the slowest one's startup instead of the sum. Every fixture is memoized (fixture.go), so each
// later Start<Kind>(t) is a cache hit; a start that fails is remembered and still surfaces through
// that Start's own t.Fatalf. A no-op without Docker — each Start<Kind> still skips on its own.
func Prewarm(kinds ...string) {
	if !IsDockerAvailable() {
		return
	}
	var wg sync.WaitGroup
	for _, kind := range kinds {
		start, ok := prewarmers[kind]
		if !ok {
			panic("testsupport: Prewarm: unknown fixture kind " + kind)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = start() // memoized: a failure here is remembered and reported by the later Start<Kind>(t)
		}()
	}
	wg.Wait()
}

// StopConcurrently tears down several memoized fixtures at once, for a TestMain that owns more
// than one.
func StopConcurrently(stops ...func()) {
	var wg sync.WaitGroup
	for _, stop := range stops {
		wg.Add(1)
		go func(stop func()) {
			defer wg.Done()
			stop()
		}(stop)
	}
	wg.Wait()
}
