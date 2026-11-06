package kafka_test

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
)

// TestMain mirrors postgres_test.go's own: the container this whole package's tests share is torn
// down once, after every test has run — never from an individual test's t.Cleanup, which Go's
// testing package would run the instant the registering test function itself returns (P58b B15).
func TestMain(m *testing.M) {
	testsupport.Prewarm("kafka", "kafkasasl")
	code := m.Run()
	testsupport.StopConcurrently(testsupport.StopKafka, testsupport.StopKafkaSasl)
	os.Exit(code)
}
