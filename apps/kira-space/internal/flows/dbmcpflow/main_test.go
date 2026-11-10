package dbmcpflow

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestMain(m *testing.M) {
	code := flowharness.Main(m)
	stopFixtures()
	os.Exit(code)
}
